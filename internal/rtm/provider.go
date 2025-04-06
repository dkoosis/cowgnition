// file: internal/rtm/provider.go
// Package rtm provides a client for interacting with the Remember The Milk (RTM) API v2.
// This file implements the MCP ResourceProvider interface for RTM authentication status.
// Authentication actions (initiate, complete) should be exposed via the ToolProvider interface.
// Terminate all comments with a period.
package rtm

import (
	"context"
	"encoding/json" // Used for marshaling JSON responses.
	"fmt"           // Used for formatting error messages and log details.

	"github.com/cockroachdb/errors"                           // Error handling library.
	"github.com/dkoosis/cowgnition/internal/logging"          // Project's structured logging helper.
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"  // Corrected MCP resource definitions.
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors" // MCP custom error types.
)

// logger initializes the structured logger for the rtm_provider part of the rtm package.
var logger = logging.GetLogger("rtm_provider")

const (
	// AuthResourceURI defines the unique identifier for the RTM authentication resource
	// within the MCP framework. Reading this resource provides the current auth status.
	AuthResourceURI = "auth://rtm"
	// AuthResourceName defines the human-readable name for the RTM authentication resource.
	AuthResourceName = "RTM Authentication Status"
	// AuthResourceDescription describes the purpose of reading the RTM authentication resource.
	AuthResourceDescription = "Provides the current authentication status with Remember The Milk (RTM)."
	// AuthResourceMimeType defines the content type returned by reading the auth resource.
	AuthResourceMimeType = "application/json"

	// PermRead represents the 'read' permission level in RTM.
	PermRead = "read"
	// PermWrite represents the 'write' permission level in RTM.
	PermWrite = "write"
	// PermDelete represents the 'delete' permission level in RTM (includes read and write).
	PermDelete = "delete"
)

// AuthProvider implements the MCP ResourceProvider interface to manage authentication status
// with the Remember The Milk API. It handles token storage and validation.
// NOTE: To handle authentication actions (initiate flow, complete with frob),
// this type should ALSO implement the mcp.ToolProvider interface.
type AuthProvider struct {
	client  *Client       // The RTM API client used for communication.
	storage *TokenStorage // Handles persistent storage of the RTM auth token.
	// Removed authState map and mutex, as flow logic moves to Tools.
}

// NewAuthProvider creates and initializes a new RTM AuthProvider.
// It sets up the RTM client and token storage based on the provided API key,
// shared secret, and token storage path. Returns an error if token storage
// initialization fails.
func NewAuthProvider(apiKey, sharedSecret, tokenPath string) (*AuthProvider, error) {
	storage, err := NewTokenStorage(tokenPath)
	if err != nil {
		// Wrap the storage creation error for context.
		wrappedErr := errors.Wrap(err, "NewAuthProvider: could not create token storage.")
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
		client:  client,
		storage: storage,
		// Removed authState initialization.
	}, nil
}

// GetResourceDefinitions returns the definition of the RTM authentication resource
// managed by this provider, conforming to the MCP ResourceProvider interface.
// This definition MUST conform to the definitions.Resource struct.
func (p *AuthProvider) GetResourceDefinitions() []definitions.Resource { // Return type changed.
	// Corrected: Assign pointers for optional fields.
	desc := AuthResourceDescription
	mime := AuthResourceMimeType
	return []definitions.Resource{ // Use the corrected struct type.
		{
			// URI is the unique identifier used in resources/read requests.
			URI: AuthResourceURI,
			// Name is the human-readable name for UIs.
			Name: AuthResourceName,
			// Description explains what reading the resource provides.
			Description: &desc, // Assign pointer.
			// MimeType indicates the format of the content returned by ReadResource.
			MimeType: &mime, // Assign pointer.
			// Arguments field removed as it's not part of the spec Resource definition.
		},
	}
}

// ReadResource handles read requests for the RTM authentication resource (AuthResourceURI).
// It checks the current authentication status (using stored token) and returns it.
// This method SHOULD NOT perform actions like initiating auth or completing auth with a frob,
// as those belong in MCP Tools. It returns the status data packaged according to the
// ReadResourceResult specification. The function signature MUST match the updated interface.
func (p *AuthProvider) ReadResource(ctx context.Context, uri string) (definitions.ReadResourceResult, error) {
	emptyResult := definitions.ReadResourceResult{} // Helper for error returns.

	// Ensure the request is for the resource URI this provider handles.
	if uri != AuthResourceURI {
		err := cgerr.NewResourceError(
			fmt.Sprintf("RTM AuthProvider does not handle resource URI: %s.", uri),
			nil,
			map[string]interface{}{
				"requested_uri": uri,
				"handled_uri":   AuthResourceURI,
			},
		)
		logger.Warn("Received read request for unhandled URI.", "uri", uri)
		return emptyResult, err
	}

	// Check the authentication status using the stored token.
	// checkExistingToken now returns map[string]interface{} representing the status, or error.
	statusMap, checkErr := p.checkExistingToken(ctx) // Pass context down.

	var statusJSON string
	if checkErr != nil {
		// If the error is specifically "No valid token found", return an "unauthorized" status.
		if cgerr.IsAuthError(checkErr, "No valid token found") {
			logger.Info("RTM authentication status checked: Unauthorized.")
			statusMap = map[string]interface{}{"status": "unauthorized"}
			// Marshal the unauthorized status map to JSON.
			jsonBytes, marshalErr := json.MarshalIndent(statusMap, "", "  ")
			if marshalErr != nil {
				// This should be unlikely for a simple map, but handle it.
				wrappedErr := errors.Wrap(marshalErr, "ReadResource: failed to marshal unauthorized status.")
				return emptyResult, cgerr.NewResourceError("Failed to marshal unauthorized status.", wrappedErr, nil)
			}
			statusJSON = string(jsonBytes)
		} else {
			// For any other error during token check (e.g., storage issue), return the error.
			logger.Error("Error checking RTM authentication status.", "error", fmt.Sprintf("%+v", checkErr))
			// Propagate the original error.
			return emptyResult, checkErr
		}
	} else {
		// Token check was successful, statusMap contains authenticated user info.
		logger.Info("RTM authentication status checked: Authenticated.")
		// Marshal the success status map to JSON.
		jsonBytes, marshalErr := json.MarshalIndent(statusMap, "", "  ")
		if marshalErr != nil {
			wrappedErr := errors.Wrap(marshalErr, "ReadResource: failed to marshal authenticated status.")
			return emptyResult, cgerr.NewResourceError("Failed to marshal authenticated status.", wrappedErr, nil)
		}
		statusJSON = string(jsonBytes)
	}

	// Construct the spec-compliant ReadResourceResult.
	mimeType := AuthResourceMimeType
	result := definitions.ReadResourceResult{
		Contents: []definitions.ResourceContents{
			{
				URI:      AuthResourceURI, // URI of the resource content being returned.
				MimeType: &mimeType,       // Optional pointer to the MIME type.
				Text:     &statusJSON,     // Pointer to the JSON string content.
				// Blob field is omitted as this is text content.
			},
		},
		// Meta field omitted as we have no metadata to add.
	}

	return result, nil
}

// checkExistingToken attempts to load and verify a token.
// Returns a map representing the status on success, or an error.
func (p *AuthProvider) checkExistingToken(ctx context.Context) (map[string]interface{}, error) {
	token, err := p.storage.LoadToken()
	if err != nil {
		logger.Warn("Error loading RTM token from storage.", "path", p.storage.TokenPath, "error", fmt.Sprintf("%+v", err))
		return nil, cgerr.NewAuthError(
			"Failed to load token from storage.",
			errors.Wrap(err, "checkExistingToken: failed loading token."),
			map[string]interface{}{"token_path": p.storage.TokenPath},
		)
	}

	if token != "" {
		p.client.SetAuthToken(token)
		// Pass context to the API client call - assumes client method CheckTokenCtx exists.
		auth, checkErr := p.client.CheckTokenCtx(ctx)
		if checkErr == nil && auth != nil && auth.Auth != nil && auth.Auth.User != nil {
			// Token is valid. Prepare the success status map.
			statusMap := map[string]interface{}{
				"status":      "authenticated",
				"username":    auth.Auth.User.Username, // Corrected: Access nested field.
				"fullname":    auth.Auth.User.Fullname, // Corrected: Access nested field.
				"permissions": auth.Auth.Perms,         // Corrected: Access nested field.
			}
			// Corrected: Use nested field in log message.
			logger.Info("Existing valid RTM token confirmed.", "user", auth.Auth.User.Username)
			return statusMap, nil
		} else if checkErr == nil {
			// Handle case where check succeeded but response format was invalid (missing Auth/User).
			logger.Error("RTM CheckToken response status ok but missing expected auth/user data.", "response", fmt.Sprintf("%+v", auth))
			return nil, cgerr.NewRTMError(0, "Invalid checkToken response format from RTM.", nil, nil)
		}

		// Token check failed (invalid, expired, or context cancelled).
		if ctx.Err() != nil {
			logger.Warn("Context cancelled during RTM token check.", "error", ctx.Err())
			// Return a context-specific error.
			return nil, errors.Wrap(ctx.Err(), "checkExistingToken: context cancelled during token check.")
		}
		logger.Info("Existing RTM token found but is invalid, proceeding as unauthorized.", "check_error", fmt.Sprintf("%+v", checkErr))
		// Fall through to return 'No valid token found' error below.
	}

	// If no token was loaded, or the loaded token was invalid.
	return nil, cgerr.NewAuthError(
		"No valid token found.", // Specific message checked in ReadResource.
		nil,                     // No underlying Go error, this represents a state.
		map[string]interface{}{"token_path": p.storage.TokenPath},
	)
}

// --- Tool Implementation Placeholder ---

// NOTE: The following functions (`handleFrobAuthentication` and `startNewAuthFlow`)
// contain logic that is NOT suitable for `ReadResource`. They should be adapted
// and exposed as MCP Tools by having AuthProvider also implement the `mcp.ToolProvider`
// interface and registering tool handlers (e.g., for "rtm_initiate_auth" and "rtm_complete_auth").

/*
// Example of how startNewAuthFlow could be adapted for a Tool's Call function.
func (p *AuthProvider) callInitiateAuthTool(ctx context.Context, args map[string]interface{}) (definitions.CallToolResult, error) {
    // 1. Parse permissions from 'args'. Default if necessary.
    permsArg, _ := args["perms"].(string)
    perms := PermDelete
    if permsArg == PermRead || permsArg == PermWrite || permsArg == PermDelete {
        perms = permsArg
    } else if permsArg != "" {
         logger.Warn("Invalid 'perms' argument provided for initiate auth tool, defaulting.", "provided_perms", permsArg)
    }

    // 2. Get Frob from RTM API.
    frob, err := p.client.GetFrobCtx(ctx) // Assume context-aware client method.
    if err != nil {
         // Handle error, return appropriate CallToolResult with isError: true or Go error.
         // ... Example:
         // textContent := fmt.Sprintf("Error initiating RTM auth: %v", err)
         // isError := true
         // return definitions.CallToolResult{ Content: []definitions.ToolResultContent{{Type: "text", Text: &textContent}}, IsError: &isError }, nil
    }

    // 3. Generate Auth URL.
    authURL := p.client.GetAuthURL(frob, perms)

    // 4. Construct success CallToolResult containing auth_url and frob.
    resultMap := map[string]interface{}{
        "auth_url": authURL,
        "frob":     frob,
        "permissions_requested": perms,
        "instructions": "Visit the auth_url, then use the rtm_complete_auth tool with the frob.",
    }
    jsonBytes, marshalErr := json.MarshalIndent(resultMap, "", "  ")
    if marshalErr != nil {
         // Handle marshal error - likely return Go error.
         return definitions.CallToolResult{}, errors.Wrap(marshalErr, "failed to marshal initiate auth result.")
    }
    textContent := string(jsonBytes)
    isError := false
    return definitions.CallToolResult{
        Content: []definitions.ToolResultContent{
            // Use corrected ToolResultContent structure.
            {Type: "text", Text: &textContent},
        },
        IsError: &isError,
    }, nil
}

// Example of how handleFrobAuthentication could be adapted for a Tool's Call function.
func (p *AuthProvider) callCompleteAuthTool(ctx context.Context, args map[string]interface{}) (definitions.CallToolResult, error) {
    // 1. Parse 'frob' from args. Handle missing/invalid frob.
    frob, ok := args["frob"].(string)
    if !ok || frob == "" {
        // Handle error: return CallToolResult with isError: true or Go error.
        // ... Example:
         textContent := "Missing or invalid 'frob' argument for rtm_complete_auth tool."
         isError := true
         return definitions.CallToolResult{ Content: []definitions.ToolResultContent{{Type: "text", Text: &textContent}}, IsError: &isError }, nil
    }

    // 2. Exchange frob for token.
    auth, err := p.client.GetTokenCtx(ctx, frob) // Assume context-aware client method.
    if err != nil {
        // Handle RTM API error, return CallToolResult with isError: true or Go error.
        // ... Example:
         textContent := fmt.Sprintf("Error completing RTM auth with frob: %v", err)
         isError := true
         return definitions.CallToolResult{ Content: []definitions.ToolResultContent{{Type: "text", Text: &textContent}}, IsError: &isError }, nil
    }

    // 3. Save token. Handle save errors (maybe log, but proceed).
     if saveErr := p.storage.SaveToken(auth.Auth.Token); saveErr != nil { // Assuming Auth nested field contains Token.
         logger.Error("Failed to save token after frob exchange.", "error", fmt.Sprintf("%+v", saveErr))
     }

    // 4. Set token on client instance.
    p.client.SetAuthToken(auth.Auth.Token) // Assuming Auth nested field contains Token.

    // 5. Construct success CallToolResult with authenticated status.
    // Corrected: Ensure Auth and User are not nil before accessing.
    var username, fullname, perms string
    if auth.Auth != nil {
        perms = auth.Auth.Perms
        if auth.Auth.User != nil {
             username = auth.Auth.User.Username
             fullname = auth.Auth.User.Fullname
        }
    }
    resultMap := map[string]interface{}{
        "status":      "authenticated",
        "username":    username,
        "fullname":    fullname,
        "permissions": perms,
    }
    jsonBytes, marshalErr := json.MarshalIndent(resultMap, "", "  ")
        if marshalErr != nil {
         // Handle marshal error - likely return Go error.
         return definitions.CallToolResult{}, errors.Wrap(marshalErr, "failed to marshal complete auth result.")
    }
    textContent := string(jsonBytes)
    isError := false
    return definitions.CallToolResult{
        Content: []definitions.ToolResultContent{
            // Use corrected ToolResultContent structure.
            {Type: "text", Text: &textContent},
        },
        IsError: &isError,
    }, nil
}
*/
