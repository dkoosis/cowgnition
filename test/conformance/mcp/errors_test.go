// Package conformance provides tests to verify MCP protocol compliance.
// file: test/conformance/mcp_error_response_test.go
package conformance

import (
	"bytes"
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
	"github.com/cowgnition/cowgnition/test/helpers/common"
	"github.com/cowgnition/cowgnition/test/mocks/common"
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
					validateStandardErrorResponse(t, response, http.StatusMethodNotAllowed)
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
					validateStandardErrorResponse(t, response, http.StatusBadRequest)
					validateErrorFieldExists(t, response, "error")
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
					validateStandardErrorResponse(t, response, http.StatusBadRequest)
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
					validateStandardErrorResponse(t, response, http.StatusBadRequest)
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
					validateStandardErrorResponse(t, response, http.StatusBadRequest)
					validateErrorMessage(t, response, "Missing resource name")
				},
			},
			{
				name:         "Method Not Allowed",
				method:       http.MethodPost,
				resourceName: "auth://rtm",
				wantStatus:   http.StatusMethodNotAllowed,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validateStandardErrorResponse(t, response, http.StatusMethodNotAllowed)
				},
			},
			{
				name:         "Resource Not Found",
				method:       http.MethodGet,
				resourceName: "invalid://resource",
				wantStatus:   http.StatusNotFound,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validateStandardErrorResponse(t, response, http.StatusNotFound)
					validateErrorMessage(t, response, "Resource not found")
				},
			},
			{
				name:         "Non-standard Resource Name Format",
				method:       http.MethodGet,
				resourceName: "not-a-valid-resource-name",
				wantStatus:   http.StatusNotFound,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validateStandardErrorResponse(t, response, http.StatusNotFound)
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

	// Section 3: Test error responses for /mcp/call_tool endpoint.
	t.Run("CallToolEndpointErrors", func(t *testing.T) {
		// Test cases for call_tool errors.
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
				body:        `{"name": "authenticate", "arguments": {"frob": "test_frob"}}`,
				contentType: "application/json",
				wantStatus:  http.StatusMethodNotAllowed,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validateStandardErrorResponse(t, response, http.StatusMethodNotAllowed)
				},
			},
			{
				name:        "Malformed JSON",
				method:      http.MethodPost,
				body:        `{"name": "authenticate", "arguments": {"frob": "test_frob"}, bad_json}`,
				contentType: "application/json",
				wantStatus:  http.StatusBadRequest,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validateStandardErrorResponse(t, response, http.StatusBadRequest)
				},
			},
			{
				name:        "Missing Tool Name",
				method:      http.MethodPost,
				body:        `{"arguments": {"frob": "test_frob"}}`,
				contentType: "application/json",
				wantStatus:  http.StatusBadRequest,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validateStandardErrorResponse(t, response, http.StatusBadRequest)
				},
			},
			{
				name:        "Unknown Tool",
				method:      http.MethodPost,
				body:        `{"name": "unknown_tool", "arguments": {}}`,
				contentType: "application/json",
				wantStatus:  http.StatusInternalServerError,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validateStandardErrorResponse(t, response, http.StatusInternalServerError)
					validateErrorMessage(t, response, "unknown tool")
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
					validateStandardErrorResponse(t, response, http.StatusBadRequest)
				},
			},
			{
				name:        "Missing Arguments",
				method:      http.MethodPost,
				body:        `{"name": "authenticate"}`,
				contentType: "application/json",
				wantStatus:  http.StatusBadRequest,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validateStandardErrorResponse(t, response, http.StatusBadRequest)
				},
			},
			{
				name:        "Authentication Tool Missing Frob",
				method:      http.MethodPost,
				body:        `{"name": "authenticate", "arguments": {}}`,
				contentType: "application/json",
				wantStatus:  http.StatusBadRequest,
				validateError: func(t *testing.T, response map[string]interface{}) {
					t.Helper()
					validateStandardErrorResponse(t, response, http.StatusBadRequest)
					validateErrorMessage(t, response, "Missing or invalid 'frob' argument")
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				req, err := http.NewRequestWithContext(ctx, tc.method, client.BaseURL+"/mcp/call_tool", bytes.NewBuffer([]byte(tc.body)))
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
}

// Section 4: Test edge cases and malformed requests.
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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Test invalid query parameter.
		invalidURL := client.BaseURL + "/mcp/read_resource?name=" + url.QueryEscape("auth://rtm") + "&invalid=%zzz"

		// Note: Go's http.NewRequest will escape invalid percent-encodings,
		// so the error will occur during the Do() call, if at all.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, invalidURL, nil)
		if err != nil {
			t.Fatalf("Unexpected error creating request: %v", err)
		}
		resp, err := client.Client.Do(req)
		if err != nil {
			// we expect some kind of error here, but it will be a url.Error due to the invalid escape
			return
		}
		defer resp.Body.Close()

		// Should return an error status.
		if resp.StatusCode == http.StatusOK {
			t.Error("Expected error status for malformed URL")
		}
		// Verify error response format.
		var response map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&response); err == nil {
			validateStandardErrorResponse(t, response, resp.StatusCode)
		}

		// Test oversized URL.
		longResourceName := "oversized://" + strings.Repeat("x", 10000)
		longURL := client.BaseURL + "/mcp/read_resource?name=" + url.QueryEscape(longResourceName)

		req, err = http.NewRequestWithContext(ctx, http.MethodGet, longURL, nil)
		if err != nil {
			// could fail here if the URL is just flat-out too long for the system
			return
		}

		resp, err = client.Client.Do(req)
		if err != nil {
			// This is also expected, as the URL is too long.
			return
		}
		defer resp.Body.Close()

		// If we got here, the server handled the oversized URL, which is fine,
		// but it should return an error status.
		if resp.StatusCode == http.StatusOK {
			t.Error("Expected error status for oversized URL")
		}
	})

	// Test oversized request bodies.
	t.Run("OversizedBodies", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Create a very large request body.
		largeBody := map[string]interface{}{
			"name": "authenticate",
			"arguments": map[string]interface{}{
				"frob": strings.Repeat("x", 10000000), // 10MB of 'x'
			},
		}

		bodyJSON, err := json.Marshal(largeBody)
		if err != nil {
			t.Fatalf("Failed to marshal large body: %v", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/mcp/call_tool", bytes.NewBuffer(bodyJSON))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Client.Do(req)
		if err != nil {
			// This might be expected if the client has request size limits.
			return
		}
		defer resp.Body.Close()

		// The server should handle this, but likely return an error status.
		if resp.StatusCode == http.StatusOK {
			t.Error("Expected error status for oversized request body")
		}

		// Verify error response format.
		var response map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&response); err == nil {
			validateStandardErrorResponse(t, response, resp.StatusCode)
		}
	})

	// Test malformed content types.
	t.Run("MalformedContentTypes", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		body := `{"name": "authenticate", "arguments": {"frob": "test_frob"}}`
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/mcp/call_tool", strings.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json; charset=invalid")

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Malformed content type should be handled gracefully.
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusOK {
			t.Errorf("Unexpected status code for malformed content type: %d", resp.StatusCode)
		}

		// Verify response format.
		var response map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			t.Errorf("Failed to parse response JSON: %v", err)
		}
	})
}

// Helper functions for validating error responses.

// validateStandardErrorResponse checks if an error response has the required fields.
func validateStandardErrorResponse(t *testing.T, response map[string]interface{}, expectedStatus int) {
	t.Helper()

	// Check for required error response fields according to MCP spec.
	requiredFields := []string{"error", "status", "timestamp"}
	for _, field := range requiredFields {
		if response[field] == nil {
			t.Errorf("Error response missing required field: %s", field)
		}
	}

	// Verify correct status code.
	if status, ok := response["status"].(float64); !ok || int(status) != expectedStatus {
		t.Errorf("Incorrect status code in error response: got %v, want %d", response["status"], expectedStatus)
	}

	// Verify timestamp is present and in a reasonable format.
	if timestamp, ok := response["timestamp"].(string); !ok || timestamp == "" {
		t.Error("Missing or invalid timestamp in error response")
	}
}

// validateErrorFieldExists checks if the error field exists and is non-empty.
func validateErrorFieldExists(t *testing.T, response map[string]interface{}, field string) {
	t.Helper()

	if response[field] == nil {
		t.Errorf("Response missing field: %s", field)
		return
	}

	if errStr, ok := response[field].(string); !ok || errStr == "" {
		t.Errorf("Field %s is not a non-empty string: %v", field, response[field])
	}
}

// validateErrorMessage checks if the error message contains an expected string.
func validateErrorMessage(t *testing.T, response map[string]interface{}, expectedContent string) {
	t.Helper()

	errMsg, ok := response["error"].(string)
	if !ok {
		t.Error("Error field is not a string")
		return
	}

	if !strings.Contains(strings.ToLower(errMsg), strings.ToLower(expectedContent)) {
		t.Errorf("Error message does not contain expected content: got %q, want to contain %q", errMsg, expectedContent)
	}
}
