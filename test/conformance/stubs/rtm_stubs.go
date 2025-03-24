// Package stubs provides test utility functions for the RTM client.
package helpers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/helpers"
)

// SetAuthTokenOnServer attempts to set the authentication token directly on the server's RTM service.
// This uses reflection for testing purposes.
func SetAuthTokenOnServer(s *server.MCPServer, token string) error {
	// Get RTM service
	rtmService := s.GetRTMService()
	if rtmService == nil {
		return fmt.Errorf("failed to get RTM service from server")
	}

	// Use reflection to access the RTM service client
	val := reflect.ValueOf(rtmService).Elem()
	clientField := val.FieldByName("client")
	if !clientField.IsValid() || !clientField.CanInterface() {
		return fmt.Errorf("cannot access client field on RTM service")
	}

	// Get the client
	client := clientField.Interface()
	clientVal := reflect.ValueOf(client)

	// Find the SetAuthToken method
	setTokenMethod := clientVal.MethodByName("SetAuthToken")
	if !setTokenMethod.IsValid() {
		return fmt.Errorf("client has no SetAuthToken method")
	}

	// Call SetAuthToken with the token
	setTokenMethod.Call([]reflect.Value{reflect.ValueOf(token)})

	// Also set the authStatus field to authenticated if possible
	authStatusField := val.FieldByName("authStatus")
	if authStatusField.IsValid() && authStatusField.CanSet() {
		// Status 3 is StatusAuthenticated in our RTM package
		authStatusField.SetInt(3)
	}

	return nil
}

// IsServerAuthenticated checks if the server is authenticated by testing
// if it can access authenticated resources.
func IsServerAuthenticated(ctx context.Context, client *helpers.MCPClient) bool {
	// Try to access a resource that requires authentication
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		client.BaseURL+"/mcp/read_resource?name="+url.QueryEscape("tasks://all"), nil)
	if err != nil {
		return false
	}

	resp, err := client.Client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// If we can access this resource, the server is authenticated
	return resp.StatusCode == http.StatusOK
}

// ReadResource is a stub implementation of resource reading for testing.
func ReadResource(ctx context.Context, client *helpers.MCPClient, uri string) (map[string]interface{}, error) {
	if uri == "auth://rtm" {
		fakeAuthURL := "https://www.rememberthemilk.com/services/auth/?api_key=YOUR_API_KEY&perms=delete&frob=FAKE_FROB"
		fakeContent := fmt.Sprintf("authURL=%s", fakeAuthURL)

		return map[string]interface{}{
			"content":   fakeContent,
			"mime_type": "text/plain",
		}, nil
	}
	return nil, errors.New("resource not found")
}

// findURLEndIndex locates the end of a URL within content starting from startIdx.
// This helps reduce complexity in the main function.
func findURLEndIndex(content string, startIdx int) int {
	endIdx := startIdx

	for i := startIdx; i < len(content); i++ {
		// URL ends at any whitespace or common ending punctuation
		if content[i] == '\n' || content[i] == '\r' || content[i] == ' ' ||
			content[i] == '"' || content[i] == ')' || content[i] == ']' {
			return i
		}
		endIdx = i
	}

	// If we reach end of content without finding endpoint
	return endIdx + 1
}

// extractFrobFromURL attempts to extract the frob parameter from a URL.
func extractFrobFromURL(authURL string) string {
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		return ""
	}
	return parsedURL.Query().Get("frob")
}

// extractFrobFromContent tries to find a frob value within content using known patterns.
func extractFrobFromContent(content string) string {
	// Common patterns that precede a frob in content text
	patterns := []string{
		"frob ",
		"frob: ",
		"Frob: ",
		"frob=",
		"\"frob\": \"",
	}

	for _, pattern := range patterns {
		idx := strings.Index(content, pattern)
		if idx == -1 {
			continue
		}

		startIdx := idx + len(pattern)
		endIdx := findURLEndIndex(content, startIdx)

		if endIdx > startIdx {
			return content[startIdx:endIdx]
		}
	}

	return ""
}

// ExtractAuthInfoFromContent attempts to extract auth URL and frob from content.
// It uses helper functions to reduce complexity.
func ExtractAuthInfoFromContent(content string) (string, string) {
	// Look for URL in content
	urlIdx := strings.Index(content, "https://www.rememberthemilk.com/services/auth/")
	if urlIdx == -1 {
		return "", ""
	}

	// Extract URL
	endURLIdx := findURLEndIndex(content, urlIdx)
	authURL := content[urlIdx:endURLIdx]

	// Try to extract frob, first from URL then from content text
	frob := extractFrobFromURL(authURL)

	// If frob not found in URL, look in content text
	if frob == "" {
		frob = extractFrobFromContent(content)
	}

	return authURL, frob
}

// CallTool is a stub implementation of tool calling for testing.
func CallTool(ctx context.Context, client *helpers.MCPClient, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
	if toolName == "authenticate" {
		return map[string]interface{}{
			"result": "success",
		}, nil
	}
	return nil, errors.New("tool not found")
}
