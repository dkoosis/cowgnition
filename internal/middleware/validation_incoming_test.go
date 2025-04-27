// Package middleware_test tests the middleware components.
package middleware_test

// file: internal/middleware/validation_incoming_test.go

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dkoosis/cowgnition/internal/middleware"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestValidationMiddleware_CallsNextHandler_When_IncomingValidationSucceeds tests that valid incoming messages are passed to the next handler.
func TestValidationMiddleware_CallsNextHandler_When_IncomingValidationSucceeds(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.StrictMode = true
	options.ValidateOutgoing = true // Keep outgoing validation enabled for this test.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc": "2.0", "method": "test_method", "id": 1, "params": {}}`)
	expectedResp := []byte(`{"jsonrpc": "2.0", "id": 1, "result": "passed"}`)
	// --- FIX: Define the expected *result* bytes separately ---
	expectedResultBytes := []byte(`"passed"`) // This is what validateOutgoingResponse will extract.

	// Expect incoming validation to be called and succeed.
	mockValidator.On("Validate", mock.Anything, "test_method", testMsg).Return(nil).Once()
	// Expect next handler to be called after successful validation.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(expectedResp, nil).Once()
	// --- FIX: Expect outgoing validation to be called with the *extracted result* bytes ---
	mockValidator.On("Validate", mock.Anything, "test_method_response", expectedResultBytes).Return(nil).Once()

	// CORRECTED: Call the middleware function mw directly.
	resp, err := mw(mockNextHandler.Handle)(context.Background(), testMsg)

	assert.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
	mockValidator.AssertExpectations(t)
	mockNextHandler.AssertExpectations(t)
}

// TestValidationMiddleware_ReturnsErrorResponse_When_IncomingValidationFailsInStrictMode tests that an error response is returned for invalid incoming messages in strict mode.
func TestValidationMiddleware_ReturnsErrorResponse_When_IncomingValidationFailsInStrictMode(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.StrictMode = true
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc": "2.0", "method": "fail_method", "id": "err-1", "params": "invalid"}`)
	validationErr := schema.NewValidationError(schema.ErrValidationFailed, "Invalid type for params", nil)
	validationErr.InstancePath = "/params"
	validationErr.SchemaPath = "#/properties/params/type"

	// Expect validation to be called and fail.
	mockValidator.On("Validate", mock.Anything, "fail_method", testMsg).Return(validationErr).Once()

	// CORRECTED: Call the middleware function mw directly.
	resp, err := mw(mockNextHandler.Handle)(context.Background(), testMsg)

	// Should not return an error from HandleMessage itself, but response bytes should contain the error.
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	mockNextHandler.AssertNotCalled(t, "Handle", mock.Anything, mock.Anything) // Verify next wasn't called.

	// Assert the response is a valid JSON-RPC error response matching the validation failure.
	var errorResp map[string]interface{}
	err = json.Unmarshal(resp, &errorResp)
	require.NoError(t, err, "Response should be valid JSON.")

	assert.Equal(t, "2.0", errorResp["jsonrpc"])
	assert.Equal(t, "err-1", errorResp["id"]) // ID should match request.
	require.Contains(t, errorResp, "error")
	errObj, ok := errorResp["error"].(map[string]interface{})
	require.True(t, ok, "Error field should be an object.")

	// Code should be InvalidParams because InstancePath starts with /params.
	assert.EqualValues(t, transport.JSONRPCInvalidParams, errObj["code"])
	assert.Equal(t, "Invalid params", errObj["message"])
	require.Contains(t, errObj, "data")
	errData, ok := errObj["data"].(map[string]interface{})
	require.True(t, ok, "Error data field should be an object.")

	assert.Equal(t, "/params", errData["validationPath"])
	assert.Equal(t, "#/properties/params/type", errData["schemaPath"])
	assert.Contains(t, errData["validationError"], "Invalid type for params")

	mockValidator.AssertExpectations(t)
}

// TestValidationMiddleware_CallsNextHandler_When_IncomingValidationFailsInNonStrictMode tests that the next handler is called even for invalid incoming messages in non-strict mode.
func TestValidationMiddleware_CallsNextHandler_When_IncomingValidationFailsInNonStrictMode(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.StrictMode = false // Non-strict mode.
	options.ValidateOutgoing = true
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc": "2.0", "method": "fail_method_nonstrict", "id": 2, "params": "invalid"}`)
	validationErr := schema.NewValidationError(schema.ErrValidationFailed, "Still invalid", nil)
	validationErr.InstancePath = "/params"
	expectedResp := []byte(`{"jsonrpc":"2.0","id":2,"result":"passed_anyway"}`)
	// --- FIX: Define expected result bytes for outgoing validation ---
	expectedResultBytes := []byte(`"passed_anyway"`)

	// Expect validation to be called and fail.
	mockValidator.On("Validate", mock.Anything, "fail_method_nonstrict", testMsg).Return(validationErr).Once()
	// Expect next handler to be called *despite* validation failure.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(expectedResp, nil).Once()
	// --- FIX: Expect outgoing validation with extracted result bytes ---
	mockValidator.On("Validate", mock.Anything, "fail_method_nonstrict_response", expectedResultBytes).Return(nil).Once()

	// CORRECTED: Call the middleware function mw directly.
	resp, err := mw(mockNextHandler.Handle)(context.Background(), testMsg)

	// Should proceed normally, returning the response from the next handler.
	assert.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
	mockValidator.AssertExpectations(t)
	mockNextHandler.AssertExpectations(t)
}
