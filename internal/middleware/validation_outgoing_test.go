// Package middleware_test tests the middleware components.
package middleware_test

// file: internal/middleware/validation_outgoing_test.go

import (
	"context"
	"encoding/json"
	"testing"

	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Added import for mcptypes.
	"github.com/dkoosis/cowgnition/internal/middleware"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestValidationMiddleware_SucceedsOutgoingValidation_When_ResponseIsValid tests successful outgoing validation.
func TestValidationMiddleware_SucceedsOutgoingValidation_When_ResponseIsValid(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.ValidateOutgoing = true // Explicitly enable.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc":"2.0", "method":"outgoing_test", "id":10}`)
	responseFromNext := []byte(`{"jsonrpc":"2.0", "id":10, "result":{"status":"all_good"}}`)
	// --- FIX: Define expected result bytes separately ---
	expectedResultBytes := []byte(`{"status":"all_good"}`)

	// Incoming validation succeeds.
	mockValidator.On("Validate", mock.Anything, "outgoing_test", testMsg).Return(nil).Once()
	// Next handler returns success response.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(responseFromNext, nil).Once()
	// --- FIX: Expect outgoing validation with extracted result bytes ---
	mockValidator.On("Validate", mock.Anything, "outgoing_test_response", expectedResultBytes).Return(nil).Once()

	// CORRECTED: Call the middleware function mw directly.
	resp, err := mw(mockNextHandler.Handle)(context.Background(), testMsg)

	assert.NoError(t, err)
	assert.Equal(t, responseFromNext, resp) // Original response should be returned.
	mockValidator.AssertExpectations(t)
	mockNextHandler.AssertExpectations(t)
}

// TestValidationMiddleware_ReturnsErrorResponse_When_OutgoingValidationFailsInStrictMode tests outgoing validation failure in strict mode.
func TestValidationMiddleware_ReturnsErrorResponse_When_OutgoingValidationFailsInStrictMode(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.ValidateOutgoing = true
	options.StrictOutgoing = true // Strict outgoing mode.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc":"2.0", "method":"outgoing_fail", "id":11}`)
	responseFromNext := []byte(`{"jsonrpc":"2.0", "id":11, "result":{"status":"actually_bad"}}`)
	// --- FIX: Define expected result bytes for outgoing validation ---
	expectedResultBytes := []byte(`{"status":"actually_bad"}`)
	outgoingValidationErr := schema.NewValidationError(schema.ErrValidationFailed, "Invalid status value", nil)
	outgoingValidationErr.InstancePath = "/result/status"
	outgoingValidationErr.SchemaPath = "#/properties/result/properties/status/enum"

	// Incoming validation succeeds.
	mockValidator.On("Validate", mock.Anything, "outgoing_fail", testMsg).Return(nil).Once()
	// Next handler returns the "bad" response.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(responseFromNext, nil).Once()
	// --- FIX: Expect outgoing validation with extracted result bytes ---
	mockValidator.On("Validate", mock.Anything, "outgoing_fail_response", expectedResultBytes).Return(outgoingValidationErr).Once()

	// CORRECTED: Call the middleware function mw directly.
	resp, err := mw(mockNextHandler.Handle)(context.Background(), testMsg)

	// HandleMessage should not return an error itself.
	assert.NoError(t, err)
	assert.NotNil(t, resp)

	// Assert response is a JSON-RPC error response reflecting the *outgoing* validation failure.
	var errorResp mcptypes.JSONRPCErrorContainer // Use type from mcptypes.
	err = json.Unmarshal(resp, &errorResp)
	require.NoError(t, err, "Response should be valid JSON.")

	assert.Equal(t, "2.0", errorResp.JSONRPC)
	// ID should match original request. json.RawMessage comparison needs care.
	assert.JSONEq(t, `11`, string(errorResp.ID)) // Compare as JSON number.
	require.NotNil(t, errorResp.Error)

	// Should be Invalid Request (-32600) because the error is in the 'result'.
	assert.EqualValues(t, transport.JSONRPCInvalidRequest, errorResp.Error.Code)
	assert.Equal(t, "Invalid Request", errorResp.Error.Message) // Corrected expected message.
	require.NotNil(t, errorResp.Error.Data)
	errData, ok := errorResp.Error.Data.(map[string]interface{})
	require.True(t, ok)

	// Paths in data might be relative to the validated *result* now, not the full response.
	assert.Equal(t, "/status", errData["validationPath"])                                // Path within the validated result object.
	assert.Equal(t, "#/properties/result/properties/status/enum", errData["schemaPath"]) // Schema path remains the same.
	assert.Contains(t, errData["validationError"], "Invalid status value")

	mockValidator.AssertExpectations(t)
	mockNextHandler.AssertExpectations(t)
}

// TestValidationMiddleware_ReturnsOriginalResponse_When_OutgoingValidationFailsInNonStrictMode tests outgoing validation failure in non-strict mode.
func TestValidationMiddleware_ReturnsOriginalResponse_When_OutgoingValidationFailsInNonStrictMode(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.ValidateOutgoing = true
	options.StrictOutgoing = false // Non-strict outgoing mode.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc":"2.0", "method":"outgoing_nonstrict", "id":12}`)
	responseFromNext := []byte(`{"jsonrpc":"2.0", "id":12, "result":{"status":"bad_but_ignored"}}`)
	// --- FIX: Define expected result bytes for outgoing validation ---
	expectedResultBytes := []byte(`{"status":"bad_but_ignored"}`)
	outgoingValidationErr := schema.NewValidationError(schema.ErrValidationFailed, "Still invalid status", nil)
	outgoingValidationErr.InstancePath = "/result/status"

	// Incoming validation succeeds.
	mockValidator.On("Validate", mock.Anything, "outgoing_nonstrict", testMsg).Return(nil).Once()
	// Next handler returns the "bad" response.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(responseFromNext, nil).Once()
	// --- FIX: Expect outgoing validation with extracted result bytes ---
	mockValidator.On("Validate", mock.Anything, "outgoing_nonstrict_response", expectedResultBytes).Return(outgoingValidationErr).Once()

	// CORRECTED: Call the middleware function mw directly.
	resp, err := mw(mockNextHandler.Handle)(context.Background(), testMsg)

	// In non-strict outgoing mode, the original response from 'next' should be returned despite validation failure.
	assert.NoError(t, err)
	assert.Equal(t, responseFromNext, resp)
	mockValidator.AssertExpectations(t)
	mockNextHandler.AssertExpectations(t)
}
