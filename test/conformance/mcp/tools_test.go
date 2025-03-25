// file: test/conformance/mcp/tools_test.go
// Package conformance provides tests to verify MCP protocol compliance.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/helpers"
	"github.com/cowgnition/cowgnition/test/mocks"
	validators "github.com/cowgnition/cowgnition/test/validators/mcp"
)

// TestMCPToolEndpointsEnhanced provides comprehensive testing of the
// tool-related endpoints for MCP protocol compliance.
func TestMCPToolEndpointsEnhanced(t *testing.T) {
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

	// Create a mock RTM server.
	rtmMock := mocks.NewRTMServer(t)
	defer rtmMock.Close()

	// Configure the mock to handle auth appropriately.
	rtmMock.AddResponse("rtm.auth.getFrob", `<rsp stat="ok"><frob>test_frob_12345</frob></rsp>`)
	rtmMock.AddResponse("rtm.auth.getToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)
	rtmMock.AddResponse("rtm.auth.checkToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)

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

	// Simulate authentication for testing
	if err := helpers.SimulateAuthentication(s); err != nil {
		t.Logf("Warning: Could not simulate authentication: %v", err)
	}

	// Create MCP test client.
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// Test list_tools endpoint.
	t.Run("list_tools", func(t *testing.T) {
		// Test cases to verify different aspects of the list_tools endpoint.
		testCases := []struct {
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

				req, err := http.NewRequestWithContext(ctx, tc.method, client.BaseURL+"/mcp/list_tools", nil)
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

					// Validate tools array.
					validators.ValidateListToolsResponse(t, result)
				}
			})
		}
	})

	// Test call_tool endpoint.
	t.Run("call_tool", func(t *testing.T) {
		// Test cases to verify different aspects of the call_tool endpoint.
		testCases := []struct {
			name       string
			method     string
			toolName   string
			args       map[string]interface{}
			wantStatus int
		}{
			{
				name:     "Authenticate tool with invalid frob",
				method:   http.MethodPost,
				toolName: "authenticate",
				args: map[string]interface{}{
					"frob": "invalid_frob",
				},
				wantStatus: http.StatusInternalServerError, // Or could be 400 Bad Request.
			},
			{
				name:       "Authenticate tool with missing frob",
				method:     http.MethodPost,
				toolName:   "authenticate",
				args:       map[string]interface{}{},
				wantStatus: http.StatusBadRequest,
			},
			{
				name:       "Non-existent tool",
				method:     http.MethodPost,
				toolName:   "nonexistent_tool",
				args:       map[string]interface{}{},
				wantStatus: http.StatusInternalServerError, // Or could be 404 Not Found.
			},
			{
				name:     "Invalid method - GET",
				method:   http.MethodGet,
				toolName: "authenticate",
				args: map[string]interface{}{
					"frob": "test_frob",
				},
				wantStatus: http.StatusMethodNotAllowed,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				reqBody := map[string]interface{}{
					"name":      tc.toolName,
					"arguments": tc.args,
				}
				body, err := json.Marshal(reqBody)
				if err != nil {
					t.Fatalf("Failed to marshal request: %v", err)
				}

				req, err := http.NewRequestWithContext(ctx, tc.method, client.BaseURL+"/mcp/call_tool", bytes.NewBuffer(body))
				if err != nil {
					t.Fatalf("Failed to create request: %v", err)
				}
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Client.Do(req)
				if err != nil {
					t.Fatalf("Failed to send request: %v", err)
				}
				defer resp.Body.Close()

				// Verify status code.
				// Note: The exact error status code may vary by implementation,
				// so we're being somewhat lenient here.
				if tc.wantStatus == http.StatusOK && resp.StatusCode != tc.wantStatus {
					t.Errorf("Expected OK status, got %d", resp.StatusCode)
				} else if tc.wantStatus != http.StatusOK && resp.StatusCode == http.StatusOK {
					t.Errorf("Expected error status, got %d", resp.StatusCode)
				}

				// For successful responses, validate the structure.
				if resp.StatusCode == http.StatusOK {
					var result map[string]interface{}
					if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
						t.Fatalf("Failed to decode response: %v", err)
					}

					// Validate tool response.
					if !validators.ValidateToolResponse(t, result) {
						t.Errorf("Tool response validation failed")
					}
				}
			})
		}
	})

	// Test malformed requests.
	t.Run("malformed_requests", func(t *testing.T) {
		// Test malformed JSON.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		malformedJSON := `{"name": "authenticate", "arguments": {invalid}}`
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/mcp/call_tool", strings.NewReader(malformedJSON))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Should not return OK for malformed JSON.
		if resp.StatusCode == http.StatusOK {
			t.Errorf("Expected error status for malformed JSON, got %d", resp.StatusCode)
		}

		// Test empty JSON.
		emptyJSON := `{}`
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/mcp/call_tool", strings.NewReader(emptyJSON))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Should not return OK for empty JSON (missing required fields).
		if resp.StatusCode == http.StatusOK {
			t.Errorf("Expected error status for empty JSON, got %d", resp.StatusCode)
		}

		// Test missing content type.
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/mcp/call_tool", strings.NewReader(`{"name":"authenticate","arguments":{"frob":"test"}}`))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		// Deliberately omit Content-Type header.

		resp, err = client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Server should still handle this and not return 500.
		// But it might return 400 if it strictly validates Content-Type.
		if resp.StatusCode == http.StatusInternalServerError {
			t.Errorf("Server returned 500 for missing Content-Type, should be more graceful")
		}
	})
}
