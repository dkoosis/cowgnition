// file: test/conformance/mcp/errors_test.go
// Package conformance provides tests to verify MCP protocol compliance.
package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
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

// TestMCPErrorResponses tests error handling and response formatting in the MCP server.
// This verifies that the server properly handles and formats error responses
// according to the MCP specification.
func TestMCPErrorResponses(t *testing.T) {
	// Create a test configuration.
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name: "Error Test Server",
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

	// Setup basic RTM responses.
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

	// Section 1: Test error responses for /mcp/initialize endpoint.
	t.Run("InitializeEndpointErrors", func(t *testing.T) {
		// Test cases for initialization errors.
		testCases := []struct {
			name          string
			method        string
			body          string
			contentType   string
			wantStatus    int
			validateError func(t *testing.T, response map[string]interface{})
		}{
			{
				name:        "Method Not Allowed",
				method:      http.MethodGet,
				body:        `{"server_name": "Test Client", "server_version": "1.0.0"}`,
				contentType: "application/json",
				wantStatus:  http.StatusMethodNotAllowed,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validators.ValidateStandardErrorResponse(t, response, http.StatusMethodNotAllowed)
				},
			},
			{
				name:        "Malformed JSON",
				method:      http.MethodPost,
				body:        `{"server_name": "Test Client", "server_version": "1.0.0", bad_json}`,
				contentType: "application/json",
				wantStatus:  http.StatusBadRequest,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validators.ValidateStandardErrorResponse(t, response, http.StatusBadRequest)
					validators.ValidateErrorFieldExists(t, response, "error")
				},
			},
			{
				name:        "Wrong Content Type",
				method:      http.MethodPost,
				body:        `{"server_name": "Test Client", "server_version": "1.0.0"}`,
				contentType: "text/plain",
				wantStatus:  http.StatusBadRequest,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validators.ValidateStandardErrorResponse(t, response, http.StatusBadRequest)
				},
			},
			{
				name:        "Empty Body",
				method:      http.MethodPost,
				body:        ``,
				contentType: "application/json",
				wantStatus:  http.StatusBadRequest,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validators.ValidateStandardErrorResponse(t, response, http.StatusBadRequest)
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				req, err := http.NewRequestWithContext(ctx, tc.method, client.BaseURL+"/mcp/initialize", strings.NewReader(tc.body))
				if err != nil {
					t.Fatalf("Failed to create request: %v", err)
				}
				if tc.contentType != "" {
					req.Header.Set("Content-Type", tc.contentType)
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

				// Parse response.
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Failed to read response body: %v", err)
				}

				if len(body) == 0 {
					t.Error("Response body is empty")
					return
				}

				var response map[string]interface{}
				if err := json.Unmarshal(body, &response); err != nil {
					t.Errorf("Failed to parse response JSON: %v\nBody: %s", err, string(body))
					return
				}

				// Validate error response.
				if tc.validateError != nil {
					tc.validateError(t, response)
				}
			})
		}
	})

	// Section 2: Test error responses for /mcp/read_resource endpoint.
	t.Run("ReadResourceEndpointErrors", func(t *testing.T) {
		// Test cases for read_resource errors.
		testCases := []struct {
			name          string
			method        string
			resourceName  string
			wantStatus    int
			validateError func(t *testing.T, response map[string]interface{})
		}{
			{
				name:         "Missing Resource Name",
				method:       http.MethodGet,
				resourceName: "",
				wantStatus:   http.StatusBadRequest,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validators.ValidateStandardErrorResponse(t, response, http.StatusBadRequest)
					validators.ValidateErrorMessage(t, response, "Missing resource name")
				},
			},
			{
				name:         "Method Not Allowed",
				method:       http.MethodPost,
				resourceName: "auth://rtm",
				wantStatus:   http.StatusMethodNotAllowed,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validators.ValidateStandardErrorResponse(t, response, http.StatusMethodNotAllowed)
				},
			},
			{
				name:         "Resource Not Found",
				method:       http.MethodGet,
				resourceName: "invalid://resource",
				wantStatus:   http.StatusNotFound,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validators.ValidateStandardErrorResponse(t, response, http.StatusNotFound)
					validators.ValidateErrorMessage(t, response, "Resource not found")
				},
			},
			{
				name:         "Non-standard Resource Name Format",
				method:       http.MethodGet,
				resourceName: "not-a-valid-resource-name",
				wantStatus:   http.StatusNotFound,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validators.ValidateStandardErrorResponse(t, response, http.StatusNotFound)
				},
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
					t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, tc.wantStatus)
				}

				// Parse response.
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Failed to read response body: %v", err)
				}

				if len(body) == 0 {
					t.Error("Response body is empty")
					return
				}

				var response map[string]interface{}
				if err := json.Unmarshal(body, &response); err != nil {
					t.Errorf("Failed to parse response JSON: %v\nBody: %s", err, string(body))
					return
				}

				// Validate error response.
				if tc.validateError != nil {
					tc.validateError(t, response)
				}
			})
		}
	})

	// Additional test sections...
}

// TestMCPMalformedRequests tests the handling of malformed requests.
func TestMCPMalformedRequests(t *testing.T) {
	// Create a test configuration.
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name: "Malformed Test Server",
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

	// Setup basic RTM responses.
	rtmMock.AddResponse("rtm.auth.getFrob", `<rsp stat="ok"><frob>test_frob_12345</frob></rsp>`)

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

	// Test malformed URLs.
	t.Run("MalformedURLs", func(t *testing.T) {
		// Test implementation...
	})

	// Test oversized request bodies.
	t.Run("OversizedBodies", func(t *testing.T) {
		// Test implementation...
	})

	// Test malformed content types.
	t.Run("MalformedContentTypes", func(t *testing.T) {
		// Test implementation...
	})
}
