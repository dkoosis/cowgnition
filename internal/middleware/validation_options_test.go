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

// TestValidationMiddleware_SkipsProcessing_When_OptionIsDisabled tests that no validation occurs if Enabled=false.
func TestValidationMiddleware_SkipsProcessing_When_OptionIsDisabled(t *testing.T) {
	t.Log("Testing ValidationMiddleware: Skips processing when validation is disabled.")
	options := middleware.DefaultValidationOptions()
	options.Enabled = false // Disable validation.
	// Use setupTestMiddleware which sets up mocks correctly.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc":"2.0","method":"test"}`)
	expectedResp := []byte(`{"result":"ok"}`)

	// Expect the next handler's Handle method to be called directly.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(expectedResp, nil).Once()

	resp, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err)
	assert.Equal(t, expectedResp, resp)
	// Verify no validation method was called.
	mockValidator.AssertNotCalled(t, "Validate", mock.Anything, mock.Anything, mock.Anything)
	mockNextHandler.AssertExpectations(t) // Verify next handler was called.
}

// TestValidationMiddleware_SkipsProcessing_When_ValidatorNotInitialized tests skipping validation if validator isn't ready.
func TestValidationMiddleware_SkipsProcessing_When_ValidatorNotInitialized(t *testing.T) {
	t.Log("Testing ValidationMiddleware: Skips processing when validator is not initialized.")
	options := middleware.DefaultValidationOptions()
	options.Enabled = true // Ensure validation is generally enabled.
	// Don't call setupTestMiddleware which initializes. Create manually.
	logger := logging.GetNoopLogger()
	mockValidator := NewMockValidator() // Uses the mock from validation_mocks_test.go.
	mockNextHandler := new(MockMessageHandler)

	// Explicitly keep validator uninitialized.
	mockValidator.initialized = false
	// Expect IsInitialized to be called and return false.
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
	// Verify Validate was NOT called.
	mockValidator.AssertNotCalled(t, "Validate", mock.Anything, mock.Anything, mock.Anything)
	mockNextHandler.AssertExpectations(t) // Verify next handler was called.
}

// TestValidationMiddleware_SkipsIncomingValidation_When_TypeIsSkipped tests skipping validation for specific message types.
func TestValidationMiddleware_SkipsIncomingValidation_When_TypeIsSkipped(t *testing.T) {
	t.Log("Testing ValidationMiddleware: Skips incoming validation when message type is in SkipTypes map.")
	options := middleware.DefaultValidationOptions()
	options.SkipTypes["ping"] = true // Ensure ping is skipped for incoming.
	options.ValidateOutgoing = true  // Keep outgoing validation enabled for this test.
	// Use standard setup which initializes the validator.
	mw, mockValidator, mockNextHandler := setupTestMiddleware(t, options)

	testMsg := []byte(`{"jsonrpc": "2.0", "method": "ping", "id": "ping-1"}`)
	expectedResp := []byte(`{"jsonrpc":"2.0","id":"ping-1","result":"pong"}`)

	// Expect next handler to be called.
	mockNextHandler.On("Handle", mock.Anything, testMsg).Return(expectedResp, nil).Once()

	// Expect outgoing validation to use the fallback schema for ping response.
	// Assuming 'ping_response' schema doesn't exist, it will likely fall back to "JSONRPCResponse".
	mockValidator.On("Validate", mock.Anything, "JSONRPCResponse", expectedResp).Return(nil).Once()

	resp, err := mw.HandleMessage(context.Background(), testMsg)

	assert.NoError(t, err)
	assert.Equal(t, expectedResp, resp)

	// Assert Validate was NOT called for the *incoming* message with schema type "ping".
	incomingValidateCalled := false
	for _, call := range mockValidator.Calls {
		if call.Method == "Validate" && len(call.Arguments) > 1 {
			// Check if the second argument (schemaKey) is "ping".
			schemaKeyArg, ok := call.Arguments.Get(1).(string)
			if ok && schemaKeyArg == "ping" {
				incomingValidateCalled = true
				break
			}
		}
	}
	assert.False(t, incomingValidateCalled, "Validate should not have been called for incoming message type 'ping'.")

	// Assert Validate *was* called for the *outgoing* response validation.
	mockValidator.AssertCalled(t, "Validate", mock.Anything, "JSONRPCResponse", expectedResp)
	mockNextHandler.AssertExpectations(t)
}
