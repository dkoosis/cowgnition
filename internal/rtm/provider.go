// internal/rtm/provider.go
// Package rtm provides a client for interacting with the Remember The Milk (RTM) API v2.
// This file implements the MCP ResourceProvider interface for handling RTM authentication.
package rtm

import (
	"context"
	"encoding/json" // Used for marshaling JSON responses.
	"fmt"           // Used for formatting error messages and log details.
	"sync"          // Provides mutex for synchronizing access to internal state.

	"github.com/cockroachdb/errors"                           // Error handling library.
	"github.com/dkoosis/cowgnition/internal/logging"          // Project's structured logging helper.
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"  // MCP resource definitions.
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors" // MCP custom error types.
)

// logger initializes the structured logger for the rtm_provider part of the rtm package.
var logger = logging.GetLogger("rtm_provider")

const (
	// AuthResourceURI defines the unique identifier for the RTM authentication resource
	// within the MCP framework.
	AuthResourceURI = "auth://rtm"

	// PermRead represents the 'read' permission level in RTM.
	PermRead = "read"
	// PermWrite represents the 'write' permission level in RTM.
	PermWrite = "write"
	// PermDelete represents the 'delete' permission level in RTM (includes read and write).
	PermDelete = "delete"
)

// AuthProvider implements the MCP ResourceProvider interface to manage authentication
// with the Remember The Milk API. It handles token storage, validation, and the
// RTM desktop application authentication flow (using frobs).
type AuthProvider struct {
	client    *Client           // The RTM API client used for communication.
	storage   *TokenStorage     // Handles persistent storage of the RTM auth token.
	authState map[string]string // Stores the requested permission level associated with an active frob during the auth flow. Key: frob, Value: permission.
	mu        sync.Mutex        // Protects concurrent access to the authState map.
}

// NewAuthProvider creates and initializes a new RTM AuthProvider.
// It sets up the RTM client and token storage based on the provided API key,
// shared secret, and token storage path. Returns an error if token storage
// initialization fails.
func NewAuthProvider(apiKey, sharedSecret, tokenPath string) (*AuthProvider, error) {
	storage, err := NewTokenStorage(tokenPath)
	if err != nil {
		// Wrap the storage creation error for context.
		wrappedErr := errors.Wrap(err, "NewAuthProvider: could not create token storage")
		// Return a specific RTM/Auth error. 0 indicates no specific RTM API error code applies here.
		return nil, cgerr.NewRTMError(
			0,
			"Failed to initialize token storage for RTM provider.",
			wrappedErr,
			map[string]interface{}{
				"token_path": tokenPath,
			},
		)
	}

	// Initialize the RTM API client.
	client := NewClient(apiKey, sharedSecret)

	// Return the fully initialized provider.
	return &AuthProvider{
		client:    client,
		storage:   storage,
		authState: make(map[string]string), // Initialize the frob state map.
	}, nil
}

// GetResourceDefinitions returns the definition of the RTM authentication resource
// managed by this provider, conforming to the MCP ResourceProvider interface.
func (p *AuthProvider) GetResourceDefinitions() []definitions.ResourceDefinition {
	return []definitions.ResourceDefinition{
		{
			Name:        AuthResourceURI,
			Description: "Manages authentication with Remember The Milk (RTM). Provides status, initiates auth flow, or completes it using a 'frob'.",
			Arguments: []definitions.ResourceArgument{
				{
					Name:        "frob",
					Description: "The 'frob' obtained from RTM during the web authentication flow. Provide this to complete authentication.",
					Required:    false, // Optional: only needed to complete the flow.
				},
				{
					Name:        "perms",
					Description: fmt.Sprintf("Requested RTM permissions (%s, %s, %s). Used when initiating a new authentication flow.", PermRead, PermWrite, PermDelete),
					Required:    false, // Optional: only needed when initiating the flow, defaults to 'delete'.
				},
			},
		},
	}
}

// ReadResource handles read requests for the RTM authentication resource (AuthResourceURI).
// It implements the core logic:
// 1. Checks if a valid token already exists.
// 2. If not, checks if a 'frob' is provided to complete an ongoing auth flow.
// 3. If neither, initiates a new authentication flow.
// Returns the resource state (JSON string), content type ("application/json"), and any error.
func (p *AuthProvider) ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) {
	// Ensure the request is for the resource this provider handles.
	if name != AuthResourceURI {
		return "", "", cgerr.NewResourceError(
			fmt.Sprintf("RTM AuthProvider does not handle resource: %s.", name),
			nil,
			map[string]interface{}{
				"requested_resource": name,
				"handled_resource":   AuthResourceURI,
			},
		)
	}

	// Attempt to use an existing, valid token first.
	tokenResult, err := p.checkExistingToken()
	if err == nil {
		// A valid token was found and verified. Return the status.
		logger.Info("RTM authentication status checked: Already authenticated.")
		return tokenResult, "application/json", nil
	}

	// Log the reason why checkExistingToken failed, unless it was the expected 'no valid token found' state.
	if !cgerr.IsAuthError(err, "No valid token found") {
		logger.Warn("Failed during existing token check.", "error", fmt.Sprintf("%+v", err))
	} else {
		// This is the expected path if no token exists or the stored one is invalid.
		logger.Debug("No valid existing RTM token found, proceeding with authentication flow.")
	}

	// If a frob is provided in the arguments, attempt to complete the authentication flow with it.
	if frob, ok := args["frob"]; ok && frob != "" {
		logger.Info("Attempting to complete RTM authentication using provided frob.", "frob", frob)
		return p.handleFrobAuthentication(frob)
	}

	// If no valid token and no frob provided, start a new authentication flow.
	logger.Info("Initiating new RTM authentication flow.")
	return p.startNewAuthFlow(args)
}

// checkExistingToken attempts to load a token from storage and verify its validity with the RTM API.
// If a valid token is found, it returns a JSON string describing the authenticated state and a nil error.
// If no token is stored, the token is invalid, or an error occurs during loading/verification,
// it returns an empty string and an appropriate error (cgerr.AuthError).
func (p *AuthProvider) checkExistingToken() (string, error) {
	// Attempt to load a token from persistent storage.
	token, err := p.storage.LoadToken()
	if err != nil {
		logger.Warn("Error loading RTM token from storage.", "path", p.storage.TokenPath, "error", fmt.Sprintf("%+v", err))
		// Wrap the loading error.
		return "", cgerr.NewAuthError(
			"Failed to load token from storage.",
			errors.Wrap(err, "checkExistingToken: failed loading token"),
			map[string]interface{}{
				"token_path": p.storage.TokenPath,
			},
		)
	}

	// If a token string was loaded from the file, verify it with RTM.
	if token != "" {
		p.client.SetAuthToken(token)
		// Call the RTM API to check if the token is still valid.
		auth, checkErr := p.client.CheckToken()
		if checkErr == nil && auth != nil {
			// Token is valid. Prepare the success response.
			response := map[string]interface{}{
				"status":      "authenticated",
				"username":    auth.User.Username,
				"fullname":    auth.User.Fullname,
				"permissions": auth.Perms,
			}
			// Marshal the response to JSON.
			responseJSON, marshalErr := json.MarshalIndent(response, "", "  ")
			if marshalErr != nil {
				// Handle failure during JSON marshaling.
				wrappedErr := errors.Wrap(marshalErr, "checkExistingToken: failed to marshal valid token response")
				return "", cgerr.NewResourceError(
					"Failed to marshal valid token status response.",
					wrappedErr,
					map[string]interface{}{
						// Log the struct that failed to marshal for debugging.
						"response_struct": fmt.Sprintf("%+v", response),
					},
				)
			}
			logger.Info("Existing valid RTM token confirmed.", "user", auth.User.Username)
			// Return the JSON response indicating authenticated state.
			return string(responseJSON), nil
		}
		// Token check failed (token is invalid or expired). Log the reason.
		logger.Info("Existing RTM token found but is invalid, proceeding to re-authenticate.", "check_error", fmt.Sprintf("%+v", checkErr))
		// Fall through to return 'No valid token found' error below.
	}

	// If no token was loaded, or the loaded token was invalid.
	return "", cgerr.NewAuthError(
		"No valid token found.", // Specific message checked in ReadResource.
		nil,                     // No underlying Go error, this represents a state.
		map[string]interface{}{
			"token_path": p.storage.TokenPath,
		},
	)
}

// handleFrobAuthentication attempts to exchange a provided 'frob' for a valid RTM auth token.
// If successful, it saves the token, updates the client, and returns a JSON response indicating
// successful authentication. Returns an error if the frob is invalid or token exchange fails.
func (p *AuthProvider) handleFrobAuthentication(frob string) (string, string, error) {
	// Exchange the frob for an authentication token via the RTM API.
	auth, err := p.client.GetToken(frob)
	if err != nil {
		// Wrap the GetToken error.
		return "", "", cgerr.NewAuthError(
			fmt.Sprintf("Failed to get RTM token using frob '%s'.", frob),
			err, // Keep the original RTM client error.
			map[string]interface{}{
				"frob": frob,
			},
		)
	}

	// Successfully obtained token. Persist it for future use.
	if saveErr := p.storage.SaveToken(auth.Token); saveErr != nil {
		// Log failure to save the token, but proceed as authentication itself succeeded.
		// The token will be used for the current session via p.client.SetAuthToken below.
		logger.Error("Failed to save newly acquired RTM token to persistent storage. Authentication will proceed for this session.",
			"path", p.storage.TokenPath, "error", fmt.Sprintf("%+v", saveErr))
	} else {
		logger.Info("Successfully obtained and saved new RTM token.", "user", auth.User.Username)
	}

	// Use the newly obtained token for the current client instance.
	p.client.SetAuthToken(auth.Token)

	// Prepare the success response.
	response := map[string]interface{}{
		"status":      "authenticated",
		"username":    auth.User.Username,
		"fullname":    auth.User.Fullname,
		"permissions": auth.Perms, // Permissions granted with the token.
	}

	// Marshal the response to JSON.
	responseJSON, marshalErr := json.MarshalIndent(response, "", "  ")
	if marshalErr != nil {
		// Handle failure during JSON marshaling.
		wrappedErr := errors.Wrap(marshalErr, "handleFrobAuthentication: failed to marshal successful auth response")
		return "", "", cgerr.NewResourceError(
			"Failed to marshal successful authentication response.",
			wrappedErr,
			map[string]interface{}{
				"frob":     frob,
				"username": auth.User.Username,
			},
		)
	}

	// Return the JSON response indicating successful authentication.
	return string(responseJSON), "application/json", nil
}

// startNewAuthFlow begins the RTM desktop authentication flow.
// It requests a 'frob' from RTM, generates the RTM web authentication URL for the user,
// and returns a JSON response containing the auth URL, the frob, and instructions for the user.
// Uses permissions from args or defaults to 'delete'.
func (p *AuthProvider) startNewAuthFlow(args map[string]string) (string, string, error) {
	// Determine the requested permission level, defaulting to 'delete' (highest).
	perms := PermDelete // Default permission.
	if pArg, ok := args["perms"]; ok {
		if pArg == PermRead || pArg == PermWrite || pArg == PermDelete {
			perms = pArg // Use valid provided permission.
		} else {
			// Log if an invalid permission string was provided.
			logger.Warn("Invalid 'perms' argument provided for RTM auth flow, defaulting to 'delete'.", "provided_perms", pArg)
		}
	}

	// Obtain a new frob from the RTM API to start the auth flow.
	frob, err := p.client.GetFrob()
	if err != nil {
		// Wrap the GetFrob error.
		return "", "", cgerr.NewAuthError(
			"Failed to get frob from RTM to start authentication flow.",
			err,
			map[string]interface{}{
				"requested_perms": perms,
			},
		)
	}

	// Store the requested permission level associated with this frob, protected by mutex.
	p.mu.Lock()
	p.authState[frob] = perms
	p.mu.Unlock()
	logger.Debug("Obtained new frob for RTM auth flow.", "frob", frob, "perms", perms)

	// Generate the URL the user needs to visit in their browser to authorize the application.
	authURL := p.client.GetAuthURL(frob, perms)

	// Prepare the response containing instructions and necessary details for the user.
	response := map[string]interface{}{
		"status":       "unauthorized", // Current state.
		"auth_url":     authURL,        // URL for the user to visit.
		"frob":         frob,           // Frob to be used after authorization.
		"permissions":  perms,          // Permissions requested.
		"instructions": "Visit the auth_url to authorize this application, then access this resource again providing the returned 'frob' as an argument.",
	}

	// Marshal the response to JSON.
	responseJSON, marshalErr := json.MarshalIndent(response, "", "  ")
	if marshalErr != nil {
		// Handle failure during JSON marshaling.
		wrappedErr := errors.Wrap(marshalErr, "startNewAuthFlow: failed to marshal auth flow instructions response")
		return "", "", cgerr.NewResourceError(
			"Failed to marshal authentication flow instructions response.",
			wrappedErr,
			map[string]interface{}{
				"frob":  frob,
				"perms": perms,
			},
		)
	}

	// Return the JSON response with auth instructions.
	return string(responseJSON), "application/json", nil
}
