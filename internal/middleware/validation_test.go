// File: internal/middleware/validation_test.go.
package middleware

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Components (Unchanged) ---

// Mock SchemaValidator for testing - Implements SchemaValidatorInterface.
type mockSchemaValidator struct {
	shouldFail        bool
	failWith          error
	lastValidatedType string
	initialized       bool
	schemas           map[string]bool // Simulate existing schemas.
	failOnlyOutgoing  bool            // Flag to control failure timing.
	outgoingCallCount int             // Counter for calls.
}

// Ensure mockSchemaValidator implements the interface.
var _ SchemaValidatorInterface = (*mockSchemaValidator)(nil)

func (m *mockSchemaValidator) Initialize(ctx context.Context) error {
	m.initialized = true
	// Simulate common schemas (ensure these match potential fallbacks in validation.go)
	m.schemas = map[string]bool{
		"base":                   true, // Base fallback
		"JSONRPCRequest":         true, // Generic request
		"JSONRPCNotification":    true, // Generic notification
		"JSONRPCResponse":        true, // Generic success response
		"JSONRPCError":           true, // Generic error response
		"ping":                   true, // Specific method
		"ping_notification":      true, // Specific notification
		"tools/list":             true,
		"tools/list_response":    true, // Specific response
		"CallToolResult":         true, // Specific result structure for outgoing check
		"someMethod":             true,
		"someMethod_response":    true, // Specific response
		"request":                true, // Simpler generic fallback (if used)
		"success_response":       true, // Simpler generic fallback (if used)
		"error_response":         true, // Simpler generic fallback (if used)
		"notification":           true, // Simpler generic fallback (if used)
		"initialize":             true, // Core method
		"notifications/progress": true, // Example standard notification
	}
	m.outgoingCallCount = 0 // Reset counter on init.
	return nil
}

func (m *mockSchemaValidator) Shutdown() error {
	m.initialized = false
	m.schemas = nil // Clear schemas on shutdown
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

// Mock implementation for duration getters.
func (m *mockSchemaValidator) GetLoadDuration() time.Duration    { return 10 * time.Millisecond }
func (m *mockSchemaValidator) GetCompileDuration() time.Duration { return 20 * time.Millisecond }

// Mock MessageHandler for testing the chain (Unchanged).
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

// --- Test Cases (Largely Unchanged, focus on Middleware Behavior) ---

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

// Test ValidationMiddleware HandleMessage - Validation Disabled (Unchanged).
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

// Test ValidationMiddleware HandleMessage - Validation Success (Unchanged).
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
	// Assumes 'ping' IS skipped by default in DefaultValidationOptions.
	validatorType := mockValidator.lastValidatedType
	if opts.SkipTypes["ping"] {
		assert.NotEqual(t, "ping", validatorType, "Validator should not have been called with 'ping' if skipped by default.")
	} else {
		// This branch might not be hit if ping is always skipped by default
		assert.Equal(t, "ping", validatorType, "Correct schema type should be used for validation if not skipped.")
	}
}

// Test ValidationMiddleware HandleMessage - Skip Type (Unchanged).
func TestValidationMiddleware_HandleMessage_SkipType(t *testing.T) {
	mockValidator := &mockSchemaValidator{shouldFail: true} // Configure mock to fail if called.
	mockNext := &mockNextHandler{}
	opts := DefaultValidationOptions()
	// Keep ping skipped as per default, or explicitly set:
	opts.SkipTypes["ping"] = true
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"ping", "id":1}`)
	_, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should not error when type is skipped.")
	assert.True(t, mockNext.called, "Next handler should be called when type is skipped.")
	// Assert that the mock validator's Validate method was NOT called (or called with a different type).
	assert.NotEqual(t, "ping", mockValidator.lastValidatedType, "Validator Validate method should not have been called with 'ping'.")
	assert.Equal(t, 0, mockValidator.outgoingCallCount, "Validator Validate method should not have been called at all.")
}

// Test ValidationMiddleware HandleMessage - Validation Failure (Strict Mode).
// Assertion logic remains the same as the middleware should *return* the same error response.
func TestValidationMiddleware_HandleMessage_ValidationFailure_Strict(t *testing.T) {
	// Simulate a schema validation error.
	simulatedValidationError := schema.NewValidationError(schema.ErrValidationFailed, "required property 'params' is missing", nil).
		WithContext("instancePath", ""). // Error is about the root object
		WithContext("schemaPath", "#/required")

	mockValidator := &mockSchemaValidator{
		shouldFail: true,
		failWith:   simulatedValidationError,
	}
	mockNext := &mockNextHandler{}
	opts := DefaultValidationOptions()
	opts.StrictMode = true
	opts.SkipTypes = make(map[string]bool) // Don't skip any types for this test
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"someMethod","id":1}`) // Invalid: Missing params
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage itself shouldn't error, it returns an error response.")
	assert.False(t, mockNext.called, "Next handler should NOT be called on validation failure in strict mode.")
	require.NotNil(t, respBytes, "An error response should be returned.")

	// Verify the returned JSON-RPC error response structure and code.
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
	require.NoError(t, errUnmarshal, "Failed to unmarshal error response.")

	assert.Equal(t, "2.0", errorResp.Jsonrpc)
	assert.Equal(t, float64(1), errorResp.ID) // JSON numbers unmarshal to float64

	// Expect Invalid Request (-32600) because the error is at the root level (missing 'params'), not inside 'params'.
	assert.Equal(t, transport.JSONRPCInvalidRequest, errorResp.Error.Code, "Error code should be Invalid Request (-32600).")
	assert.Equal(t, "Invalid Request", errorResp.Error.Message) // Check message matches code

	// Check data field for validation details.
	errorData, ok := errorResp.Error.Data.(map[string]interface{})
	require.True(t, ok, "Error data should be a map.")
	assert.Equal(t, "", errorData["validationPath"], "Error data should contain instance path.") // Path is empty for root error
	assert.Equal(t, "#/required", errorData["schemaPath"])
	assert.Contains(t, errorData["validationError"], "required property 'params' is missing")
	assert.Contains(t, errorData["suggestion"], "Ensure the required field 'params' is provided")
}

// Test ValidationMiddleware HandleMessage - Validation Failure (Non-Strict Mode) (Unchanged).
func TestValidationMiddleware_HandleMessage_ValidationFailure_NonStrict(t *testing.T) {
	mockValidator := &mockSchemaValidator{shouldFail: true}
	mockNext := &mockNextHandler{}
	opts := DefaultValidationOptions()
	opts.StrictMode = false                // Non-strict mode.
	opts.SkipTypes = make(map[string]bool) // Don't skip any types for this test
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"someMethod","id":1}`) // Missing params, would fail validation
	_, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should not error on validation failure in non-strict mode.")
	assert.True(t, mockNext.called, "Next handler SHOULD be called on validation failure in non-strict mode.")
	assert.Equal(t, testMsg, mockNext.receivedMsg, "Next handler should receive the original message.")
	assert.Equal(t, "someMethod", mockValidator.lastValidatedType, "Validator should have been called with 'someMethod'.")
}

// Test ValidationMiddleware HandleMessage - Invalid JSON Syntax.
// Assertion logic remains the same as the middleware should *return* the same error response.
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
			Code    int                    `json:"code"`
			Message string                 `json:"message"`
			Data    map[string]interface{} `json:"data"`
		} `json:"error"`
		ID interface{} `json:"id"` // Should be null for parse error before ID extraction
	}
	errUnmarshal := json.Unmarshal(respBytes, &errorResp)
	require.NoError(t, errUnmarshal, "Failed to unmarshal the JSON-RPC error response structure.")
	assert.Equal(t, transport.JSONRPCParseError, errorResp.Error.Code, "Error code should be Parse Error (-32700).")
	assert.Equal(t, "Parse error", errorResp.Error.Message)
	assert.Nil(t, errorResp.ID, "ID should be null for parse errors.")
	assert.Contains(t, errorResp.Error.Data["details"], "could not be parsed as valid JSON")
}

// Test identifyMessage function (This tests an internal helper, unchanged by the refactor).
func TestIdentifyMessage(t *testing.T) {
	// Need an instance with a mock validator to call HasSchema internally
	mockValidator := &mockSchemaValidator{}
	err := mockValidator.Initialize(context.Background()) // Initialize mock schemas
	require.NoError(t, err)

	mw := ValidationMiddleware{validator: mockValidator} // Use the initialized mock

	tests := []struct {
		name          string
		input         []byte
		expectedType  string
		expectedID    interface{}
		expectError   bool
		errorContains string // Kept for specific checks where type assertion isn't feasible.
	}{
		{"Valid Request", []byte(`{"jsonrpc":"2.0","method":"ping","id":1}`), "ping", float64(1), false, ""},
		{"Valid Request String ID", []byte(`{"jsonrpc":"2.0","method":"ping","id":"abc"}`), "ping", "abc", false, ""},
		{"Valid Notification No ID", []byte(`{"jsonrpc":"2.0","method":"ping"}`), "ping_notification", nil, false, ""},
		{"Valid Notification Null ID", []byte(`{"jsonrpc":"2.0","method":"ping","id":null}`), "ping_notification", nil, false, ""},
		{"Valid Standard Notification", []byte(`{"jsonrpc":"2.0","method":"notifications/progress"}`), "notifications/progress_notification", nil, false, ""},
		{"Valid Success Response", []byte(`{"jsonrpc":"2.0","result":{"ok":true},"id":2}`), "success_response", float64(2), false, ""},
		{"Valid Error Response", []byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid"},"id":3}`), "error_response", float64(3), false, ""},
		{"Valid Error Response Null ID", []byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid"},"id":null}`), "error_response", nil, false, ""}, // Error response *can* have null id
		{"Invalid JSON", []byte(`{invalid`), "", nil, true, "failed to parse message structure"},                                                              // Expect specific internal error message.
		{"Missing Method/Result/Error", []byte(`{"jsonrpc":"2.0","id":4}`), "", float64(4), true, "unable to identify message type"},                          // Expect specific internal error message.
		{"Invalid Method Type", []byte(`{"jsonrpc":"2.0","method":123,"id":5}`), "", float64(5), true, "failed to parse method"},                              // Expect specific internal error message.
		{"Empty Method String", []byte(`{"jsonrpc":"2.0","method":"","id":6}`), "", float64(6), true, "method cannot be empty"},
		{"Invalid ID Type (Object)", []byte(`{"jsonrpc":"2.0","method":"test","id":{}}`), "", nil, true, "invalid JSON-RPC ID type detected"}, // Specific internal error from identifyRequestID.
		{"Invalid ID Type (Array)", []byte(`{"jsonrpc":"2.0","method":"test","id":[]}`), "", nil, true, "invalid JSON-RPC ID type detected"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msgType, reqID, err := mw.identifyMessage(tc.input)

			if tc.expectError {
				assert.Error(t, err)
				if err != nil && tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
			} else {
				assert.NoError(t, err)
				if err == nil {
					assert.Equal(t, tc.expectedType, msgType)
					// Handle potential map comparison if needed, though float64/string/nil should be fine.
					assert.Equal(t, tc.expectedID, reqID)
				}
			}
		})
	}
}

// Test Outgoing Validation - Success (Unchanged behavior).
func TestValidationMiddleware_HandleMessage_OutgoingValidation_Success(t *testing.T) {
	mockValidator := &mockSchemaValidator{}
	mockNext := &mockNextHandler{
		responseToReturn: []byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"getTasksV2","inputSchema":{"type":"object"}}]}}`), // Valid response
	}
	opts := DefaultValidationOptions()
	opts.ValidateOutgoing = true
	opts.StrictOutgoing = true
	opts.SkipTypes = make(map[string]bool) // Ensure incoming isn't skipped
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should not error when outgoing validation succeeds.")
	assert.Equal(t, mockNext.responseToReturn, respBytes, "Original response should be returned.")
	require.Equal(t, 2, mockValidator.outgoingCallCount, "Validator should have been called twice (incoming+outgoing).")
	assert.Equal(t, "tools/list_response", mockValidator.lastValidatedType, "Correct schema type used for outgoing validation.")
}

// Test Outgoing Validation - Failure (Strict) (Unchanged behavior).
// Middleware should still return an error, even if generation is elsewhere.
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
		responseToReturn: []byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"INVALID TOOL NAME"}]}}`), // Invalid response structure.
	}
	opts := DefaultValidationOptions()
	opts.ValidateOutgoing = true
	opts.StrictOutgoing = true             // Fail on outgoing validation errors.
	opts.SkipTypes = make(map[string]bool) // Ensure incoming isn't skipped
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	// IMPORTANT: In strict outgoing mode, the middleware now RETURNS the formatted
	// JSON-RPC error response bytes, and a nil error for the Go error return.
	// The internal processing failed validation, but the middleware handled it by creating
	// the error response.
	assert.NoError(t, err, "HandleMessage should NOT return a Go error when outgoing validation fails strictly (it returns error response bytes).")
	require.NotNil(t, respBytes, "An error response should be returned on strict outgoing failure.")

	// Verify the returned JSON-RPC error response structure and code.
	var errorResp struct {
		Error struct {
			Code int         `json:"code"`
			Data interface{} `json:"data"`
		} `json:"error"`
	}
	errUnmarshal := json.Unmarshal(respBytes, &errorResp)
	require.NoError(t, errUnmarshal, "Failed to unmarshal error response.")

	// Expect Invalid Request (-32600) because the error is in the 'result', not 'params'.
	assert.Equal(t, transport.JSONRPCInvalidRequest, errorResp.Error.Code, "Error code should be Invalid Request (-32600).")
	assert.Equal(t, 2, mockValidator.outgoingCallCount, "Validator should have been called twice (incoming+outgoing).")
}

// Test Outgoing Validation - Failure (Non-Strict) (Unchanged behavior).
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
	opts.StrictOutgoing = false            // Do NOT fail on outgoing validation errors.
	opts.SkipTypes = make(map[string]bool) // Ensure incoming isn't skipped
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"someMethod","id":1}`) // Assume someMethod_response schema exists or falls back.
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err, "HandleMessage should NOT error when outgoing validation fails in non-strict mode.")
	assert.Equal(t, mockNext.responseToReturn, respBytes, "Original (invalid) response should still be returned.")
	assert.Equal(t, 2, mockValidator.outgoingCallCount, "Validator should have been called twice (incoming+outgoing).")
}

// Test Outgoing Validation - Skips Error Responses (Unchanged behavior).
func TestValidationMiddleware_HandleMessage_OutgoingValidation_SkipsErrors(t *testing.T) {
	mockValidator := &mockSchemaValidator{shouldFail: true, failOnlyOutgoing: true}
	mockNext := &mockNextHandler{
		// This is a valid JSON-RPC error response structure.
		responseToReturn: []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"Internal"}}`),
	}
	opts := DefaultValidationOptions()
	opts.ValidateOutgoing = true
	opts.StrictOutgoing = true
	opts.SkipTypes = make(map[string]bool) // Ensure incoming isn't skipped
	mw := createTestMiddleware(t, mockValidator, mockNext, opts)

	// Temporarily allow incoming validation to pass for this specific test setup.
	mockValidator.shouldFail = false

	testMsg := []byte(`{"jsonrpc":"2.0","method":"someMethod","id":1}`)
	respBytes, err := mw.HandleMessage(context.Background(), testMsg)

	mockValidator.shouldFail = true // Reset mock state

	assert.NoError(t, err, "HandleMessage should not error when returning an error response.")
	assert.Equal(t, mockNext.responseToReturn, respBytes, "Error response should be returned.")
	// Because outgoing validation is skipped for error responses, only incoming validation occurs.
	assert.Equal(t, 1, mockValidator.outgoingCallCount, "Validator should only have been called once (incoming).")
	// Check that the *last* validation attempt was for the incoming request.
	assert.Equal(t, "someMethod", mockValidator.lastValidatedType, "Last validation should have been for the incoming 'someMethod' request.")
}

// Test Helper: identifyMessage correctly identifies message types from raw bytes (Unchanged Test Logic).
// Duplicated from above, can be removed if desired. Kept for structural consistency.
func TestIdentifyMessageHelper(t *testing.T) {
	mockValidator := &mockSchemaValidator{}
	err := mockValidator.Initialize(context.Background())
	require.NoError(t, err)
	mw := ValidationMiddleware{validator: mockValidator}

	tests := []struct {
		name          string
		input         []byte
		expectedType  string
		expectedID    interface{}
		expectError   bool
		errorContains string
	}{
		{"Valid Request", []byte(`{"jsonrpc":"2.0","method":"ping","id":1}`), "ping", float64(1), false, ""},
		{"Valid Notification", []byte(`{"jsonrpc":"2.0","method":"ping"}`), "ping_notification", nil, false, ""},
		{"Valid Success Response", []byte(`{"jsonrpc":"2.0","result":{"ok":true},"id":2}`), "success_response", float64(2), false, ""},
		{"Valid Error Response", []byte(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid"},"id":3}`), "error_response", float64(3), false, ""},
		{"Invalid JSON", []byte(`{invalid`), "", nil, true, "failed to parse message structure"},
		{"Missing Method/Result/Error", []byte(`{"jsonrpc":"2.0","id":4}`), "", float64(4), true, "unable to identify message type"},
	}

	for _, tc := range tests {
		t.Run(tc.name+"_Helper", func(t *testing.T) { // Append suffix to avoid name clash
			msgType, reqID, err := mw.identifyMessage(tc.input)

			if tc.expectError {
				assert.Error(t, err)
				if err != nil && tc.errorContains != "" {
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
