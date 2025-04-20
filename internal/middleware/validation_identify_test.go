// Package middleware_test tests the middleware components.
package middleware_test

// file: internal/middleware/validation_identify_test.go.

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

// TestIdentifyMessageHelper_HandlesVariousCases_When_GivenDifferentMessages tests the internal identifyMessage logic via HandleMessage.
// Renamed function to follow ADR-008 convention.
func TestIdentifyMessageHelper_HandlesVariousCases_When_GivenDifferentMessages(t *testing.T) {
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
			expectedID:     float64(1),
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
			name:           "ErrorResponse_MissingID_NoError",
			message:        `{"jsonrpc": "2.0", "error": {"code": -32700, "message": "Parse error"}}`,
			expectedType:   "error_response",
			expectedID:     nil,
			expectError:    false,
			expectErrorMsg: "",
		},
		{
			name:           "JSON_InvalidSyntax_ReturnsError",
			message:        `{"jsonrpc": "2.0", "method": "test`,
			expectedType:   "",
			expectedID:     nil,
			expectError:    true,
			expectErrorMsg: "Parse error", // This comes from createParseErrorResponse message.
		},
		{
			name:           "Request_MissingMethod_ReturnsError",
			message:        `{"jsonrpc": "2.0", "id": 1}`,
			expectedType:   "",
			expectedID:     float64(1),
			expectError:    true,
			expectErrorMsg: "missing method, result, or error", // This detail comes from identifyMessage.
		},
		{
			name:           "Request_MissingJsonrpc_NoError",
			message:        `{"method": "test", "id": 1}`,
			expectedType:   "test",
			expectedID:     float64(1),
			expectError:    false, // identifyMessage doesn't check jsonrpc, validation should.
			expectErrorMsg: "",
		},
		{
			name:           "Request_InvalidIDTypeBool_ReturnsError",
			message:        `{"jsonrpc": "2.0", "method": "test", "id": true}`,
			expectedType:   "",
			expectedID:     nil,
			expectError:    true,
			expectErrorMsg: "Invalid JSON-RPC ID type detected", // This detail comes from identifyMessage.
		},
		{
			name:           "Request_InvalidIDTypeObject_ReturnsError",
			message:        `{"jsonrpc": "2.0", "method": "test", "id": {}}`,
			expectedType:   "",
			expectedID:     nil,
			expectError:    true,
			expectErrorMsg: "Invalid JSON-RPC ID type detected", // This detail comes from identifyMessage.
		},
		{
			name:           "Response_ResultAndError_ReturnsError",
			message:        `{"jsonrpc": "2.0", "id": 1, "result": "ok", "error": {}}`,
			expectedType:   "",
			expectedID:     float64(1),
			expectError:    true,
			expectErrorMsg: "cannot contain both 'result' and 'error'", // This detail comes from identifyMessage.
		},
		{
			name:           "SuccessResponse_MissingID_ReturnsError",
			message:        `{"jsonrpc": "2.0", "result": "ok"}`,
			expectedType:   "",
			expectedID:     nil,
			expectError:    true,
			expectErrorMsg: "success response message must contain an 'id' field", // This detail comes from identifyMessage.
		},
	}

	for _, tc := range testCases {
		// Run each case as a subtest with the test case name.
		t.Run(tc.name, func(t *testing.T) {
			options := middleware.DefaultValidationOptions()
			options.StrictMode = true
			options.ValidateOutgoing = false
			options.SkipTypes = make(map[string]bool)
			mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

			if !tc.expectError && tc.expectedType != "" {
				mockValidator.On("Validate", mock.Anything, mock.AnythingOfType("string"), []byte(tc.message)).Return(nil).Maybe()
				mockNextHandler.On("Handle", mock.Anything, []byte(tc.message)).Return([]byte(`{"result":"ok"}`), nil).Maybe()
			}

			// Corrected: Call the middleware function directly.
			respBytes, handleErr := mw(mockNextHandler.Handle)(context.Background(), []byte(tc.message))

			if tc.expectError {
				assert.NoError(t, handleErr, "HandleMessage should handle expected errors internally by returning error response bytes.")
				require.NotNil(t, respBytes, "Error response bytes should not be nil for expected error case.")

				var errResp map[string]interface{}
				err := json.Unmarshal(respBytes, &errResp)
				require.NoError(t, err, "Error response should be valid JSON.")
				require.Contains(t, errResp, "error", "Response must contain error field.")
				errObj := errResp["error"].(map[string]interface{})

				// Check if the expected message substring is in the main message OR the data field (cause/detail).
				mainMessage, _ := errObj["message"].(string)
				dataField, dataOk := errObj["data"].(map[string]interface{})
				causeField, causeOk := "", false
				detailField, detailOk := "", false
				if dataOk {
					// Check for "cause" or "detail" keys which might hold the specific error string.
					causeField, causeOk = dataField["cause"].(string)
					detailField, detailOk = dataField["detail"].(string)
				}

				foundInMessage := strings.Contains(mainMessage, tc.expectErrorMsg)
				foundInDataCause := causeOk && strings.Contains(causeField, tc.expectErrorMsg)
				foundInDataDetail := detailOk && strings.Contains(detailField, tc.expectErrorMsg)

				if !foundInMessage && !foundInDataCause && !foundInDataDetail {
					// If not found in any relevant field, fail the test explicitly.
					t.Errorf("Expected error message/detail containing %q not found in message: %q or data: %v.",
						tc.expectErrorMsg, mainMessage, dataField)
				}

				if tc.expectedID != nil {
					assert.EqualValues(t, tc.expectedID, errResp["id"], "Error response ID mismatch.")
				}
			} else {
				assert.NoError(t, handleErr, "HandleMessage returned unexpected Go error.")
				// Can add more checks here, e.g., verify next handler was called if expected.
				if tc.expectedType != "" { // If it's a valid request/notification.
					mockNextHandler.AssertCalled(t, "Handle", mock.Anything, []byte(tc.message))
				}
			}
		})
	}
}
