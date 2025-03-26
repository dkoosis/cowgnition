// internal/server/middleware/auth.go
package middleware

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dkoosis/cowgnition/internal/server/httputils"
)

// AuthHandler holds methods for authentication handling
type AuthHandler struct {
	Server httputils.ServerInterface
}

// NewAuthHandler creates a new auth handler with the given server
func NewAuthHandler(s httputils.ServerInterface) *AuthHandler {
	return &AuthHandler{Server: s}
}

// HandleAuthResource handles the auth://rtm resource
func (h *AuthHandler) HandleAuthResource(w http.ResponseWriter, r *http.Request) {
	if h.Server.GetRTMService().IsAuthenticated() {
		// Already authenticated
		response := h.formatAuthSuccessResponse()
		httputils.WriteJSONResponse(w, http.StatusOK, response)
		return
	}

	// Start authentication flow
	authURL, frob, err := h.Server.GetRTMService().StartAuthFlow()
	if err != nil {
		log.Printf("Error starting auth flow: %v", err)
		httputils.WriteStandardErrorResponse(w, httputils.InternalError,
			fmt.Sprintf("Error starting authentication flow: %v", err),
			map[string]interface{}{
				"component": "rtm_service",
				"function":  "StartAuthFlow",
			})
		return
	}

	// Return auth URL and instructions
	content := h.formatAuthInstructions(authURL, frob)

	response := map[string]interface{}{
		"content":   content,
		"mime_type": "text/markdown",
	}

	httputils.WriteJSONResponse(w, http.StatusOK, response)
}

// formatAuthSuccessResponse creates a rich response for successful authentication.
func (h *AuthHandler) formatAuthSuccessResponse() map[string]interface{} {
	// Create formatted content with emoji and rich formatting
	content := `# ✅ Authentication Successful

You are already authenticated with Remember The Milk. You can now:

- View task lists with resources like ` + "`tasks://today`" + ` or ` + "`lists://all`" + `
- Create new tasks using the ` + "`add_task`" + ` tool
- Create new tasks using the ` + "`complete_task`" + ` and ` + "`set_due_date`" + `

**Example**: Ask "What tasks are due today?" or "Add milk to my shopping list"
`

	return map[string]interface{}{
		"content":   content,
		"mime_type": "text/markdown",
	}
}

// formatAuthInstructions creates rich instructions for the authentication flow.
func (h *AuthHandler) formatAuthInstructions(authURL, frob string) string {
	return fmt.Sprintf(`# 🔑 Remember The Milk Authentication

To connect Claude with your Remember The Milk account, please follow these steps:

## Step 1: Authorize CowGnition

[Click here to authorize](%s) or copy this URL into your browser:

%s

## Step 2: Complete Authentication

After authorizing, return here and run this command:

> Use the authenticate tool with frob %s

## What's happening?

This secure authentication process uses Remember The Milk's OAuth-like flow. CowGnition never sees your RTM password.

---

**Technical details**: Using frob %s (valid for 24 hours)
`, authURL, authURL, frob, frob)
}

// HandleAuthenticationTool handles the authenticate tool.
// This completes the RTM authentication flow.
func (h *AuthHandler) HandleAuthenticationTool(w http.ResponseWriter, args map[string]interface{}) {
	if h.Server.GetRTMService().IsAuthenticated() {
		httputils.WriteJSONResponse(w, http.StatusOK, map[string]interface{}{
			"result": "✅ You're already authenticated with Remember The Milk! You can use all features now.",
		})
		return
	}

	// Get frob from arguments
	frob, ok := args["frob"].(string)
	if !ok || frob == "" {
		httputils.WriteStandardErrorResponse(w, httputils.InvalidParams,
			"Missing or invalid 'frob' argument. Please provide the 'frob' value from the authentication URL.",
			map[string]interface{}{
				"required_parameter": "frob",
				"parameter_type":     "string",
			})
		return
	}

	// Complete authentication flow
	if err := h.Server.GetRTMService().CompleteAuthFlow(frob); err != nil {
		log.Printf("Error completing auth flow: %v", err)

		// Check for specific errors to provide more helpful messages
		errMsg := err.Error()
		if strings.Contains(errMsg, "expired") {
			httputils.WriteStandardErrorResponse(w, httputils.AuthError,
				"Authentication flow expired. Please initiate a new authentication process by accessing the auth://rtm resource again.",
				map[string]interface{}{
					"error_type":    "expired_flow",
					"auth_resource": "auth://rtm",
				})
			return
		}

		if strings.Contains(errMsg, "invalid frob") {
			httputils.WriteStandardErrorResponse(w, httputils.InvalidParams,
				"Invalid 'frob' value provided. Ensure you are using the 'frob' from the most recent authentication attempt.",
				map[string]interface{}{
					"error_type": "invalid_frob",
					"parameter":  "frob",
				})
			return
		}

		httputils.WriteStandardErrorResponse(w, httputils.RTMServiceError,
			fmt.Sprintf("Authentication failed: %v. Please try starting the authentication process again.", err),
			map[string]interface{}{
				"component":     "rtm_service",
				"function":      "CompleteAuthFlow",
				"auth_resource": "auth://rtm",
			})
		return
	}

	// Success!
	successMsg := `# ✅ Authentication Successful!

Your Remember The Milk account is now connected to Claude. You can now:

- **View tasks** - Ask about today's tasks, upcoming deadlines, or browse specific lists
- **Create tasks** - Add new items to any list
- **Manage tasks** - Complete tasks, set due dates, add tags, and more

Try asking about your tasks or creating a new one!`

	httputils.WriteJSONResponse(w, http.StatusOK, map[string]interface{}{
		"result": successMsg,
	})
}

// HandleLogoutTool handles the logout tool.
// This removes the stored authentication token.
func (h *AuthHandler) HandleLogoutTool(args map[string]interface{}) (string, error) {
	// Check if confirmation is provided
	confirm, _ := args["confirm"].(bool)
	if !confirm {
		return "To log out from Remember The Milk, please execute this tool with `confirm: true` to confirm the logout action.", nil
	}

	// Clear authentication
	if err := h.Server.GetRTMService().ClearAuthentication(); err != nil {
		return "", fmt.Errorf("error logging out: %w", err)
	}

	return "You have been successfully logged out from Remember The Milk. To reconnect, access the auth://rtm resource.", nil
}

// HandleAuthStatusTool provides information about the current authentication status.
func (h *AuthHandler) HandleAuthStatusTool(_ map[string]interface{}) (string, error) {
	var result strings.Builder

	result.WriteString("# Remember The Milk Authentication Status\n\n")

	if h.Server.GetRTMService().IsAuthenticated() {
		result.WriteString("✅ **Status:** Authenticated\n\n")

		// Get token info if possible
		if h.Server.GetTokenManager() != nil && h.Server.GetTokenManager().HasToken() {
			if fileInfo, err := h.Server.GetTokenManager().GetTokenFileInfo(); err == nil {
				result.WriteString(fmt.Sprintf("- **Last authenticated:** %s\n",
					fileInfo.ModTime().Format(time.RFC1123)))
			}
		}

		result.WriteString("\nYou can use all Remember The Milk features through Claude.")
	} else {
		result.WriteString("❌ **Status:** Not authenticated\n\n")

		// Check if there's a pending auth flow
		if h.Server.GetRTMService().GetActiveAuthFlows() > 0 {
			result.WriteString("There is a pending authentication flow. Please complete it or start a new one.\n\n")
		}

		result.WriteString("To authenticate, please access the `auth://rtm` resource.")
	}

	return result.String(), nil
}
