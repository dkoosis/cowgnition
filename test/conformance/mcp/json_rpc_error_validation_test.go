// file: test/conformance/mcp/json_rpc_error_validation_test.go
// Package conformance provides tests to verify MCP protocol compliance.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	helpers "github.com/cowgnition/cowgnition/test/helpers/common"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/server"
	"github.com/dkoosis/cowgnition/test/mocks"
	validators "github.com/dkoosis/cowgnition/test/validators/mcp"
)

// TestJSONRPCErrorValidation provides comprehensive testing for the JSON-RPC 2.0
// error handling implementation, ensuring it complies with the MCP specification.
func TestJSONRPCErrorValidation(t *testing.T) {
	// Create a test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name: "JSON-RPC Error Test Server",
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

	// Create a mock RTM server
	rtmMock := mocks.NewRTMServer(t)
	defer rtmMock.Close()

	// Create and start the MCP server
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create MCP test client
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// Test error response schema compliance
	t.Run("ErrorResponseSchema", func(t *testing.T) {
		testErrorResponseSchema(t, client)
	})

	// Test error code mapping
	t.Run("ErrorCodeMapping", func(t *testing.T) {
		testErrorCodeMapping(t)
	})

	// Test error detail handling
	t.Run("ErrorDetailHandling", func(t *testing.T) {
		testErrorDetailHandling(t)
	})

	// Test complete error lifecycle
	t.Run("ErrorLifecycle", func(t *testing.T) {
		testErrorLifecycle(t, client)
	})

	// Test error response status code mapping
	t.Run("StatusCodeMapping", func(t *testing.T) {
		testStatusCodeMapping(t)
	})
}

// testErrorResponseSchema validates that error responses conform to the JSON-RPC 2.0 schema.
func testErrorResponseSchema(t *testing.T, client *helpers.MCPClient) {
	t.Helper()

	// Generate errors by making invalid requests
	errorScenarios := []struct {
		name           string
		requestPath    string
		method         string
		body           string
		contentType    string
		expectedStatus int
	}{
		{
			name:           "InvalidMethod",
			requestPath:    "/mcp/initialize",
			method:         http.MethodGet, // Should be POST
			body:           "",
			contentType:    "application/json",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "MalformedJSON",
			requestPath:    "/mcp/initialize",
			method:         http.MethodPost,
			body:           `{"invalid json`,
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "ResourceNotFound",
			requestPath:    "/mcp/read_resource",
			method:         http.MethodGet,
			body:           "",
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest, // Missing name parameter
		},
	}

	for _, scenario := range errorScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Create request
			req, err := http.NewRequestWithContext(
				ctx,
				scenario.method,
				client.BaseURL+scenario.requestPath,
				strings.NewReader(scenario.body),
			)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			if scenario.contentType != "" {
				req.Header.Set("Content-Type", scenario.contentType)
			}

			// Send request
			resp, err := client.Client.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			// Verify status code
			if resp.StatusCode != scenario.expectedStatus {
				t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, scenario.expectedStatus)
			}

			// Read and parse response
			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("Failed to decode error response: %v", err)
			}

			// Validate response schema against JSON-RPC 2.0 specification
			validators.ValidateJSONRPCErrorSchema(t, result)
		})
	}
}

// testErrorCodeMapping validates that HTTP status codes are properly mapped to JSON-RPC error codes.
func testErrorCodeMapping(t *testing.T) {
	t.Helper()

	// Define status code to error code mapping expectations
	expectedMappings := map[int]int{
		http.StatusBadRequest:          -32600, // InvalidRequest
		http.StatusNotFound:            -32601, // MethodNotFound
		http.StatusMethodNotAllowed:    -32601, // MethodNotFound
		http.StatusInternalServerError: -32603, // InternalError
	}

	for httpStatus, expectedErrorCode := range expectedMappings {
		t.Run(fmt.Sprintf("HTTP%d", httpStatus), func(t *testing.T) {
			// Create a test server that returns the specified HTTP status
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Use the server's error handling to generate a JSON-RPC error response
				switch httpStatus {
				case http.StatusBadRequest:
					server.WriteJSONRPCErrorWithContext(w, server.InvalidRequest, "Bad request", nil)
				case http.StatusNotFound:
					server.WriteJSONRPCErrorWithContext(w, server.MethodNotFound, "Method not found", nil)
				case http.StatusMethodNotAllowed:
					server.WriteJSONRPCErrorWithContext(w, server.MethodNotFound, "Method not allowed", nil)
				case http.StatusInternalServerError:
					server.WriteJSONRPCErrorWithContext(w, server.InternalError, "Internal server error", nil)
				default:
					w.WriteHeader(httpStatus)
				}
			}))
			defer ts.Close()

			// Make a request with context
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			// Parse response
			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			// Validate error code mapping
			errObj, ok := result["error"].(map[string]interface{})
			if !ok {
				t.Fatalf("Error field is not an object: %T", result["error"])
			}

			code, ok := errObj["code"].(float64)
			if !ok {
				t.Fatalf("Error code is not a number: %T", errObj["code"])
			}

			if int(code) != expectedErrorCode {
				t.Errorf("Incorrect error code mapping: got %d, want %d", int(code), expectedErrorCode)
			}
		})
	}
}

// testErrorDetailHandling validates that sensitive information is not leaked in error responses.
func testErrorDetailHandling(t *testing.T) {
	t.Helper()

	// Create a server with a handler that includes sensitive information in the error
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sensitive := map[string]interface{}{
			"credential": "secret_api_key",
			"password":   "p@ssw0rd",
			"token":      "jwt_token_with_sensitive_data",
		}

		// Create detailed error with sensitive context
		context := map[string]interface{}{
			"public_data":  "This is public",
			"request_path": r.URL.Path,
			"details":      sensitive, // This should not appear in the response
		}

		server.WriteJSONRPCErrorWithContext(w, server.InternalError,
			"An error occurred", context)
	}))
	defer ts.Close()

	// Make a request with context
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Make another request to get body as string for checks
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp2, err := httpClient.Do(req2)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp2.Body.Close()

	body, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(body)

	// Check that sensitive info is not included
	sensitiveStrings := []string{
		"secret_api_key",
		"p@ssw0rd",
		"jwt_token_with_sensitive_data",
	}

	for _, s := range sensitiveStrings {
		if strings.Contains(bodyStr, s) {
			t.Errorf("Response contains sensitive information: %s", s)
		}
	}

	// Verify that stack trace is not included
	if strings.Contains(bodyStr, "runtime.") || strings.Contains(bodyStr, ".go:") {
		t.Error("Response contains stack trace information")
	}
}

// testErrorLifecycle validates the complete error handling lifecycle.
func testErrorLifecycle(t *testing.T, client *helpers.MCPClient) {
	t.Helper()

	// Get an example of each type of error
	errorTypes := []struct {
		name           string
		requestPath    string
		method         string
		body           string
		expectedStatus int
	}{
		{
			name:           "ParseError",
			requestPath:    "/mcp/initialize",
			method:         http.MethodPost,
			body:           `{"invalid": json}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "InvalidRequest",
			requestPath:    "/mcp/read_resource", // Missing required parameter
			method:         http.MethodGet,
			body:           "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "MethodNotFound",
			requestPath:    "/mcp/nonexistent_endpoint",
			method:         http.MethodGet,
			body:           "",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "MethodNotAllowed",
			requestPath:    "/mcp/initialize", // Should be POST
			method:         http.MethodGet,
			body:           "",
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, scenario := range errorTypes {
		t.Run(scenario.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Create request
			req, err := http.NewRequestWithContext(
				ctx,
				scenario.method,
				client.BaseURL+scenario.requestPath,
				strings.NewReader(scenario.body),
			)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			if scenario.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}

			// Send request
			resp, err := client.Client.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			// Verify status code
			if resp.StatusCode != scenario.expectedStatus {
				t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, scenario.expectedStatus)
			}

			// Check Content-Type header
			contentType := resp.Header.Get("Content-Type")
			if !strings.HasPrefix(contentType, "application/json") {
				t.Errorf("Unexpected Content-Type: got %s, want application/json", contentType)
			}

			// Parse response
			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("Failed to decode error response: %v", err)
			}

			// Validate JSON-RPC error schema
			validators.ValidateJSONRPCErrorSchema(t, result)
		})
	}
}

// testStatusCodeMapping validates that error codes are properly translated to HTTP status codes.
func testStatusCodeMapping(t *testing.T) {
	t.Helper()

	// Create test server that uses various error codes
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var errorCode server.ErrorCode

		// Extract error code from query parameter
		if codeStr := r.URL.Query().Get("code"); codeStr != "" {
			if code, err := strconv.Atoi(codeStr); err == nil {
				errorCode = server.ErrorCode(code)
			}
		}

		// Create error response with the specified code.
		errorResp := server.NewErrorResponse(errorCode, "Test error", nil)
		server.WriteJSONRPCError(w, errorResp)
	}))
	defer ts.Close()

	// Test error code to HTTP status mapping.
	testCases := []struct {
		errorCode      server.ErrorCode
		expectedStatus int
	}{
		{server.ParseError, http.StatusBadRequest},
		{server.InvalidRequest, http.StatusBadRequest},
		{server.MethodNotFound, http.StatusNotFound},
		{server.InvalidParams, http.StatusBadRequest},
		{server.InternalError, http.StatusInternalServerError},
		{server.AuthError, http.StatusUnauthorized},
		{server.ResourceError, http.StatusNotFound},
		{server.ValidationError, http.StatusBadRequest},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("ErrorCode%d", tc.errorCode), func(t *testing.T) {
			// Make request with specific error code and context
			ctx := context.Background()
			url := fmt.Sprintf("%s?code=%d", ts.URL, tc.errorCode)

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			httpClient := &http.Client{}
			resp, err := httpClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			// Verify HTTP status code matches expected mapping
			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Incorrect status code mapping: got %d, want %d for error code %d",
					resp.StatusCode, tc.expectedStatus, tc.errorCode)
			}
		})
	}
}

// Helper function for importing error codes directly.
func init() {
	// Importing error codes for direct access in this test.
	_ = reflect.TypeOf(server.ErrorCode(0))
}
