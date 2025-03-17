// Package conformance provides tests to verify MCP protocol compliance.
package conformance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/helpers"
	"github.com/cowgnition/cowgnition/test/mocks"
)

// TestMCPResourceEndpointsEnhanced provides comprehensive testing of the
// resource-related endpoints for MCP protocol compliance.
func TestMCPResourceEndpointsEnhanced(t *testing.T) {
	// Create a test configuration.
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name: "Test MCP Server",
			Port: 8080,
		},
		RTM: config.RTMConfig{
			APIKey:       "test_key",
			SharedSecret: "test_secret",
		},
		Auth: config.AuthConfig{
			TokenPath: t.TempDir() + "/token",
		},
	}

	// Create a mock RTM server with proper responses.
	rtmMock := mocks.NewRTMServer(t)

	// Configure the mock to handle auth appropriately.
	rtmMock.AddResponse("rtm.auth.getFrob", `<rsp stat="ok"><frob>test_frob_12345</frob></rsp>`)
	rtmMock.AddResponse("rtm.auth.getToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)
	rtmMock.AddResponse("rtm.auth.checkToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)

	defer rtmMock.Close()

	// Override RTM API endpoint in client.
	if err := os.Setenv("RTM_API_ENDPOINT", rtmMock.BaseURL); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer os.Unsetenv("RTM_API_ENDPOINT")

	// Create and start the MCP server.
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create MCP test client.
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// Test list_resources endpoint.
	t.Run("list_resources", func(t *testing.T) {
		// Test cases to verify different aspects of the list_resources endpoint.
		testCases := struct {
			name       string
			method     string
			wantStatus int
		}{
			{
				name:       "Valid request",
				method:     http.MethodGet,
				wantStatus: http.StatusOK,
			},
			{
				name:       "Invalid method - POST",
				method:     http.MethodPost,
				wantStatus: http.StatusMethodNotAllowed,
			},
			{
				name:       "Invalid method - PUT",
				method:     http.MethodPut,
				wantStatus: http.StatusMethodNotAllowed,
			},
			{
				name:       "Invalid method - DELETE",
				method:     http.MethodDelete,
				wantStatus: http.StatusMethodNotAllowed,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				req, err := http.NewRequestWithContext(ctx, tc.method, client.BaseURL+"/mcp/list_resources", nil)
				if err != nil {
					t.Fatalf("Failed to create request: %v", err)
				}

				resp, err := client.Client.Do(req)
				if err != nil {
					t.Fatalf("Failed to send request: %v", err)
				}
				defer resp.Body.Close()

				// Verify status code.
				if resp.StatusCode != tc.wantStatus {
					t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, tc.wantStatus)
				}

				// For successful responses, validate the structure.
				if resp.StatusCode == http.StatusOK {
					var result map[string]interface{}
					if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
						t.Fatalf("Failed to decode response: %v", err)
					}

					// Validate resources array.
					validateListResourcesResponse(t, result)
				}
			})
		}
	})

	// Test read_resource endpoint.
	t.Run("read_resource", func(t *testing.T) {
		// Test cases to verify different aspects of the read_resource endpoint.
		testCases := struct {
			name         string
			method       string
			resourceName string
			wantStatus   int
		}{
			{
				name:         "Auth resource",
				method:       http.MethodGet,
				resourceName: "auth://rtm",
				wantStatus:   http.StatusOK,
			},
			{
				name:         "Missing resource name",
				method:       http.MethodGet,
				resourceName: "",
				wantStatus:   http.StatusBadRequest,
			},
			{
				name:         "Invalid resource name",
				method:       http.MethodGet,
				resourceName: "invalid-resource-name",
				wantStatus:   http.StatusNotFound,
			},
			{
				name:         "Non-existent resource",
				method:       http.MethodGet,
				resourceName: "nonexistent://resource",
				wantStatus:   http.StatusNotFound,
			},
			{
				name:         "Invalid method - POST",
				method:       http.MethodPost,
				resourceName: "auth://rtm",
				wantStatus:   http.StatusMethodNotAllowed,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				urlPath := client.BaseURL + "/mcp/read_resource"
				if tc.resourceName != "" {
					urlPath += "?name=" + url.QueryEscape(tc.resourceName)
				}

				req, err := http.NewRequestWithContext(ctx, tc.method, urlPath, nil)
				if err != nil {
					t.Fatalf("Failed to create request: %v", err)
				}

				resp, err := client.Client.Do(req)
				if err != nil {
					t.Fatalf("Failed to send request: %v", err)
				}
				defer resp.Body.Close()

				// Verify status code.
				if resp.StatusCode != tc.wantStatus {
					body, _ := io.ReadAll(resp.Body)
					t.Errorf("Unexpected status code: got %d, want %d. Body: %s",
						resp.StatusCode, tc.wantStatus, string(body))
					return
				}

				// For successful responses, validate the structure.
				if resp.StatusCode == http.StatusOK {
					var result map[string]interface{}
					if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
						t.Fatalf("Failed to decode response: %v", err)
					}

					// Validate resource response.
					if !validateResourceResponse(t, result) {
						t.Errorf("Resource response validation failed")
					}
				}
			})
		}
	})

	// Test error responses.
	t.Run("error_responses", func(t *testing.T) {
		// Test malformed URL.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Deliberately include an invalid URL query parameter.
		malformedURL := client.BaseURL + "/mcp/read_resource?name=auth://rtm&invalid=%"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, malformedURL, nil)
		if err != nil {
			// Expected error due to malformed URL.
			return
		}

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()
		// We expect an error response.
		if resp.StatusCode == http.StatusOK {
			t.Errorf("Expected error status for malformed URL, got %d", resp.StatusCode)
		}
	})
}

// validateListResourcesResponse validates the response from list_resources.
func validateListResourcesResponse(t *testing.T, result map[string]interface{}) {
	t.Helper()

	// Check for resources field.
	resources, ok := result["resources"].(interface{})
	if !ok {
		t.Errorf("resources is not an array: %v", result["resources"])
		return
	}

	// At minimum, we should have at least one resource (auth://rtm).
	if len(resources) < 1 {
		t.Error("Expected at least one resource")
		return
	}

	// Validate each resource.
	for i, res := range resources {
		if !validateMCPResource(t, res) {
			t.Errorf("Resource %d failed validation", i)
		}
	}

	// Check for auth resource specifically.
	authResourceFound := false
	for _, res := range resources {
		resource, ok := res.(map[string]interface{})
		if !ok {
			continue
		}

		if name, ok := resource["name"].(string); ok && name == "auth://rtm" {
			authResourceFound = true
			break
		}
	}

	if !authResourceFound {
		t.Error("auth://rtm resource not found in list_resources response")
	}
}

// validateMCPResource validates the structure of a single MCP resource.
func validateMCPResource(t *testing.T, res interface{}) bool {
	t.Helper()

	resource, ok := res.(map[string]interface{})
	if !ok {
		t.Errorf("Resource is not a map: %v", res)
		return false
	}

	// Check for required fields like "name", "type", etc.
	if _, ok := resource["name"].(string); !ok {
		t.Error("Resource missing 'name' field or wrong type")
		return false
	}

	if _, ok := resource["type"].(string); !ok {
		t.Error("Resource missing 'type' field or wrong type")
		return false
	}

	return true
}

// validateResourceResponse validates the response from read_resource.
// NOTE: You need to implement the actual validation logic here based on
// the expected structure of a resource response.
func validateResourceResponse(t *testing.T, result map[string]interface{}) bool {
	t.Helper()
	// Add your validation logic here. This is a placeholder.
	return true
}
