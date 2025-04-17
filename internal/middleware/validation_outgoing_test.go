// Package middleware_test tests the middleware components.
package middleware_test

// file: internal/middleware/validation_outgoing_test.go

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

func TestValidationMiddleware_HandleMessage_OutgoingValidation_Success(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.ValidateOutgoing = true // Explicitly enable.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc":"2.0", "method":"outgoing_test", "id":10}`)
	responseFromNext := []byte(`{"jsonrpc":"2.0", "id":10, "result":{"status":"all_good"}}`)

	// Incoming validation succeeds.
	mockValidator.On("Validate", mock.Anything, "outgoing_test", testMsg).Return(nil).Once()
	// Next handler returns success response.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(responseFromNext, nil).Once()
	// Outgoing validation succeeds.
	mockValidator.On("Validate", mock.Anything, "outgoing_test_response", responseFromNext).Return(nil).Once()

	resp, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err)
	assert.Equal(t, responseFromNext, resp) // Original response should be returned.
	mockValidator.AssertExpectations(t)
	mockNextHandler.AssertExpectations(t)
}

func TestValidationMiddleware_HandleMessage_OutgoingValidation_Failure_Strict(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.ValidateOutgoing = true
	options.StrictOutgoing = true // Strict outgoing mode.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc":"2.0", "method":"outgoing_fail", "id":11}`)
	responseFromNext := []byte(`{"jsonrpc":"2.0", "id":11, "result":{"status":"actually_bad"}}`)
	outgoingValidationErr := schema.NewValidationError(schema.ErrValidationFailed, "Invalid status value", nil)
	outgoingValidationErr.InstancePath = "/result/status"
	outgoingValidationErr.SchemaPath = "#/properties/result/properties/status/enum"

	// Incoming validation succeeds.
	mockValidator.On("Validate", mock.Anything, "outgoing_fail", testMsg).Return(nil).Once()
	// Next handler returns the "bad" response.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(responseFromNext, nil).Once()
	// Outgoing validation fails.
	mockValidator.On("Validate", mock.Anything, "outgoing_fail_response", responseFromNext).Return(outgoingValidationErr).Once()

	resp, err := mw.HandleMessage(context.Background(), testMsg)

	// HandleMessage should not return an error itself.
	assert.NoError(t, err)
	assert.NotNil(t, resp)

	// Assert response is a JSON-RPC error response reflecting the *outgoing* validation failure.
	var errorResp map[string]interface{}
	err = json.Unmarshal(resp, &errorResp)
	require.NoError(t, err, "Response should be valid JSON")

	assert.Equal(t, "2.0", errorResp["jsonrpc"])
	require.IsType(t, float64(0), errorResp["id"], "ID should be a number")
	assert.EqualValues(t, 11, errorResp["id"]) // ID should match original request.
	require.Contains(t, errorResp, "error")
	errObj, ok := errorResp["error"].(map[string]interface{})
	require.True(t, ok)

	// Should be Invalid Request (-32600) because the error is in the 'result'.
	assert.EqualValues(t, transport.JSONRPCInvalidRequest, errObj["code"])
	assert.Equal(t, "Invalid Request", errObj["message"])
	require.Contains(t, errObj, "data")
	errData, ok := errObj["data"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, "/result/status", errData["validationPath"])
	assert.Equal(t, "#/properties/result/properties/status/enum", errData["schemaPath"])
	assert.Contains(t, errData["validationError"], "Invalid status value")

	mockValidator.AssertExpectations(t)
	mockNextHandler.AssertExpectations(t)
}

func TestValidationMiddleware_HandleMessage_OutgoingValidation_Failure_NonStrict(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.ValidateOutgoing = true
	options.StrictOutgoing = false // Non-strict outgoing mode.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc":"2.0", "method":"outgoing_nonstrict", "id":12}`)
	responseFromNext := []byte(`{"jsonrpc":"2.0", "id":12, "result":{"status":"bad_but_ignored"}}`)
	outgoingValidationErr := schema.NewValidationError(schema.ErrValidationFailed, "Still invalid status", nil)
	outgoingValidationErr.InstancePath = "/result/status"

	// Incoming validation succeeds.
	mockValidator.On("Validate", mock.Anything, "outgoing_nonstrict", testMsg).Return(nil).Once()
	// Next handler returns the "bad" response.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(responseFromNext, nil).Once()
	// Outgoing validation fails.
	mockValidator.On("Validate", mock.Anything, "outgoing_nonstrict_response", responseFromNext).Return(outgoingValidationErr).Once()

	resp, err := mw.HandleMessage(context.Background(), testMsg)

	// In non-strict outgoing mode, the original response from 'next' should be returned despite validation failure.
	assert.NoError(t, err)
	assert.Equal(t, responseFromNext, resp)
	mockValidator.AssertExpectations(t)
	mockNextHandler.AssertExpectations(t)
}
