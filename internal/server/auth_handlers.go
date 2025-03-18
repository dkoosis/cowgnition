// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// handleAuthResource handles the auth://rtm resource.
// It provides authentication status and initiates the auth flow if needed.
func (s *MCPServer) handleAuthResource(w http.ResponseWriter) {
	if s.rtmService.IsAuthenticated() {
		// Already authenticated
		response := formatAuthSuccessResponse()
		writeJSONResponse(w, http.StatusOK, response)
		return
	}

	// Start authentication flow
	authURL, frob, err := s.rtmService.StartAuthFlow()
	if err != nil {
		log.Printf("Error starting auth flow: %v", err)
		writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error starting authentication flow: %v", err))
		return
	}

	// Return auth URL and instructions
	content := formatAuthInstructions(authURL, frob)

	response := map[string]interface{}{
		"content":   content,
		"mime_type": "text/markdown",
	}

	writeJSONResponse(w, http.StatusOK, response)
}

// formatAuthSuccessResponse creates a rich response for successful authentication.
func formatAuthSuccessResponse() map[string]interface{} {
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
func formatAuthInstructions(authURL, frob string) string {
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

// handleAuthenticationTool handles the authenticate tool.
// This completes the RTM authentication flow.
func (s *MCPServer) handleAuthenticationTool(w http.ResponseWriter, args map[string]interface{}) {
	if s.rtmService.IsAuthenticated() {
		writeJSONResponse(w, http.StatusOK, map[string]interface{}{
			"result": "✅ You're already authenticated with Remember The Milk! You can use all features now.",
		})
		return
	}

	// Get frob from arguments
	frob, ok := args["frob"].(string)
	if !ok || frob == "" {
		// SUGGESTION (Readability): Improve error message for missing or invalid frob.
		writeErrorResponse(w, http.StatusBadRequest, "Missing or invalid 'frob' argument. Please provide the 'frob' value from the authentication URL.")
		return
	}

	// Complete authentication flow
	if err := s.rtmService.CompleteAuthFlow(frob); err != nil {
		log.Printf("Error completing auth flow: %v", err)

		// Check for specific errors to provide more helpful messages
		errMsg := err.Error()
		if strings.Contains(errMsg, "expired") {
			// SUGGESTION (Readability): Clarify the message for an expired authentication flow.
			writeErrorResponse(w, http.StatusBadRequest,
				"Authentication flow expired. Please initiate a new authentication process by accessing the auth://rtm resource again.")
			return
		}

		if strings.Contains(errMsg, "invalid frob") {
			// SUGGESTION (Readability): Clarify the message for an invalid frob.
			writeErrorResponse(w, http.StatusBadRequest,
				"Invalid 'frob' value provided. Ensure you are using the 'frob' from the most recent authentication attempt.")
			return
		}

		writeErrorResponse(w, http.StatusInternalServerError,
			fmt.Sprintf("Authentication failed: %v. Please try starting the authentication process again.", err))
		return
	}

	// Success!
	successMsg := `# ✅ Authentication Successful!

Your Remember The Milk account is now connected to Claude. You can now:

- **View tasks** - Ask about today's tasks, upcoming deadlines, or browse specific lists
- **Create tasks** - Add new items to any list
- **Manage tasks** - Complete tasks, set due dates, add tags, and more

Try asking about your tasks or creating a new one!`

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"result": successMsg,
	})
}

// handleLogoutTool handles the logout tool.
// This removes the stored authentication token.
func (s *MCPServer) handleLogoutTool(args map[string]interface{}) (string, error) {
	// Check if confirmation is provided
	confirm, _ := args["confirm"].(bool)
	if !confirm {
		// SUGGESTION (Readability): Improve clarity of logout confirmation message.
		return "To log out from Remember The Milk, please execute this tool with `confirm: true` to confirm the logout action.", nil
	}

	// Clear authentication
	if err := s.rtmService.ClearAuthentication(); err != nil {
		return "", fmt.Errorf("error logging out: %w", err)
	}

	return "You have been successfully logged out from Remember The Milk. To reconnect, access the auth://rtm resource.", nil
}

// handleAuthStatusTool provides information about the current authentication status.
func (s *MCPServer) handleAuthStatusTool(_ map[string]interface{}) (string, error) {
	var result strings.Builder

	result.WriteString("# Remember The Milk Authentication Status\n\n")

	if s.rtmService.IsAuthenticated() {
		result.WriteString("✅ **Status:** Authenticated\n\n")

		// Get token info if possible
		if s.tokenManager != nil && s.tokenManager.HasToken() {
			if fileInfo, err := s.tokenManager.GetTokenFileInfo(); err == nil {
				result.WriteString(fmt.Sprintf("- **Last authenticated:** %s\n",
					fileInfo.ModTime().Format(time.RFC1123)))
			}
		}

		result.WriteString("\nYou can use all Remember The Milk features through Claude.")
	} else {
		result.WriteString("❌ **Status:** Not authenticated\n\n")

		// Check if there's a pending auth flow
		if s.rtmService.GetActiveAuthFlows() > 0 {
			result.WriteString("There is a pending authentication flow. Please complete it or start a new one.\n\n")
		}

		result.WriteString("To authenticate, please access the `auth://rtm` resource.")
	}

	return result.String(), nil
}

// ErrorMsgEnhanced: 2024-03-18
