// internal/rtm/provider.go
package rtm

import (
	"context"
	"encoding/json"
	"fmt" // Import slog
	"sync"

	"github.com/cockroachdb/errors"                  // Import errors
	"github.com/dkoosis/cowgnition/internal/logging" // Import project logging helper
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// Initialize the logger at the package level
var logger = logging.GetLogger("rtm_provider")

const (
	// Resource URIs.
	AuthResourceURI = "auth://rtm"

	// Permissions.
	PermRead   = "read"
	PermWrite  = "write"
	PermDelete = "delete"
)

// AuthProvider implements the MCP ResourceProvider interface for RTM authentication.
type AuthProvider struct {
	client    *Client
	storage   *TokenStorage
	authState map[string]string // Maps frobs to their permission level
	mu        sync.Mutex
}

// NewAuthProvider creates a new RTM auth provider.
func NewAuthProvider(apiKey, sharedSecret, tokenPath string) (*AuthProvider, error) {
	storage, err := NewTokenStorage(tokenPath)
	if err != nil {
		// Apply change from assessment example: Wrap err explicitly before passing to cgerr helper
		wrappedErr := errors.Wrap(err, "NewAuthProvider: could not create token storage")
		return nil, cgerr.NewRTMError(
			0, // Assuming 0 means no specific RTM API error code
			"Failed to create token storage",
			wrappedErr, // Pass the wrapped error
			map[string]interface{}{
				"token_path": tokenPath,
			},
		)
	}

	client := NewClient(apiKey, sharedSecret)

	return &AuthProvider{
		client:    client,
		storage:   storage,
		authState: make(map[string]string),
	}, nil
}

// GetResourceDefinitions returns the resource definitions provided by this provider.
func (p *AuthProvider) GetResourceDefinitions() []definitions.ResourceDefinition {
	return []definitions.ResourceDefinition{
		{
			Name:        AuthResourceURI,
			Description: "Remember The Milk authentication",
			Arguments: []definitions.ResourceArgument{
				{
					Name:        "frob",
					Description: "RTM frob for authentication flow",
					Required:    false,
				},
				{
					Name:        "perms",
					Description: "RTM permissions (read, write, delete)",
					Required:    false,
				},
			},
		},
	}
}

// ReadResource handles reading RTM authentication resources.
func (p *AuthProvider) ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) {
	if name != AuthResourceURI {
		// This already uses cgerr, no change needed based on specific rule
		return "", "", cgerr.NewResourceError(
			fmt.Sprintf("Resource not found: %s", name),
			nil,
			map[string]interface{}{
				"resource_name":       name,
				"available_resources": []string{AuthResourceURI},
			},
		)
	}

	// Check for existing token first
	tokenResult, err := p.checkExistingToken()
	if err == nil {
		// Token is valid and checked, return result
		return tokenResult, "application/json", nil
	}
	// Log the error from checkExistingToken if it's not just 'no valid token found'
	if !cgerr.IsAuthError(err, "No valid token found") {
		logger.Warn("Failed to check existing token", "error", fmt.Sprintf("%+v", err))
	} else {
		logger.Debug("No valid existing token found, proceeding with auth flow.")
	}

	// Handle authentication with frob if provided
	if frob, ok := args["frob"]; ok && frob != "" {
		return p.handleFrobAuthentication(frob)
	}

	// Start new authentication flow
	return p.startNewAuthFlow(args)
}

// checkExistingToken verifies if we already have a valid token.
// Returns ("", error) if no valid token or error occurred.
// Returns (jsonData, nil) if valid token found.
func (p *AuthProvider) checkExistingToken() (string, error) {
	// Check if a token is already stored
	token, err := p.storage.LoadToken()
	if err != nil {
		// Replace log.Printf with structured logging
		logger.Warn("Error loading token from storage", "path", p.storage.TokenPath, "error", fmt.Sprintf("%+v", err))
		// Wrap error before returning, keeping cgerr type
		return "", cgerr.NewAuthError(
			"Failed to load token",
			errors.Wrap(err, "checkExistingToken: failed loading token"), // Add wrap context
			map[string]interface{}{
				"token_path": p.storage.TokenPath,
			},
		)
	}

	// If we have a token, verify it's still valid
	if token != "" {
		p.client.SetAuthToken(token)
		auth, checkErr := p.client.CheckToken() // Renamed err to checkErr to avoid conflict
		if checkErr == nil && auth != nil {
			// Token is valid
			response := map[string]interface{}{
				"status":      "authenticated",
				"username":    auth.User.Username,
				"fullname":    auth.User.Fullname,
				"permissions": auth.Perms,
			}
			responseJSON, marshalErr := json.MarshalIndent(response, "", "  ") // Renamed err to marshalErr
			if marshalErr != nil {
				// This already uses cgerr, but let's ensure wrapping consistency
				wrappedErr := errors.Wrap(marshalErr, "checkExistingToken: failed to marshal valid token response")
				return "", cgerr.NewResourceError(
					"Failed to marshal response",
					wrappedErr,
					map[string]interface{}{
						"response_struct": fmt.Sprintf("%+v", response), // Log struct representation
					},
				)
			}
			logger.Info("Existing valid RTM token confirmed", "user", auth.User.Username)
			return string(responseJSON), nil
		}
		// Token is invalid, log and continue with auth flow
		// Replace log.Printf with structured logging
		logger.Info("Existing RTM token found but invalid, proceeding to re-authenticate", "check_error", fmt.Sprintf("%+v", checkErr))
	}

	// No token loaded or token was invalid
	return "", cgerr.NewAuthError(
		"No valid token found",
		nil, // No underlying error to wrap here, it's a state
		map[string]interface{}{
			"token_path": p.storage.TokenPath,
		},
	)
}

// handleFrobAuthentication processes authentication with a provided frob.
func (p *AuthProvider) handleFrobAuthentication(frob string) (string, string, error) {
	// Try to get token for frob
	auth, err := p.client.GetToken(frob)
	if err != nil {
		// This already uses cgerr, ensure function context in message
		return "", "", cgerr.NewAuthError(
			fmt.Sprintf("handleFrobAuthentication: Failed to get token for frob '%s'", frob), // Add context
			err, // Keep original error for wrapping
			map[string]interface{}{
				"frob": frob,
			},
		)
	}

	// Store the token
	if saveErr := p.storage.SaveToken(auth.Token); saveErr != nil { // Renamed err to saveErr
		// Replace log.Printf with structured logging
		logger.Error("Failed to save newly acquired token", "path", p.storage.TokenPath, "error", fmt.Sprintf("%+v", saveErr))
		// Decide if this should be a hard error or just a warning.
		// Current logic continues, so logging is appropriate.
		// We could potentially add a detail to the final response indicating the save issue.
	} else {
		logger.Info("Successfully saved new RTM token.", "user", auth.User.Username)
	}

	// Set the token for future requests in this provider instance
	p.client.SetAuthToken(auth.Token)

	// Return successful authentication response
	response := map[string]interface{}{
		"status":      "authenticated",
		"username":    auth.User.Username,
		"fullname":    auth.User.Fullname,
		"permissions": auth.Perms,
	}

	responseJSON, marshalErr := json.MarshalIndent(response, "", "  ") // Renamed err to marshalErr
	if marshalErr != nil {
		// This already uses cgerr, ensure function context
		wrappedErr := errors.Wrap(marshalErr, "handleFrobAuthentication: failed to marshal successful auth response")
		return "", "", cgerr.NewResourceError(
			"Failed to marshal authentication response",
			wrappedErr,
			map[string]interface{}{
				"frob":     frob,
				"username": auth.User.Username,
			},
		)
	}

	return string(responseJSON), "application/json", nil
}

// startNewAuthFlow initiates a new authentication flow.
func (p *AuthProvider) startNewAuthFlow(args map[string]string) (string, string, error) {
	// Get permissions parameter, default to "delete" (highest)
	perms := PermDelete
	if pArg, ok := args["perms"]; ok && (pArg == PermRead || pArg == PermWrite || pArg == PermDelete) {
		perms = pArg
	} else if ok {
		logger.Warn("Invalid 'perms' argument provided, defaulting to 'delete'", "provided_perms", pArg)
	}

	// Start desktop authentication flow
	frob, err := p.client.GetFrob()
	if err != nil {
		// This already uses cgerr, ensure function context
		return "", "", cgerr.NewAuthError(
			"startNewAuthFlow: Failed to get frob for authentication flow", // Add context
			err,
			map[string]interface{}{
				"requested_perms": perms,
			},
		)
	}

	// Store frob with requested permissions
	p.mu.Lock()
	p.authState[frob] = perms
	p.mu.Unlock()
	logger.Debug("Obtained new frob for auth flow", "frob", frob, "perms", perms)

	// Generate authentication URL
	authURL := p.client.GetAuthURL(frob, perms)

	// Return response with auth URL and frob
	response := map[string]interface{}{
		"status":       "unauthorized",
		"auth_url":     authURL,
		"frob":         frob,
		"permissions":  perms,
		"instructions": "Visit the auth_url to authorize this application, then access this resource again with the frob parameter.",
	}

	responseJSON, marshalErr := json.MarshalIndent(response, "", "  ") // Renamed err to marshalErr
	if marshalErr != nil {
		// This already uses cgerr, ensure function context
		wrappedErr := errors.Wrap(marshalErr, "startNewAuthFlow: failed to marshal auth flow instructions response")
		return "", "", cgerr.NewResourceError(
			"Failed to marshal auth flow response",
			wrappedErr,
			map[string]interface{}{
				"frob":  frob,
				"perms": perms,
			},
		)
	}

	return string(responseJSON), "application/json", nil
}
