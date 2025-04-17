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

func TestValidationMiddleware_HandleMessage_IncomingValidation_Success(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.StrictMode = true
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc": "2.0", "method": "test_method", "id": 1, "params": {}}`)
	expectedResp := []byte(`{"jsonrpc": "2.0", "id": 1, "result": "passed"}`)

	// Expect validation to be called and succeed.
	mockValidator.On("Validate", mock.Anything, "test_method", testMsg).Return(nil).Once()
	// Expect next handler to be called after successful validation.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(expectedResp, nil).Once()
	// Expect outgoing validation to be called (and succeed implicitly by default mock).
	mockValidator.On("Validate", mock.Anything, "test_method_response", expectedResp).Return(nil).Once()

	resp, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
	mockValidator.AssertExpectations(t)
	mockNextHandler.AssertExpectations(t)
}

func TestValidationMiddleware_HandleMessage_IncomingValidation_Failure_Strict(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.StrictMode = true
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc": "2.0", "method": "fail_method", "id": "err-1", "params": "invalid"}`)
	validationErr := schema.NewValidationError(schema.ErrValidationFailed, "Invalid type for params", nil)
	validationErr.InstancePath = "/params"
	validationErr.SchemaPath = "#/properties/params/type"

	// Expect validation to be called and fail.
	mockValidator.On("Validate", mock.Anything, "fail_method", testMsg).Return(validationErr).Once()

	resp, err := mw.HandleMessage(context.Background(), testMsg)

	// Should not return an error from HandleMessage itself, but response bytes should contain the error.
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	mockNextHandler.AssertNotCalled(t, "Handle", mock.Anything, mock.Anything) // Verify next wasn't called.

	// Assert the response is a valid JSON-RPC error response matching the validation failure.
	var errorResp map[string]interface{}
	err = json.Unmarshal(resp, &errorResp)
	require.NoError(t, err, "Response should be valid JSON")

	assert.Equal(t, "2.0", errorResp["jsonrpc"])
	assert.Equal(t, "err-1", errorResp["id"]) // ID should match request.
	require.Contains(t, errorResp, "error")
	errObj, ok := errorResp["error"].(map[string]interface{})
	require.True(t, ok, "Error field should be an object")

	// Code should be InvalidParams because InstancePath starts with /params.
	assert.EqualValues(t, transport.JSONRPCInvalidParams, errObj["code"])
	assert.Equal(t, "Invalid params", errObj["message"])
	require.Contains(t, errObj, "data")
	errData, ok := errObj["data"].(map[string]interface{})
	require.True(t, ok, "Error data field should be an object")

	assert.Equal(t, "/params", errData["validationPath"])
	assert.Equal(t, "#/properties/params/type", errData["schemaPath"])
	assert.Contains(t, errData["validationError"], "Invalid type for params")

	mockValidator.AssertExpectations(t)
}

func TestValidationMiddleware_HandleMessage_IncomingValidation_Failure_NonStrict(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.StrictMode = false // Non-strict mode.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc": "2.0", "method": "fail_method_nonstrict", "id": 2, "params": "invalid"}`)
	validationErr := schema.NewValidationError(schema.ErrValidationFailed, "Still invalid", nil)
	validationErr.InstancePath = "/params"
	expectedResp := []byte(`{"jsonrpc":"2.0","id":2,"result":"passed_anyway"}`)

	// Expect validation to be called and fail.
	mockValidator.On("Validate", mock.Anything, "fail_method_nonstrict", testMsg).Return(validationErr).Once()
	// Expect next handler to be called *despite* validation failure.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(expectedResp, nil).Once()
	// Expect outgoing validation.
	mockValidator.On("Validate", mock.Anything, "fail_method_nonstrict_response", expectedResp).Return(nil).Once()

	resp, err := mw.HandleMessage(context.Background(), testMsg)

	// Should proceed normally, returning the response from the next handler.
	assert.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
	mockValidator.AssertExpectations(t)
	mockNextHandler.AssertExpectations(t)
}
