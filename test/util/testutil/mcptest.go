// file: test/util/testutil/mcptest.go
// Package mcptest provides testing utilities for MCP protocol testing.
package mcptest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	validators "github.com/dkoosis/cowgnition/test/validators/mcp"
)

// ReadResource sends a read_resource request to the MCP server.
// Returns the resource content and MIME type if successful, empty strings otherwise.
// It constructs a read_resource request to the MCP server's specified baseURL.
// The resourceName parameter is URL-escaped to ensure safe inclusion in the request.
// If the request is successful (HTTP status 200), it parses the JSON response to extract the resource content and MIME type.
// Empty strings are returned if the request fails or if content/MIME type extraction encounters an error.
func ReadResource(t *testing.T, client *http.Client, baseURL, resourceName string) (string, string) {
	t.Helper()

	urlPath := fmt.Sprintf("%s/mcp/read_resource?name=%s",
		baseURL, url.QueryEscape(resourceName))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, urlPath, nil)
	if err != nil {
		t.Errorf("Failed to create read_resource request: %v.", err)
		return "", ""
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Errorf("Failed to send read_resource request: %v.", err)
		return "", ""
	}
	defer resp.Body.Close()

	// Check status code.
	if resp.StatusCode != http.StatusOK {
		t.Logf("read_resource failed with status: %d.", resp.StatusCode)
		return "", ""
	}

	// Check response structure.
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Errorf("Failed to decode read_resource response: %v.", err)
		return "", ""
	}

	// Extract content and mimeType.
	content, _ := result["content"].(string)
	mimeType, _ := result["mime_type"].(string)

	return content, mimeType
}

// ValidateResourceResponse validates a response from read_resource.
// It leverages the centralized validator function `validators.ValidateResourceResponse` to perform the validation.
func ValidateResourceResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Use the centralized validator function
	return validators.ValidateResourceResponse(t, response)
}

// ValidateToolResponse validates a response from call_tool.
// It uses the `validators.ValidateToolResponse` function to validate the tool call response.
func ValidateToolResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Use the centralized validator function
	return validators.ValidateToolResponse(t, response)
}

// IsServerAuthenticated checks if the server is authenticated.
// It attempts to access a protected resource ("tasks://all") to determine authentication status.
// If the server returns a 200 OK status, it implies the server is authenticated.
func IsServerAuthenticated(t *testing.T, client *http.Client, baseURL string) bool {
	t.Helper()

	// Try to access an authenticated resource.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		baseURL+"/mcp/read_resource?name="+url.QueryEscape("tasks://all"), nil)
	if err != nil {
		t.Logf("Error creating request: %v.", err)
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Logf("Error sending request: %v.", err)
		return false
	}
	defer resp.Body.Close()

	// If we can access tasks, the server is authenticated.
	return resp.StatusCode == http.StatusOK
}

// DocEnhanced: 2025-03-25
