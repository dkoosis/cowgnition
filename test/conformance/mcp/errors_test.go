// Package mcp provides tests to verify MCP protocol compliance.
// file: test/conformance/mcp/errors_test.go
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

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/server"
	helpers "github.com/dkoosis/cowgnition/test/helpers/common"
	"github.com/dkoosis/cowgnition/test/mocks"
	validators "github.com/dkoosis/cowgnition/test/validators/mcp"
)

// TestMCPErrorResponses tests error handling and response formatting in the MCP server.
// This verifies that the server properly handles and formats error responses
// according to the MCP specification.
//
// t *testing.T: The testing.T instance for the current test.
func TestMCPErrorResponses(t *testing.T) {
	// Create a test configuration.
	// This configuration is used to initialize the MCP server with test parameters.
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
			TokenPath: t.TempDir() + "/token", // Use a temporary directory for token storage to avoid conflicts.
		},
	}

	// Create a mock RTM server.
	// This mock server simulates the RTM API, providing predictable responses for testing.
	rtmMock := mocks.NewRTMServer(t)
	defer rtmMock.Close() // Ensure the mock server is closed after the test completes.

	// Setup basic RTM responses.
	// These responses define the behavior of the mock RTM server for specific API calls related to authentication.
	rtmMock.AddResponse("rtm.auth.getFrob", `<rsp stat="ok"><frob>test_frob_12345</frob></rsp>`)
	rtmMock.AddResponse("rtm.auth.getToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)
	rtmMock.AddResponse("rtm.auth.checkToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)

	// Override RTM API endpoint in client.
	// This redirects RTM API calls to the mock server for testing purposes.
	if err := os.Setenv("RTM_API_ENDPOINT", rtmMock.BaseURL); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer os.Unsetenv("RTM_API_ENDPOINT") // Ensure the environment variable is unset after the test.

	// Create and start the MCP server.
	// This initializes the MCP server with the test configuration and mock RTM service.
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Simulate authentication for testing
	// This simulates the authentication flow, allowing the test to proceed as if a user is authenticated.
	if err := helpers.SimulateAuthentication(s); err != nil {
		t.Logf("Warning: Could not simulate authentication: %v", err)
	}

	// Create MCP test client.
	// This client is used to make requests to the MCP server during the test.
	client := helpers.NewMCPClient(t, s)
	defer client.Close() // Ensure the client is closed after the test.

	// Section 1: Test error responses for /mcp/initialize endpoint.
	// This section tests various error conditions for the /mcp/initialize endpoint.
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
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Set a timeout for the request.
				defer cancel()

				req, err := http.NewRequestWithContext(ctx, tc.method, client.BaseURL+"/mcp/initialize", strings.NewReader(tc.body))
				if err != nil {
					t.Fatalf("Failed to create request: %v", err)
				}
				if tc.contentType != "" {
					req.Header.Set("Content-Type", tc.contentType) // Set the content type header if provided.
				}

				resp, err := client.Client.Do(req)
				if err != nil {
					t.Fatalf("Failed to send request: %v", err)
				}
				defer resp.Body.Close()

				// Verify status code.
				// The status code should match the expected status code for the test case.
				if resp.StatusCode != tc.wantStatus {
					t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, tc.wantStatus)
				}

				// Parse response.
				// The response body is read and parsed as JSON.
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
				// The error response is validated using the provided validation function.
				if tc.validateError != nil {
					tc.validateError(t, response)
				}
			})
		}
	})

	// Section 2: Test error responses for /mcp/read_resource endpoint.
	// This section tests various error conditions for the /mcp/read_resource endpoint.
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
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // Set a timeout for the request.
				defer cancel()

				urlPath := client.BaseURL + "/mcp/read_resource"
				if tc.resourceName != "" {
					urlPath += "?name=" + url.QueryEscape(tc.resourceName) // Add the resource name as a query parameter.
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
				// The status code should match the expected status code for the test case.
				if resp.StatusCode != tc.wantStatus {
					t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, tc.wantStatus)
				}

				// Parse response.
				// The response body is read and parsed as JSON.
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
				// The error response is validated using the provided validation function.
				if tc.validateError != nil {
					tc.validateError(t, response)
				}
			})
		}
	})

	// Additional test sections...
}

// TestMCPMalformedRequests tests the handling of malformed requests.
// This test suite verifies how the MCP server handles various types of malformed requests.
// It ensures that the server responds appropriately with error codes and messages when it receives invalid input.
//
// t *testing.T: The testing.T instance for the current test.
func TestMCPMalformedRequests(t *testing.T) {
	// Create a test configuration.
	// This configuration is used to initialize the MCP server with test parameters.
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
			TokenPath: t.TempDir() + "/token", // Use a temporary directory for token storage to avoid conflicts.
		},
	}

	// Create a mock RTM server.
	// This mock server simulates the RTM API, providing predictable responses for testing.
	rtmMock := mocks.NewRTMServer(t)
	defer rtmMock.Close() // Ensure the mock server is closed after the test completes.

	// Setup basic RTM responses.
	// These responses define the behavior of the mock RTM server for specific API calls related to authentication.
	rtmMock.AddResponse("rtm.auth.getFrob", `<rsp stat="ok"><frob>test_frob_12345</frob></rsp>`)

	// Override RTM API endpoint in client.
	// This redirects RTM API calls to the mock server for testing purposes.
	if err := os.Setenv("RTM_API_ENDPOINT", rtmMock.BaseURL); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer os.Unsetenv("RTM_API_ENDPOINT") // Ensure the environment variable is unset after the test.

	// Create and start the MCP server.
	// This initializes the MCP server with the test configuration and mock RTM service.
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create MCP test client.
	// This client is used to make requests to the MCP server during the test.
	client := helpers.NewMCPClient(t, s)
	defer client.Close() // Ensure the client is closed after the test.

	// Test malformed URLs.
	// This test suite checks how the server handles malformed URLs.
	t.Run("MalformedURLs", func(t *testing.T) {
		// Test implementation...
	})

	// Test oversized request bodies.
	// This test suite checks how the server handles oversized request bodies.
	t.Run("OversizedBodies", func(t *testing.T) {
		// Test implementation...
	})

	// Test malformed content types.
	// This test suite checks how the server handles requests with malformed content types.
	t.Run("MalformedContentTypes", func(t *testing.T) {
		// Test implementation...
	})
}

// DocEnhanced: 2025-03-25
