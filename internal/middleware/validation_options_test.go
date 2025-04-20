// Package middleware_test tests the middleware components.
package middleware_test

// file: internal/middleware/validation_options_test.go.

import (
	"context"
	"testing"

	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/middleware"

	// Corrected: Need schema import if interface is used directly, though likely through mock.
	// "github.com/dkoosis/cowgnition/internal/schema".
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestValidationMiddleware_HandleMessage_ValidationDisabled(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.Enabled = false // Disable validation.
	mw, _, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"test"}`)
	expectedResp := []byte(`{"result":"ok"}`)

	// Expect the next handler's Handle method to be called directly.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(expectedResp, nil).Once()

	resp, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
	mockNextHandler.AssertExpectations(t) // Verify next handler was called.
}

func TestValidationMiddleware_HandleMessage_ValidatorNotInitialized(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.Enabled = true
	// Don't call setupTestMiddleware which initializes. Create manually.
	logger := logging.GetNoopLogger()
	// Corrected: Use NewMockValidator.
	mockValidator := NewMockValidator()
	mockNextHandler := new(MockMessageHandler)

	// Explicitly keep validator uninitialized.
	mockValidator.initialized = false
	// Expectation still needed for the mock framework, even if not asserted directly.
	// Ensure mock responds correctly to IsInitialized call.
	mockValidator.On("IsInitialized").Return(false).Once()

	mw := middleware.NewValidationMiddleware(mockValidator, options, logger)
	mw.SetNext(mockNextHandler.Handle)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"test"}`)
	expectedResp := []byte(`{"result":"ok"}`)

	// Expect the next handler to be called directly because validator is not initialized.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(expectedResp, nil).Once()

	resp, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
	// Check if IsInitialized was called as expected.
	mockValidator.AssertCalled(t, "IsInitialized")
	mockNextHandler.AssertExpectations(t) // Verify next handler was called.
}

func TestValidationMiddleware_HandleMessage_SkipType(t *testing.T) {
	options := middleware.DefaultValidationOptions()
	options.SkipTypes["ping"] = true // Ensure ping is skipped.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc": "2.0", "method": "ping", "id": "ping-1"}`)
	expectedResp := []byte(`{"jsonrpc":"2.0","id":"ping-1","result":"pong"}`)

	// Expect validation NOT to be called for incoming "ping".
	// Expect next handler to be called.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(expectedResp, nil).Once()

	// --- FIX: Expect outgoing validation to use the fallback schema for ping response ---.
	// Expect outgoing validation *to be* called for the response (unless skipped).
	// Assuming 'ping_response' schema doesn't exist or isn't found by determineOutgoingSchemaType,
	// it will likely fall back to "JSONRPCResponse". Adjust if your fallback logic differs.
	mockValidator.On("Validate", mock.Anything, "JSONRPCResponse", expectedResp).Return(nil).Once()
	// --- End FIX ---.

	resp, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
	// Assert Validate was NOT called for the incoming "ping" schema.
	calledValidate := false
	for _, call := range mockValidator.Calls {
		if call.Method == "Validate" && len(call.Arguments) > 1 && call.Arguments.String(1) == "ping" {
			calledValidate = true
			break
		}
	}
	assert.False(t, calledValidate, "Validate should not have been called for incoming 'ping'.")
	mockNextHandler.AssertExpectations(t)

	// --- FIX: Assert Validate *was* called for the outgoing "JSONRPCResponse" ---.
	mockValidator.AssertCalled(t, "Validate", mock.Anything, "JSONRPCResponse", expectedResp)
	// --- End FIX ---.
}
