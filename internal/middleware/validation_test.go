// File: internal/middleware/validation_test.go.
package middleware

import (
	"context"
	"encoding/json" // Import standard errors package.
	"testing"
	"time"

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
		"base":                true,
		"JSONRPCRequest":      true, // Added common base type
		"JSONRPCNotification": true, // Added common base type
		"ping":                true,
		"ping_notification":   true,
		"tools/list":          true,
		"tools/list_response": true, // Keep for OutgoingValidation test.
		"someMethod":          true,
		"request":             true, // Generic fallback
		"success_response":    true, // Generic fallback
		"error_response":      true, // Generic fallback
		"notification":        true, // Generic fallback
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

	if !opts.SkipTypes["ping"] {
		// Check validation was attempted with the correct schema type derived from the method
		assert.Equal(t, "ping", mockValidator.lastValidatedType, "Correct schema type should be used for validation.")
	} else {
		assert.NotEqual(t, "ping", mockValidator.lastValidatedType, "Validator should not have been called with 'ping' if skipped.")
	}
}

// Test ValidationMiddleware HandleMessage - Skip Type.
func TestValidationMiddleware_HandleMessage_SkipType(t *testing.T) {
	mockValidator := &mockSchemaValidator{shouldFail: true}
	mockNext := &mockNextHandler{}
	opts := DefaultValidationOptions()
	opts.SkipTypes = map[string]bool{"ping": true}
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"ping", "id":1}`)
	_, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should not error when type is skipped.")
	assert.True(t, mockNext.called, "Next handler should be called when type is skipped.")
	assert.NotEqual(t, "ping", mockValidator.lastValidatedType, "Validator Validate method should not have been called with 'ping'.")
}

// Test ValidationMiddleware HandleMessage - Validation Failure (Strict Mode).
func TestValidationMiddleware_HandleMessage_ValidationFailure_Strict(t *testing.T) {
	mockValidator := &mockSchemaValidator{shouldFail: true}
	mockNext := &mockNextHandler{}
	opts := DefaultValidationOptions()
	opts.StrictMode = true
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"someMethod","id":1}`)
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage itself shouldn't error, it returns an error response.")
	assert.False(t, mockNext.called, "Next handler should NOT be called on validation failure in strict mode.")
	assert.NotNil(t, respBytes, "An error response should be returned.")

	var errorResp map[string]interface{}
	errUnmarshal := json.Unmarshal(respBytes, &errorResp)
	require.NoError(t, errUnmarshal, "Failed to unmarshal error response.")
	errorField, hasError := errorResp["error"]
	assert.True(t, hasError, "Response should contain an 'error' field.")

	errMap, ok := errorField.(map[string]interface{})
	require.True(t, ok, "Error field should be a map.")
	codeFloat, ok := errMap["code"].(float64)
	require.True(t, ok, "Error code should be a number.")
	code := int(codeFloat)
	assert.True(t, code == transport.JSONRPCInvalidRequest || code == transport.JSONRPCInvalidParams, "Error code should be Invalid Request (-32600) or Invalid Params (-32602). Got %d", code)
}

// Test ValidationMiddleware HandleMessage - Validation Failure (Non-Strict Mode).
func TestValidationMiddleware_HandleMessage_ValidationFailure_NonStrict(t *testing.T) {
	mockValidator := &mockSchemaValidator{shouldFail: true}
	mockNext := &mockNextHandler{}
	opts := DefaultValidationOptions()
	opts.StrictMode = false
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
	assert.NotNil(t, respBytes, "An error response should be returned for invalid JSON.")

	var errorResp struct {
		Jsonrpc string      `json:"jsonrpc"`
		ID      interface{} `json:"id"`
		Error   struct {
			Code    int         `json:"code"`
			Message string      `json:"message"`
			Data    interface{} `json:"data,omitempty"`
		} `json:"error"`
	}
	errUnmarshal := json.Unmarshal(respBytes, &errorResp)
	require.NoError(t, errUnmarshal, "Failed to unmarshal the JSON-RPC error response structure.")
	assert.Equal(t, transport.JSONRPCParseError, errorResp.Error.Code, "Error code should be Parse Error (-32700).")
}

// Test identifyMessage function (Doesn't depend on validator).
func TestIdentifyMessage(t *testing.T) {
	mw := ValidationMiddleware{}

	tests := []struct {
		name          string
		input         []byte
		expectedType  string
		expectedID    interface{}
		expectError   bool
		errorContains string
	}{
		{"Valid Request", []byte(`{"jsonrpc":"2.0","method":"test","id":1}`), "test", float64(1), false, ""},
		{"Valid Request String ID", []byte(`{"jsonrpc":"2.0","method":"test","id":"abc"}`), "test", "abc", false, ""},
		{"Valid Notification No ID", []byte(`{"jsonrpc":"2.0","method":"notify"}`), "notify_notification", nil, false, ""},
		{"Valid Notification Null ID", []byte(`{"jsonrpc":"2.0","method":"notify","id":null}`), "notify_notification", nil, false, ""},
		{"Valid Standard Notification", []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`), "notifications/initialized_notification", nil, false, ""},
		{"Valid Success Response", []byte(`{"jsonrpc":"2.0","result":{"ok":true},"id":2}`), "success_response", float64(2), false, ""},
		{"Valid Error Response", []byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid"},"id":3}`), "error_response", float64(3), false, ""},
		{"Valid Error Response Null ID", []byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid"},"id":null}`), "error_response", nil, false, ""},
		{"Invalid JSON", []byte(`{invalid`), "", nil, true, "failed to parse message"},
		{"Missing Method/Result/Error", []byte(`{"jsonrpc":"2.0","id":4}`), "", float64(4), true, "unable to identify message type"},
		{"Invalid Method Type", []byte(`{"jsonrpc":"2.0","method":123,"id":5}`), "", float64(5), true, "failed to parse method"},
		{"Invalid ID Type", []byte(`{"jsonrpc":"2.0","method":"test","id":{}}`), "test", map[string]interface{}{}, true, "invalid type for id"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgType, reqID, err := mw.identifyMessage(tc.input)

			if tc.expectError {
				assert.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedType, msgType)
				if _, ok := tc.expectedID.(map[string]interface{}); ok {
					assert.Equal(t, tc.expectedID, reqID)
				} else {
					require.Equal(t, tc.expectedID, reqID)
				}
			}
		})
	}
}

// Test Outgoing Validation - Success.
func TestValidationMiddleware_HandleMessage_OutgoingValidation_Success(t *testing.T) {
	mockValidator := &mockSchemaValidator{}
	errInit := mockValidator.Initialize(context.Background())
	require.NoError(t, errInit)

	mockNext := &mockNextHandler{
		// Provide a response that's valid according to your *full* schema definition for tools/list_response
		responseToReturn: []byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"getTasks","inputSchema":{"type":"object"}}]}}`),
	}
	opts := DefaultValidationOptions()
	opts.ValidateOutgoing = true
	opts.StrictOutgoing = true
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	// Incoming message needs to be valid. Assume "tools/list" schema exists in mock.
	testMsg := []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should not error when outgoing validation succeeds.")
	assert.Equal(t, mockNext.responseToReturn, respBytes, "Original response should be returned.")
	// Check if the correct schema type was attempted for validation.
	assert.Equal(t, "tools/list_response", mockValidator.lastValidatedType, "Correct schema type used for outgoing validation.")
}

// Test Outgoing Validation - Failure (Strict).
func TestValidationMiddleware_HandleMessage_OutgoingValidation_Failure_Strict(t *testing.T) {
	mockValidator := &mockSchemaValidator{
		failOnlyOutgoing: true, // Use flag to fail only on the outgoing call.
		shouldFail:       true, // Set underlying failure flag.
	}
	mockNext := &mockNextHandler{
		responseToReturn: []byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"description":"a tool"}]}}`), // Invalid response structure
	}
	opts := DefaultValidationOptions()
	opts.ValidateOutgoing = true
	opts.StrictOutgoing = true // Fail on outgoing validation errors.
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	assert.Error(t, err, "HandleMessage should error when outgoing validation fails in strict mode.")
	assert.Nil(t, respBytes, "No response should be returned on strict outgoing failure.")
	assert.Contains(t, err.Error(), "failed outgoing message validation", "Error message should indicate outgoing validation failure.")
	assert.Equal(t, 2, mockValidator.outgoingCallCount, "Validator should have been called twice (incoming+outgoing).")
}

// Test Outgoing Validation - Failure (Non-Strict).
func TestValidationMiddleware_HandleMessage_OutgoingValidation_Failure_NonStrict(t *testing.T) {
	mockValidator := &mockSchemaValidator{
		failOnlyOutgoing: true,
		shouldFail:       true,
	}
	mockNext := &mockNextHandler{
		responseToReturn: []byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"description":"a tool"}]}}`), // Invalid response
	}
	opts := DefaultValidationOptions()
	opts.ValidateOutgoing = true
	opts.StrictOutgoing = false // Do NOT fail on outgoing validation errors.
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should NOT error when outgoing validation fails in non-strict mode.")
	assert.Equal(t, mockNext.responseToReturn, respBytes, "Original (invalid) response should still be returned.")
	assert.Equal(t, 2, mockValidator.outgoingCallCount, "Validator should have been called twice (incoming+outgoing).")
}

// Test Outgoing Validation - Skips Error Responses.
func TestValidationMiddleware_HandleMessage_OutgoingValidation_SkipsErrors(t *testing.T) {
	mockValidator := &mockSchemaValidator{shouldFail: true} // Configure mock to fail if outgoing validation were attempted
	mockNext := &mockNextHandler{
		responseToReturn: []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"Internal"}}`),
	}
	opts := DefaultValidationOptions()
	opts.ValidateOutgoing = true
	opts.StrictOutgoing = true
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	// Ensure incoming validation passes by temporarily setting shouldFail to false for the first call
	mockValidator.shouldFail = false

	testMsg := []byte(`{"jsonrpc":"2.0","method":"someMethod","id":1}`)
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	// Reset shouldFail for other tests if needed, though not strictly necessary here
	mockValidator.shouldFail = true // Reset just in case

	assert.NoError(t, err, "HandleMessage should not error when returning an error response.")
	assert.Equal(t, mockNext.responseToReturn, respBytes, "Error response should be returned.")
	assert.Equal(t, 1, mockValidator.outgoingCallCount, "Validator should only have been called once (incoming).")
	assert.NotEqual(t, "error_response", mockValidator.lastValidatedType, "Validator should not be called for error responses.")
}
