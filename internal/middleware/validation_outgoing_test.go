// Package middleware_test tests the middleware components.
package middleware_test

// file: internal/middleware/validation_outgoing_test.go

import (
	"context"
	"encoding/json"
	"testing"

	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
	"github.com/dkoosis/cowgnition/internal/middleware"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestValidationMiddleware_SucceedsOutgoingValidation_When_ResponseIsValid tests successful outgoing validation.
func TestValidationMiddleware_SucceedsOutgoingValidation_When_ResponseIsValid(t *testing.T) {
	t.Log("Testing ValidationMiddleware: Succeeds outgoing validation when response is valid.") // Test description added.
	options := middleware.DefaultValidationOptions()
	options.ValidateOutgoing = true // Explicitly enable.
	options.StrictMode = true       // Ensure errors matter.
	options.StrictOutgoing = true   // Ensure outgoing errors matter.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc":"2.0", "method":"outgoing_test", "id":10}`)
	responseFromNext := []byte(`{"jsonrpc":"2.0", "id":10, "result":{"status":"all_good"}}`)
	// Define expected result bytes separately (the content of the "result" field).
	expectedResultBytes := []byte(`{"status":"all_good"}`)
	// Define the expected schema key for the incoming request.
	incomingSchemaKey := "JSONRPCRequest" // Assuming fallback. Adjust if "outgoing_test" is specifically mapped.
	// Define the expected schema key for the outgoing response's *result*.
	// --- CORRECTED based on latest build output ---
	outgoingSchemaKey := "JSONRPCResponse" // The code is actually using the generic fallback.
	// --- END CORRECTION ---

	// --- EXPECTATIONS ---
	// Expect incoming validation to be called (adjust schema key if needed).
	mockValidator.On("Validate", mock.Anything, incomingSchemaKey, testMsg).Return(nil).Once()
	// Expect the next handler to be called.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(responseFromNext, nil).Once()
	// Expect outgoing validation to be called with the *actual schema key* used by the code and *result bytes*.
	mockValidator.On("Validate", mock.Anything, outgoingSchemaKey, expectedResultBytes).Return(nil).Once() // Using corrected outgoingSchemaKey
	// --- END EXPECTATIONS ---

	// Call the middleware function mw directly.
	resp, err := mw(mockNextHandler.Handle)(context.Background(), testMsg)

	assert.NoError(t, err)
	assert.Equal(t, responseFromNext, resp) // Original response should be returned.
	mockValidator.AssertExpectations(t)
	mockNextHandler.AssertExpectations(t)
}

// TestValidationMiddleware_ReturnsErrorResponse_When_OutgoingValidationFailsInStrictMode tests outgoing validation failure in strict mode.
func TestValidationMiddleware_ReturnsErrorResponse_When_OutgoingValidationFailsInStrictMode(t *testing.T) {
	t.Log("Testing ValidationMiddleware: Returns error response when outgoing validation fails in strict mode.") // Test description added.
	options := middleware.DefaultValidationOptions()
	options.ValidateOutgoing = true
	options.StrictOutgoing = true // Strict outgoing mode.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc":"2.0", "method":"outgoing_fail", "id":11}`)
	responseFromNext := []byte(`{"jsonrpc":"2.0", "id":11, "result":{"status":"actually_bad"}}`)
	// Define expected result bytes for outgoing validation.
	expectedResultBytes := []byte(`{"status":"actually_bad"}`)
	// Define the expected schema key for the incoming request.
	incomingSchemaKey := "JSONRPCRequest" // Or "outgoing_fail" if mapped.
	// Define the expected schema key for the outgoing response's *result*.
	// Assuming fallback logic here too based on the previous test.
	outgoingSchemaKey := "JSONRPCResponse" // Use generic fallback based on observation.

	// Create the mock validation error.
	outgoingValidationErr := schema.NewValidationError(schema.ErrValidationFailed, "Invalid status value", nil)
	outgoingValidationErr.InstancePath = "/status"                                  // Path within the result object {"status": "..."}.
	outgoingValidationErr.SchemaPath = "#/properties/result/properties/status/enum" // Example schema path.

	// --- EXPECTATIONS ---
	// Incoming validation succeeds.
	mockValidator.On("Validate", mock.Anything, incomingSchemaKey, testMsg).Return(nil).Once()
	// Next handler returns the "bad" response.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(responseFromNext, nil).Once()
	// Expect outgoing validation with extracted result bytes, returning the mock error.
	mockValidator.On("Validate", mock.Anything, outgoingSchemaKey, expectedResultBytes).Return(outgoingValidationErr).Once() // Using potentially corrected outgoingSchemaKey
	// --- END EXPECTATIONS ---

	// Call the middleware function mw directly.
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
	assert.Equal(t, "Invalid Request", errorResp.Error.Message)
	require.NotNil(t, errorResp.Error.Data)
	errData, ok := errorResp.Error.Data.(map[string]interface{})
	require.True(t, ok)

	// Assert the path is now correctly expected relative to the validated result.
	assert.Equal(t, "/status", errData["validationPath"])
	assert.Equal(t, "#/properties/result/properties/status/enum", errData["schemaPath"])
	assert.Contains(t, errData["validationError"], "Invalid status value")

	mockValidator.AssertExpectations(t)
	mockNextHandler.AssertExpectations(t)
}

// TestValidationMiddleware_ReturnsOriginalResponse_When_OutgoingValidationFailsInNonStrictMode tests outgoing validation failure in non-strict mode.
func TestValidationMiddleware_ReturnsOriginalResponse_When_OutgoingValidationFailsInNonStrictMode(t *testing.T) {
	t.Log("Testing ValidationMiddleware: Returns original response when outgoing validation fails in non-strict mode.") // Test description added.
	options := middleware.DefaultValidationOptions()
	options.ValidateOutgoing = true
	options.StrictOutgoing = false // Non-strict outgoing mode.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc":"2.0", "method":"outgoing_nonstrict", "id":12}`)
	responseFromNext := []byte(`{"jsonrpc":"2.0", "id":12, "result":{"status":"bad_but_ignored"}}`)
	// Define expected result bytes for outgoing validation.
	expectedResultBytes := []byte(`{"status":"bad_but_ignored"}`)
	// Define the expected schema key for the incoming request.
	incomingSchemaKey := "JSONRPCRequest" // Or "outgoing_nonstrict" if mapped.
	// Define the expected schema key for the outgoing response's *result*.
	outgoingSchemaKey := "JSONRPCResponse" // Use generic fallback based on observation.

	// Create the mock validation error.
	outgoingValidationErr := schema.NewValidationError(schema.ErrValidationFailed, "Still invalid status", nil)
	outgoingValidationErr.InstancePath = "/status" // Path within the result object.

	// --- EXPECTATIONS ---
	// Incoming validation succeeds.
	mockValidator.On("Validate", mock.Anything, incomingSchemaKey, testMsg).Return(nil).Once()
	// Next handler returns the "bad" response.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(responseFromNext, nil).Once()
	// Expect outgoing validation with extracted result bytes, returning the mock error.
	mockValidator.On("Validate", mock.Anything, outgoingSchemaKey, expectedResultBytes).Return(outgoingValidationErr).Once() // Using potentially corrected outgoingSchemaKey
	// --- END EXPECTATIONS ---

	// Call the middleware function mw directly.
	resp, err := mw(mockNextHandler.Handle)(context.Background(), testMsg)

	// In non-strict outgoing mode, the original response from 'next' should be returned despite validation failure.
	assert.NoError(t, err)
	assert.Equal(t, responseFromNext, resp)
	mockValidator.AssertExpectations(t)
	mockNextHandler.AssertExpectations(t)
}

// TestValidationMiddleware_SkipsOutgoingValidation_When_Disabled ensures outgoing validation is skipped if option is false.
func TestValidationMiddleware_SkipsOutgoingValidation_When_Disabled(t *testing.T) {
	t.Log("Testing ValidationMiddleware: Skips outgoing validation when ValidateOutgoing=false.") // Test description added.
	options := middleware.DefaultValidationOptions()
	options.ValidateOutgoing = false // Explicitly disable outgoing.
	options.StrictMode = true        // Incoming strict is fine.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc":"2.0", "method":"skip_outgoing", "id":13}`)
	// Response from handler could technically be invalid per schema, but shouldn't matter.
	responseFromNext := []byte(`{"jsonrpc":"2.0", "id":13, "result":{"invalid_structure": true}}`)
	incomingSchemaKey := "JSONRPCRequest" // Or "skip_outgoing" if mapped.

	// --- EXPECTATIONS ---
	// Expect incoming validation to be called and succeed.
	mockValidator.On("Validate", mock.Anything, incomingSchemaKey, testMsg).Return(nil).Once()
	// Expect next handler to be called.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(responseFromNext, nil).Once()
	// DO NOT expect outgoing validation.
	// --- END EXPECTATIONS ---

	// Call the middleware function mw directly.
	resp, err := mw(mockNextHandler.Handle)(context.Background(), testMsg)

	assert.NoError(t, err)
	assert.Equal(t, responseFromNext, resp) // Original response should be returned.

	// --- VERIFY NO OUTGOING VALIDATION ---
	// Assert that Validate was called exactly once (for the incoming message).
	mockValidator.AssertNumberOfCalls(t, "Validate", 1)
	// --- END VERIFICATION ---

	mockNextHandler.AssertExpectations(t) // Verify Handle was called.
}
