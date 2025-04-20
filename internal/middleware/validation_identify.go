// Package middleware_test tests the middleware components.
package middleware_test

// file: internal/middleware/validation_identify_test.go

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dkoosis/cowgnition/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestValidationMiddleware_IdentifiesMessages_When_GivenVariousInputs tests the internal identifyMessage logic via HandleMessage.
// It uses a table-driven approach to cover different message structures and expected outcomes.
func TestValidationMiddleware_IdentifiesMessages_When_GivenVariousInputs(t *testing.T) {
	testCases := []struct {
		name           string
		message        string
		expectedType   string // Expected type hint returned by identifyMessage (used internally).
		expectedID     interface{}
		expectError    bool   // Whether HandleMessage should return an error *response*.
		expectErrorMsg string // Substring expected in the error message OR data field.
	}{
		{
			name:           "Request_Valid_NoError",
			message:        `{"jsonrpc": "2.0", "method": "initialize", "id": 1, "params": {}}`,
			expectedType:   "initialize",
			expectedID:     float64(1), // JSON numbers unmarshal to float64 by default.
			expectError:    false,
			expectErrorMsg: "",
		},
		{
			name:           "Request_ValidStringID_NoError",
			message:        `{"jsonrpc": "2.0", "method": "tools/list", "id": "req-abc", "params": null}`,
			expectedType:   "tools/list",
			expectedID:     "req-abc",
			expectError:    false,
			expectErrorMsg: "",
		},
		{
			name:           "Notification_NoID_NoError",
			message:        `{"jsonrpc": "2.0", "method": "ping"}`,
			expectedType:   "ping",
			expectedID:     nil,
			expectError:    false,
			expectErrorMsg: "",
		},
		{
			name:           "Notification_NullID_NoError",
			message:        `{"jsonrpc": "2.0", "method": "ping", "id": null}`,
			expectedType:   "ping",
			expectedID:     nil,
			expectError:    false,
			expectErrorMsg: "",
		},
		{
			name:           "SuccessResponse_Valid_NoError",
			message:        `{"jsonrpc": "2.0", "id": 10, "result": "ok"}`,
			expectedType:   "success_response",
			expectedID:     float64(10),
			expectError:    false,
			expectErrorMsg: "",
		},
		{
			name:           "SuccessResponse_NullID_NoError",
			message:        `{"jsonrpc": "2.0", "id": null, "result": "ok_for_null"}`,
			expectedType:   "success_response",
			expectedID:     nil,
			expectError:    false,
			expectErrorMsg: "",
		},
		{
			name:           "ErrorResponse_Valid_NoError",
			message:        `{"jsonrpc": "2.0", "id": 11, "error": {"code": -32600, "message": "Invalid Request"}}`,
			expectedType:   "error_response",
			expectedID:     float64(11),
			expectError:    false,
			expectErrorMsg: "",
		},
		{
			name:           "ErrorResponse_NullID_NoError",
			message:        `{"jsonrpc": "2.0", "id": null, "error": {"code": -32700, "message": "Parse error"}}`,
			expectedType:   "error_response",
			expectedID:     nil,
			expectError:    false,
			expectErrorMsg: "",
		},
		{
			name:           "ErrorResponse_MissingID_NoError", // identifyMessage handles this gracefully.
			message:        `{"jsonrpc": "2.0", "error": {"code": -32700, "message": "Parse error"}}`,
			expectedType:   "error_response",
			expectedID:     nil,
			expectError:    false,
			expectErrorMsg: "",
		},
		{
			name:           "JSON_InvalidSyntax_ReturnsError",
			message:        `{"jsonrpc": "2.0", "method": "test`, // Missing closing brace.
			expectedType:   "",
			expectedID:     nil,
			expectError:    true,
			expectErrorMsg: "Parse error", // This comes from createParseErrorResponse message.
		},
		{
			name:           "Request_MissingMethod_ReturnsError",
			message:        `{"jsonrpc": "2.0", "id": 1}`,
			expectedType:   "",         // identifyMessage fails to identify type.
			expectedID:     float64(1), // ID can still be extracted.
			expectError:    true,
			expectErrorMsg: "missing method, result, or error", // Detail from identifyMessage failure.
		},
		{
			name:           "Request_MissingJsonrpc_NoErrorAtIdentification", // identifyMessage doesn't strictly require jsonrpc field.
			message:        `{"method": "test", "id": 1}`,
			expectedType:   "test", // identifyMessage identifies based on 'method' and 'id'.
			expectedID:     float64(1),
			expectError:    false, // No error *during identification*. Schema validation middleware *would* fail this later.
			expectErrorMsg: "",
		},
		{
			name:           "Request_InvalidIDTypeBool_ReturnsError",
			message:        `{"jsonrpc": "2.0", "method": "test", "id": true}`,
			expectedType:   "",  // identifyMessage fails due to invalid ID type.
			expectedID:     nil, // ID extraction returns nil for invalid types.
			expectError:    true,
			expectErrorMsg: "Invalid JSON-RPC ID type detected", // Detail from identifyRequestID via identifyMessage.
		},
		{
			name:           "Request_InvalidIDTypeObject_ReturnsError",
			message:        `{"jsonrpc": "2.0", "method": "test", "id": {}}`,
			expectedType:   "",
			expectedID:     nil,
			expectError:    true,
			expectErrorMsg: "Invalid JSON-RPC ID type detected", // Detail from identifyRequestID via identifyMessage.
		},
		{
			name:           "Response_ResultAndError_ReturnsError",
			message:        `{"jsonrpc": "2.0", "id": 1, "result": "ok", "error": {}}`,
			expectedType:   "", // identifyMessage fails due to invalid combination.
			expectedID:     float64(1),
			expectError:    true,
			expectErrorMsg: "cannot contain both 'result' and 'error'", // Detail from identifyMessage.
		},
		{
			name:           "SuccessResponse_MissingID_ReturnsError",
			message:        `{"jsonrpc": "2.0", "result": "ok"}`,
			expectedType:   "", // identifyMessage fails specification check.
			expectedID:     nil,
			expectError:    true,
			expectErrorMsg: "success response message must contain an 'id' field", // Detail from identifyMessage.
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup middleware for each test case.
			options := middleware.DefaultValidationOptions()
			options.StrictMode = true                 // Test identification error handling in strict mode.
			options.ValidateOutgoing = false          // Focus on incoming identification/validation.
			options.SkipTypes = make(map[string]bool) // Don't skip any types for these tests.
			mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

			// Setup mock expectations ONLY if NO error is expected during identification.
			// If an ID/type identification error occurs, validation/next handler won't be reached.
			if !tc.expectError && tc.expectedType != "" {
				// Expect validation to be called IF identification succeeds AND it's not a response.
				// Responses might only trigger outgoing validation later (which is disabled here).
				// For simplicity, we set a lenient expectation here, focusing on the identification part.
				mockValidator.On("Validate", mock.Anything, mock.AnythingOfType("string"), []byte(tc.message)).Return(nil).Maybe()
				// Expect next handler only if it's a valid request/notification.
				if tc.expectedType != "success_response" && tc.expectedType != "error_response" {
					mockNextHandler.On("Handle", mock.Anything, []byte(tc.message)).Return([]byte(`{"result":"mock_ok"}`), nil).Once()
				}
			}

			// Execute the HandleMessage, which internally calls identifyMessage.
			respBytes, handleErr := mw.HandleMessage(context.Background(), []byte(tc.message))

			// Assertions based on whether an identification/syntax error was expected.
			if tc.expectError {
				assert.NoError(t, handleErr, "HandleMessage should handle expected identification/syntax errors internally by returning error response bytes, not a Go error.")
				require.NotNil(t, respBytes, "Error response bytes should not be nil for expected error case: %s.", tc.name)

				var errResp map[string]interface{}
				err := json.Unmarshal(respBytes, &errResp)
				require.NoError(t, err, "Error response should be valid JSON: %s.", tc.name)
				require.Contains(t, errResp, "error", "Error response must contain 'error' field: %s.", tc.name)
				errObj, ok := errResp["error"].(map[string]interface{})
				require.True(t, ok, "Error field should be a map: %s.", tc.name)

				// Check if the expected message substring is in the main message OR the data field (cause/detail).
				mainMessage, _ := errObj["message"].(string)
				dataField, dataOk := errObj["data"].(map[string]interface{})
				causeField, causeOk := "", false
				detailField, detailOk := "", false
				if dataOk {
					causeField, causeOk = dataField["cause"].(string)     // Check for "cause" key.
					detailField, detailOk = dataField["details"].(string) // Check for "details" key (used by createParseErrorResponse etc.).
				}

				foundInMessage := strings.Contains(mainMessage, tc.expectErrorMsg)
				foundInDataCause := causeOk && strings.Contains(causeField, tc.expectErrorMsg)
				foundInDataDetail := detailOk && strings.Contains(detailField, tc.expectErrorMsg)

				assert.True(t, foundInMessage || foundInDataCause || foundInDataDetail,
					"Case '%s': Expected error message/detail containing '%s' not found in message: '%s' or data: %v.",
					tc.name, tc.expectErrorMsg, mainMessage, dataField)

				// Assert the ID in the error response matches the expected ID (which might be nil).
				// Use EqualValues for flexibility between float64(1) and int(1) if needed, though JSON unmarshals numbers to float64.
				assert.EqualValues(t, tc.expectedID, errResp["id"], "Case '%s': Error response ID mismatch.", tc.name)

			} else {
				assert.NoError(t, handleErr, "Case '%s': HandleMessage returned unexpected Go error.", tc.name)
				// If it was a valid request/notification, assert the next handler was called.
				if tc.expectedType != "" && tc.expectedType != "success_response" && tc.expectedType != "error_response" {
					mockNextHandler.AssertCalled(t, "Handle", mock.Anything, []byte(tc.message))
					t.Logf("Case '%s': Correctly identified and passed to next handler.", tc.name)
				} else if tc.expectedType != "" {
					t.Logf("Case '%s': Correctly identified as response/notification not needing further handling in this test.", tc.name)
				}
			}
		})
	}
}
