// Package middleware_test tests the middleware components.
package middleware_test

// file: internal/middleware/validation_identify_test.go.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dkoosis/cowgnition/internal/middleware"
	"github.com/dkoosis/cowgnition/internal/transport" // Import transport for error codes
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestIdentifyMessageHelper_HandlesVariousCases_When_GivenDifferentMessages tests the internal identifyMessage logic via HandleMessage.
// Renamed function to follow ADR-008 convention.
func TestIdentifyMessageHelper_HandlesVariousCases_When_GivenDifferentMessages(t *testing.T) {
	// Test cases define various message structures and expected outcomes.
	// The 'expectedErrorCode' field is added for error cases.
	testCases := []struct {
		name              string
		message           string
		expectedType      string // Expected type hint returned by identifyMessage (used internally).
		expectedID        interface{}
		expectError       bool // Whether HandleMessage should return an error *response*.
		expectedErrorCode int  // Expected JSON-RPC error code if expectError is true.
		// expectErrorMsg is removed as we focus on codes now
	}{
		{
			name:         "Request_Valid_NoError",
			message:      `{"jsonrpc": "2.0", "method": "initialize", "id": 1, "params": {}}`,
			expectedType: "initialize",
			expectedID:   float64(1),
			expectError:  false,
		},
		{
			name:         "Request_ValidStringID_NoError",
			message:      `{"jsonrpc": "2.0", "method": "tools/list", "id": "req-abc", "params": null}`,
			expectedType: "tools/list",
			expectedID:   "req-abc",
			expectError:  false,
		},
		{
			name:         "Notification_NoID_NoError",
			message:      `{"jsonrpc": "2.0", "method": "ping"}`,
			expectedType: "ping",
			expectedID:   nil,
			expectError:  false,
		},
		{
			name:         "Notification_NullID_NoError",
			message:      `{"jsonrpc": "2.0", "method": "ping", "id": null}`,
			expectedType: "ping",
			expectedID:   nil,
			expectError:  false,
		},
		{
			name:         "SuccessResponse_Valid_NoError",
			message:      `{"jsonrpc": "2.0", "id": 10, "result": "ok"}`,
			expectedType: "success_response",
			expectedID:   float64(10),
			expectError:  false,
		},
		{
			name:         "SuccessResponse_NullID_NoError",
			message:      `{"jsonrpc": "2.0", "id": null, "result": "ok_for_null"}`,
			expectedType: "success_response",
			expectedID:   nil,
			expectError:  false,
		},
		{
			name:         "ErrorResponse_Valid_NoError",
			message:      `{"jsonrpc": "2.0", "id": 11, "error": {"code": -32600, "message": "Invalid Request"}}`,
			expectedType: "error_response",
			expectedID:   float64(11),
			expectError:  false, // identifyMessage doesn't error on valid error responses
		},
		{
			name:         "ErrorResponse_NullID_NoError",
			message:      `{"jsonrpc": "2.0", "id": null, "error": {"code": -32700, "message": "Parse error"}}`,
			expectedType: "error_response",
			expectedID:   nil,
			expectError:  false, // identifyMessage doesn't error on valid error responses
		},
		{
			name:         "ErrorResponse_MissingID_NoError",
			message:      `{"jsonrpc": "2.0", "error": {"code": -32700, "message": "Parse error"}}`,
			expectedType: "error_response",
			expectedID:   nil,
			expectError:  false, // identifyMessage doesn't error on valid error responses (allows missing ID)
		},
		// --- Error Cases ---
		{
			name:              "JSON_InvalidSyntax_ReturnsError",
			message:           `{"jsonrpc": "2.0", "method": "test`,
			expectedType:      "",
			expectedID:        nil, // ID cannot be reliably parsed
			expectError:       true,
			expectedErrorCode: transport.JSONRPCParseError, // -32700
		},
		{
			name:              "Request_MissingMethod_ReturnsError",
			message:           `{"jsonrpc": "2.0", "id": 1}`,
			expectedType:      "",
			expectedID:        float64(1), // Request had an ID
			expectError:       true,
			expectedErrorCode: transport.JSONRPCInvalidRequest, // -32600
		},
		{
			name:         "Request_MissingJsonrpc_NoError", // identifyMessage doesn't fail here, validation middleware would
			message:      `{"method": "test", "id": 1}`,
			expectedType: "test",
			expectedID:   float64(1),
			expectError:  false, // Middleware won't return error based on identifyMessage alone
		},
		{
			name:              "Request_InvalidIDTypeBool_ReturnsError",
			message:           `{"jsonrpc": "2.0", "method": "test", "id": true}`,
			expectedType:      "",
			expectedID:        nil, // Invalid ID type
			expectError:       true,
			expectedErrorCode: transport.JSONRPCInvalidRequest, // -32600 (Identify message fails)
		},
		{
			name:              "Request_InvalidIDTypeObject_ReturnsError",
			message:           `{"jsonrpc": "2.0", "method": "test", "id": {}}`,
			expectedType:      "",
			expectedID:        nil, // Invalid ID type
			expectError:       true,
			expectedErrorCode: transport.JSONRPCInvalidRequest, // -32600 (Identify message fails)
		},
		{
			name:              "Response_ResultAndError_ReturnsError",
			message:           `{"jsonrpc": "2.0", "id": 1, "result": "ok", "error": {}}`,
			expectedType:      "",
			expectedID:        float64(1), // Response had an ID
			expectError:       true,
			expectedErrorCode: transport.JSONRPCInvalidRequest, // -32600 (Identify message fails)
		},
		{
			name:              "SuccessResponse_MissingID_ReturnsError",
			message:           `{"jsonrpc": "2.0", "result": "ok"}`,
			expectedType:      "",
			expectedID:        nil, // ID is missing
			expectError:       true,
			expectedErrorCode: transport.JSONRPCInvalidRequest, // -32600 (Identify message fails)
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mocks for each test run
			options := middleware.DefaultValidationOptions()
			options.StrictMode = true // Ensure errors are returned
			options.ValidateOutgoing = false
			options.SkipTypes = make(map[string]bool)
			mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

			// Conditionally expect validation/next handler calls only on success cases
			if !tc.expectError && tc.expectedType != "" {
				mockValidator.On("Validate", mock.Anything, mock.AnythingOfType("string"), []byte(tc.message)).Return(nil).Maybe()
				mockNextHandler.On("Handle", mock.Anything, []byte(tc.message)).Return([]byte(`{"result":"ok"}`), nil).Maybe()
			}

			// Execute the middleware
			respBytes, handleErr := mw(mockNextHandler.Handle)(context.Background(), []byte(tc.message))

			// --- Assertion Logic ---
			if tc.expectError {
				// HandleMessage should handle errors internally and return error *response bytes*, not a Go error.
				assert.NoError(t, handleErr, "HandleMessage should not return a Go error for expected JSON-RPC errors.")
				require.NotNil(t, respBytes, "Error response bytes should not be nil for expected error case.")

				// Parse the error response
				var errResp map[string]interface{}
				err := json.Unmarshal(respBytes, &errResp)
				require.NoError(t, err, "Error response should be valid JSON.")

				// 1. Check for the presence of the 'error' object
				errObjRaw, ok := errResp["error"]
				require.True(t, ok, "Response must contain an 'error' field.")
				errObj, ok := errObjRaw.(map[string]interface{})
				require.True(t, ok, "The 'error' field must be a JSON object.")

				// 2. Assert on the JSON-RPC Error Code
				require.Contains(t, errObj, "code", "Error object must contain a 'code' field.")
				assert.EqualValues(t, tc.expectedErrorCode, errObj["code"], "JSON-RPC error code mismatch for test case '%s'.", tc.name)

				// 3. Assert on the ID (should match request ID if present and applicable)
				idVal, idExists := errResp["id"]
				if tc.expectedID != nil {
					// If the original request had an ID, the error response should echo it (except maybe for ParseError)
					require.True(t, idExists, "Error response missing expected ID for test case '%s'.", tc.name)
					assert.EqualValues(t, tc.expectedID, idVal, "Error response ID mismatch for test case '%s'.", tc.name)
				} else {
					// If the original request had no ID (notification) or ID was invalid/unparseable,
					// the error response ID should be null.
					// Note: Error responses to notifications technically shouldn't happen per spec,
					// but servers might send them. ParseError (-32700) also often results in null ID.
					if idExists { // ID might be missing entirely or explicitly null
						assert.Nil(t, idVal, "Expected null or missing ID in error response for test case '%s'.", tc.name)
					}
				}

				// 4. Optional: Log the error message and data for debugging if needed
				//    This avoids making the test brittle based on the exact message string.
				// mainMessage, _ := errObj["message"].(string)
				// dataField, _ := errObj["data"]
				// t.Logf("Received error response: code=%v, id=%v, message=%q, data=%v", errObj["code"], idVal, mainMessage, dataField)
			} else {
				// --- Success Case Assertions ---
				assert.NoError(t, handleErr, "HandleMessage returned unexpected Go error for test case '%s'.", tc.name)
				// If it was a valid request/notification, ensure the next handler was called.
				if tc.expectedType != "" && tc.expectedType != "success_response" && tc.expectedType != "error_response" {
					mockNextHandler.AssertCalled(t, "Handle", mock.Anything, []byte(tc.message))
				}
				// You could add checks on respBytes for success cases if needed,
				// e.g., assert it's not nil if a response was expected from mockNextHandler.
			}
		})
	}
}
