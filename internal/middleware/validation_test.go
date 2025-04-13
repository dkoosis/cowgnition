// File: internal/middleware/validation_test.go.
package middleware

import (
	"context"
	"encoding/json" // Import standard errors package.
	"testing"
	"time"

	"github.com/cockroachdb/errors" // Import cockroachdb errors for errors.As.
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Components. ---

// Mock SchemaValidator for testing - Implements SchemaValidatorInterface (defined in validation.go).
type mockSchemaValidator struct {
	shouldFail        bool
	failWith          error
	lastValidatedType string
	initialized       bool
	schemas           map[string]bool // Simulate existing schemas.
	failOnlyOutgoing  bool            // Flag to control failure timing.
	outgoingCallCount int             // Counter for calls.
}

// Ensure mockSchemaValidator implements the interface (defined in validation.go).
var _ SchemaValidatorInterface = (*mockSchemaValidator)(nil)

func (m *mockSchemaValidator) Initialize(ctx context.Context) error {
	m.initialized = true
	// Simulate some common schemas based on current usage.
	m.schemas = map[string]bool{
		"base":                                   true,
		"JSONRPCRequest":                         true, // Added common base type.
		"JSONRPCNotification":                    true, // Added common base type.
		"ping":                                   true,
		"ping_notification":                      true,
		"tools/list":                             true,
		"tools/list_response":                    true, // Keep for OutgoingValidation test.
		"someMethod":                             true,
		"someMethod_response":                    true, // Added for outgoing tests.
		"request":                                true, // Generic fallback.
		"success_response":                       true, // Generic fallback.
		"error_response":                         true, // Generic fallback.
		"notification":                           true, // Generic fallback.
		"notifications/initialized":              true, // Added for identifyMessage test.
		"notifications/initialized_notification": true, // Added for identifyMessage test.
	}
	m.outgoingCallCount = 0 // Reset counter on init.
	return nil
}
func (m *mockSchemaValidator) Shutdown() error {
	m.initialized = false
	return nil
}
func (m *mockSchemaValidator) IsInitialized() bool { return m.initialized }

func (m *mockSchemaValidator) Validate(ctx context.Context, messageType string, data []byte) error {
	m.lastValidatedType = messageType
	m.outgoingCallCount++ // Increment call count.

	shouldCurrentCallFail := m.shouldFail
	// If failOnlyOutgoing is set, only fail after the second call (first is incoming).
	if m.failOnlyOutgoing && m.outgoingCallCount <= 1 {
		shouldCurrentCallFail = false
	}

	if shouldCurrentCallFail {
		if m.failWith != nil {
			return m.failWith // Return specific error if provided.
		}
		// Return a generic validation error.
		return schema.NewValidationError(schema.ErrValidationFailed, "mock validation error", nil).
			WithContext("instancePath", "/mock/path"). // Add some detail.
			WithContext("schemaPath", "#/mock")
	}
	return nil
}
func (m *mockSchemaValidator) HasSchema(name string) bool {
	if !m.initialized || m.schemas == nil {
		return false
	}
	_, ok := m.schemas[name]
	return ok
}
func (m *mockSchemaValidator) GetLoadDuration() time.Duration    { return 0 } // Not relevant for mock.
func (m *mockSchemaValidator) GetCompileDuration() time.Duration { return 0 } // Not relevant for mock.

// Mock MessageHandler for testing the chain.
type mockNextHandler struct {
	called           bool
	receivedMsg      []byte
	shouldFail       bool
	failWith         error
	responseToReturn []byte
}

func (m *mockNextHandler) HandleMessage(ctx context.Context, message []byte) ([]byte, error) {
	m.called = true
	m.receivedMsg = message
	if m.shouldFail {
		return nil, m.failWith
	}
	return m.responseToReturn, nil
}

// --- Test Cases. ---

// Helper to create a ValidationMiddleware with a mock validator and handler.
func createTestMiddleware(t *testing.T, validator SchemaValidatorInterface, next *mockNextHandler, opts ValidationOptions) *ValidationMiddleware {
	t.Helper()
	if !validator.IsInitialized() {
		err := validator.Initialize(context.Background())
		require.NoError(t, err, "Mock validator initialization failed in helper.")
	}

	if next == nil {
		next = &mockNextHandler{}
	}
	logger := logging.GetNoopLogger()

	mw := NewValidationMiddleware(validator, opts, logger)
	mw.SetNext(next.HandleMessage)

	return mw
}

// Test ValidationMiddleware HandleMessage - Validation Disabled.
func TestValidationMiddleware_HandleMessage_Disabled(t *testing.T) {
	mockValidator := &mockSchemaValidator{}
	mockNext := &mockNextHandler{}
	opts := DefaultValidationOptions()
	opts.Enabled = false
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"ping"}`)
	_, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should not error when disabled.")
	assert.True(t, mockNext.called, "Next handler should be called when validation is disabled.")
	assert.Equal(t, testMsg, mockNext.receivedMsg, "Next handler should receive the original message.")
}

// Test ValidationMiddleware HandleMessage - Validation Success.
func TestValidationMiddleware_HandleMessage_ValidationSuccess(t *testing.T) {
	mockValidator := &mockSchemaValidator{}
	mockNext := &mockNextHandler{}
	opts := DefaultValidationOptions()
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"ping","id":1}`) // Request requires ID.
	_, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should not error on successful validation.")
	assert.True(t, mockNext.called, "Next handler should be called on successful validation.")
	assert.Equal(t, testMsg, mockNext.receivedMsg, "Next handler should receive the message.")

	// Use the mock's state to check if validation was attempted with the correct type.
	// Assumes 'ping' is not in SkipTypes by default.
	validatorType := mockValidator.lastValidatedType
	if !opts.SkipTypes["ping"] {
		assert.Equal(t, "ping", validatorType, "Correct schema type should be used for validation.")
	} else {
		assert.NotEqual(t, "ping", validatorType, "Validator should not have been called with 'ping' if skipped.")
	}
}

// Test ValidationMiddleware HandleMessage - Skip Type.
func TestValidationMiddleware_HandleMessage_SkipType(t *testing.T) {
	mockValidator := &mockSchemaValidator{shouldFail: true} // Configure mock to fail if called.
	mockNext := &mockNextHandler{}
	opts := DefaultValidationOptions()
	opts.SkipTypes = map[string]bool{"ping": true} // Explicitly skip ping.
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"ping", "id":1}`)
	_, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should not error when type is skipped.")
	assert.True(t, mockNext.called, "Next handler should be called when type is skipped.")
	// Assert that the mock validator's Validate method was NOT called with 'ping'.
	assert.NotEqual(t, "ping", mockValidator.lastValidatedType, "Validator Validate method should not have been called with 'ping'.")
}

// Test ValidationMiddleware HandleMessage - Validation Failure (Strict Mode).
func TestValidationMiddleware_HandleMessage_ValidationFailure_Strict(t *testing.T) {
	// Simulate a schema validation error.
	simulatedValidationError := schema.NewValidationError(schema.ErrValidationFailed, "mock validation error", nil).
		WithContext("instancePath", "/params/someField") // Simulate error in params.

	mockValidator := &mockSchemaValidator{
		shouldFail: true,
		failWith:   simulatedValidationError,
	}
	mockNext := &mockNextHandler{}
	opts := DefaultValidationOptions()
	opts.StrictMode = true
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"someMethod","id":1, "params": {"someField": "invalid"}}`) // Example invalid msg.
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage itself shouldn't error, it returns an error response.")
	assert.False(t, mockNext.called, "Next handler should NOT be called on validation failure in strict mode.")
	require.NotNil(t, respBytes, "An error response should be returned.")

	// Verify the returned JSON-RPC error response structure and code.
	var errorResp struct {
		Error struct {
			Code int         `json:"code"`
			Data interface{} `json:"data"` // Check data field for more context.
		} `json:"error"`
	}
	errUnmarshal := json.Unmarshal(respBytes, &errorResp)
	require.NoError(t, errUnmarshal, "Failed to unmarshal error response.")

	// Expect Invalid Params (-32602) because simulated error path includes /params.
	assert.Equal(t, transport.JSONRPCInvalidParams, errorResp.Error.Code, "Error code should be Invalid Params (-32602).")

	// Optionally check data field for validation details.
	errorData, ok := errorResp.Error.Data.(map[string]interface{})
	require.True(t, ok, "Error data should be a map.")
	assert.Contains(t, errorData["validationPath"], "/params/someField", "Error data should contain instance path.")
}

// Test ValidationMiddleware HandleMessage - Validation Failure (Non-Strict Mode).
func TestValidationMiddleware_HandleMessage_ValidationFailure_NonStrict(t *testing.T) {
	mockValidator := &mockSchemaValidator{shouldFail: true}
	mockNext := &mockNextHandler{}
	opts := DefaultValidationOptions()
	opts.StrictMode = false // Non-strict mode.
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"someMethod","id":1}`)
	_, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should not error on validation failure in non-strict mode.")
	assert.True(t, mockNext.called, "Next handler SHOULD be called on validation failure in non-strict mode.")
	assert.Equal(t, testMsg, mockNext.receivedMsg, "Next handler should receive the original message.")
}

// Test ValidationMiddleware HandleMessage - Invalid JSON Syntax.
func TestValidationMiddleware_HandleMessage_InvalidJSON(t *testing.T) {
	mockValidator := &mockSchemaValidator{}
	mockNext := &mockNextHandler{}
	opts := DefaultValidationOptions()
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":`) // Invalid JSON.
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should return an error response, not an internal error.")
	assert.False(t, mockNext.called, "Next handler should not be called for invalid JSON.")
	require.NotNil(t, respBytes, "An error response should be returned for invalid JSON.")

	// Verify the returned JSON-RPC error response structure and code.
	var errorResp struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	errUnmarshal := json.Unmarshal(respBytes, &errorResp)
	require.NoError(t, errUnmarshal, "Failed to unmarshal the JSON-RPC error response structure.")
	assert.Equal(t, transport.JSONRPCParseError, errorResp.Error.Code, "Error code should be Parse Error (-32700).")
}

// Test identifyMessage function (Doesn't depend on validator).
func TestIdentifyMessage(t *testing.T) {
	mw := ValidationMiddleware{} // Need an instance to call the method.

	tests := []struct {
		name          string
		input         []byte
		expectedType  string
		expectedID    interface{}
		expectError   bool
		errorContains string // Kept for specific checks where type assertion isn't feasible.
	}{
		{"Valid Request", []byte(`{"jsonrpc":"2.0","method":"test","id":1}`), "test", float64(1), false, ""},
		{"Valid Request String ID", []byte(`{"jsonrpc":"2.0","method":"test","id":"abc"}`), "test", "abc", false, ""},
		{"Valid Notification No ID", []byte(`{"jsonrpc":"2.0","method":"notify"}`), "notify_notification", nil, false, ""},
		{"Valid Notification Null ID", []byte(`{"jsonrpc":"2.0","method":"notify","id":null}`), "notify_notification", nil, false, ""},
		{"Valid Standard Notification", []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`), "notifications/initialized_notification", nil, false, ""},
		{"Valid Success Response", []byte(`{"jsonrpc":"2.0","result":{"ok":true},"id":2}`), "success_response", float64(2), false, ""},
		{"Valid Error Response", []byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid"},"id":3}`), "error_response", float64(3), false, ""},
		{"Valid Error Response Null ID", []byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid"},"id":null}`), "error_response", nil, false, ""},
		{"Invalid JSON", []byte(`{invalid`), "", nil, true, "failed to parse message"},                                                          // Expect specific internal error message.
		{"Missing Method/Result/Error", []byte(`{"jsonrpc":"2.0","id":4}`), "", float64(4), true, "unable to identify message type"},            // Expect specific internal error message.
		{"Invalid Method Type", []byte(`{"jsonrpc":"2.0","method":123,"id":5}`), "", float64(5), true, "failed to parse method"},                // Expect specific internal error message.
		{"Invalid ID Type", []byte(`{"jsonrpc":"2.0","method":"test","id":{}}`), "test", map[string]interface{}{}, true, "invalid type for id"}, // Specific internal error. Note: This check remains string-based as identifyMessage doesn't return custom types.
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// t.Parallel() // Can re-enable later if tests are confirmed independent.

			msgType, reqID, err := mw.identifyMessage(tc.input) // Assuming mw is defined in the test setup.

			if tc.expectError {
				// Assert that an error *was* returned.
				assert.Error(t, err)
				// Only check contents if an error was actually returned and substring provided.
				if err != nil && tc.errorContains != "" {
					// Acknowledging this is string checking for internal helper function errors.
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				// Assert that *no* error was returned.
				assert.NoError(t, err)
				// Only check type and ID if no error occurred.
				if err == nil {
					assert.Equal(t, tc.expectedType, msgType)
					// Handle potential map comparison if needed, though float64/string/nil should be fine.
					assert.Equal(t, tc.expectedID, reqID)
				}
			}
		})
	}
}

// Test Outgoing Validation - Success.
func TestValidationMiddleware_HandleMessage_OutgoingValidation_Success(t *testing.T) {
	mockValidator := &mockSchemaValidator{}
	// No need for explicit Initialize call here, createTestMiddleware handles it.

	mockNext := &mockNextHandler{
		// Provide a response that's valid according to your *full* schema definition for tools/list_response.
		// Ensure the mock schema includes 'tools/list_response'.
		responseToReturn: []byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"getTasks","inputSchema":{"type":"object"}}]}}`),
	}
	opts := DefaultValidationOptions()
	opts.ValidateOutgoing = true
	opts.StrictOutgoing = true // Doesn't matter for success case.
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	// Incoming message needs to be valid. Assume "tools/list" schema exists in mock.
	testMsg := []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should not error when outgoing validation succeeds.")
	assert.Equal(t, mockNext.responseToReturn, respBytes, "Original response should be returned.")
	// Check if the correct schema type was attempted for validation.
	// The identifyMessage logic should derive 'tools/list_response' from the request 'tools/list'.
	assert.Equal(t, "tools/list_response", mockValidator.lastValidatedType, "Correct schema type used for outgoing validation.")
	assert.Equal(t, 2, mockValidator.outgoingCallCount, "Validator should have been called twice (incoming+outgoing).")
}

// Test Outgoing Validation - Failure (Strict).
func TestValidationMiddleware_HandleMessage_OutgoingValidation_Failure_Strict(t *testing.T) {
	// Simulate an outgoing validation error.
	simulatedOutgoingError := schema.NewValidationError(schema.ErrValidationFailed, "mock outgoing validation error", nil).
		WithContext("instancePath", "/result/tools/0/name") // Example path.

	mockValidator := &mockSchemaValidator{
		failOnlyOutgoing: true,                   // Use flag to fail only on the outgoing call.
		shouldFail:       true,                   // Set underlying failure flag.
		failWith:         simulatedOutgoingError, // Specify the error to return.
	}
	mockNext := &mockNextHandler{
		responseToReturn: []byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"INVALID-TOOL-NAME"}]}}`), // Invalid response structure.
	}
	opts := DefaultValidationOptions()
	opts.ValidateOutgoing = true
	opts.StrictOutgoing = true // Fail on outgoing validation errors.
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	require.Error(t, err, "HandleMessage should error when outgoing validation fails in strict mode.")
	assert.Nil(t, respBytes, "No response should be returned on strict outgoing failure.")

	// Check that the error returned by HandleMessage wraps the specific validation error.
	var validationErr *schema.ValidationError
	assert.True(t, errors.As(err, &validationErr), "Returned error should wrap a *schema.ValidationError.")
	if validationErr != nil { // Avoid nil panic if errors.As fails.
		assert.Equal(t, schema.ErrValidationFailed, validationErr.Code)
		assert.Equal(t, "/result/tools/0/name", validationErr.InstancePath)
	}
	assert.Contains(t, err.Error(), "outgoing response validation failed", "Error message should indicate outgoing validation failure.") // Check wrapped message.

	assert.Equal(t, 2, mockValidator.outgoingCallCount, "Validator should have been called twice (incoming+outgoing).")
}

// Test Outgoing Validation - Failure (Non-Strict).
func TestValidationMiddleware_HandleMessage_OutgoingValidation_Failure_NonStrict(t *testing.T) {
	mockValidator := &mockSchemaValidator{
		failOnlyOutgoing: true,
		shouldFail:       true,
		failWith:         schema.NewValidationError(schema.ErrValidationFailed, "mock outgoing validation error", nil), // Provide an error.
	}
	mockNext := &mockNextHandler{
		responseToReturn: []byte(`{"jsonrpc":"2.0","id":1,"result":{"invalid":true}}`), // Invalid response.
	}
	opts := DefaultValidationOptions()
	opts.ValidateOutgoing = true
	opts.StrictOutgoing = false // Do NOT fail on outgoing validation errors.
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"someMethod","id":1}`) // Assume someMethod_response schema exists or falls back.
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should NOT error when outgoing validation fails in non-strict mode.")
	assert.Equal(t, mockNext.responseToReturn, respBytes, "Original (invalid) response should still be returned.")
	assert.Equal(t, 2, mockValidator.outgoingCallCount, "Validator should have been called twice (incoming+outgoing).")
	// lastValidatedType check depends on fallback logic, might be 'someMethod_response', 'success_response' or 'base'.
	// assert.Equal(t, "someMethod_response", mockValidator.lastValidatedType) // Or appropriate fallback.
}

// Test Outgoing Validation - Skips Error Responses.
func TestValidationMiddleware_HandleMessage_OutgoingValidation_SkipsErrors(t *testing.T) {
	// Mock configured to fail if outgoing validation *were* attempted.
	mockValidator := &mockSchemaValidator{shouldFail: true, failOnlyOutgoing: true}
	mockNext := &mockNextHandler{
		// This is a valid JSON-RPC error response structure.
		responseToReturn: []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"Internal"}}`),
	}
	opts := DefaultValidationOptions()
	opts.ValidateOutgoing = true
	opts.StrictOutgoing = true
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	// Ensure incoming validation passes by temporarily setting shouldFail to false for the first call.
	// This is a bit of a hack due to the mock's setup.
	mockValidator.shouldFail = false // Allow incoming validation to pass.

	testMsg := []byte(`{"jsonrpc":"2.0","method":"someMethod","id":1}`)
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	mockValidator.shouldFail = true // Reset mock state just in case.

	assert.NoError(t, err, "HandleMessage should not error when returning an error response.")
	assert.Equal(t, mockNext.responseToReturn, respBytes, "Error response should be returned.")
	assert.Equal(t, 1, mockValidator.outgoingCallCount, "Validator should only have been called once (incoming).")
	// Check that the *last* validation attempt was for the incoming request, not the error response.
	assert.Equal(t, "someMethod", mockValidator.lastValidatedType, "Last validation should have been for the incoming 'someMethod' request.")
}

// Test Helper: identifyMessage correctly identifies message types from raw bytes.
// No changes needed here based on previous analysis, but added comments.
func TestIdentifyMessageHelper(t *testing.T) {
	mw := ValidationMiddleware{} // Need an instance to call the method.

	tests := []struct {
		name          string
		input         []byte
		expectedType  string
		expectedID    interface{}
		expectError   bool
		errorContains string // Kept for specific checks where type assertion isn't feasible.
	}{
		{"Valid Request", []byte(`{"jsonrpc":"2.0","method":"test","id":1}`), "test", float64(1), false, ""},
		{"Valid Request String ID", []byte(`{"jsonrpc":"2.0","method":"test","id":"abc"}`), "test", "abc", false, ""},
		{"Valid Notification No ID", []byte(`{"jsonrpc":"2.0","method":"notify"}`), "notify_notification", nil, false, ""},
		{"Valid Notification Null ID", []byte(`{"jsonrpc":"2.0","method":"notify","id":null}`), "notify_notification", nil, false, ""},
		{"Valid Standard Notification", []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`), "notifications/initialized_notification", nil, false, ""},
		{"Valid Success Response", []byte(`{"jsonrpc":"2.0","result":{"ok":true},"id":2}`), "success_response", float64(2), false, ""},
		{"Valid Error Response", []byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid"},"id":3}`), "error_response", float64(3), false, ""},
		{"Valid Error Response Null ID", []byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid"},"id":null}`), "error_response", nil, false, ""},
		{"Invalid JSON", []byte(`{invalid`), "", nil, true, "failed to parse message"},                                                          // Expect specific internal error message.
		{"Missing Method/Result/Error", []byte(`{"jsonrpc":"2.0","id":4}`), "", float64(4), true, "unable to identify message type"},            // Expect specific internal error message.
		{"Invalid Method Type", []byte(`{"jsonrpc":"2.0","method":123,"id":5}`), "", float64(5), true, "failed to parse method"},                // Expect specific internal error message.
		{"Invalid ID Type", []byte(`{"jsonrpc":"2.0","method":"test","id":{}}`), "test", map[string]interface{}{}, true, "invalid type for id"}, // Specific internal error. Note: This check remains string-based as identifyMessage doesn't return custom types.
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// t.Parallel() // Can re-enable later if tests are confirmed independent.

			msgType, reqID, err := mw.identifyMessage(tc.input)

			if tc.expectError {
				assert.Error(t, err)
				if err != nil && tc.errorContains != "" {
					// Acknowledging this is string checking for internal helper function errors.
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
				if err == nil {
					assert.Equal(t, tc.expectedType, msgType)
					assert.Equal(t, tc.expectedID, reqID)
				}
			}
		})
	}
}
