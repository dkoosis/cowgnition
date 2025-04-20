// Package middleware_test tests the middleware components.
package middleware_test

// file: internal/middleware/validation_mocks_test.go.

import (
	"context"
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/logging"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
	"github.com/dkoosis/cowgnition/internal/middleware"

	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---.

// MockValidator is a mock implementation of ValidatorInterface.
type MockValidator struct {
	mock.Mock
	initialized      bool
	initErr          error
	schemas          map[string]bool
	validationErrors map[string]error // Map schemaKey -> error to return.
}

// Ensure mock implements the interface (using the imported schema package).
var _ schema.ValidatorInterface = (*MockValidator)(nil)

func NewMockValidator() *MockValidator {
	return &MockValidator{
		initialized:      false,
		schemas:          make(map[string]bool),
		validationErrors: make(map[string]error),
	}
}

func (m *MockValidator) Initialize(ctx context.Context) error {
	m.Called(ctx)
	if m.initErr == nil {
		m.initialized = true
	}
	// Simulate adding base schemas on successful init.
	if m.initialized {
		m.schemas["base"] = true
		m.schemas["JSONRPCRequest"] = true
		m.schemas["JSONRPCResponse"] = true
		m.schemas["JSONRPCNotification"] = true
		m.schemas["JSONRPCError"] = true
		m.schemas["request"] = true          // Generic fallback.
		m.schemas["notification"] = true     // Generic fallback.
		m.schemas["success_response"] = true // Generic fallback.
		m.schemas["error_response"] = true   // Generic fallback.
		// --- Add Schemas Used by TestIdentifyMessage ---.
		m.schemas["initialize"] = true
		m.schemas["tools/list"] = true
		m.schemas["ping"] = true
		// --- End Added Schemas ---.
		// Add schemas used in other tests (relevant ones moved to specific test files if needed).
		m.schemas["test_method"] = true
		m.schemas["test_method_response"] = true
		m.schemas["fail_method"] = true
		m.schemas["fail_method_nonstrict"] = true
		m.schemas["fail_method_nonstrict_response"] = true
		m.schemas["outgoing_test"] = true
		m.schemas["outgoing_test_response"] = true
		m.schemas["outgoing_fail"] = true
		m.schemas["outgoing_fail_response"] = true
		m.schemas["outgoing_nonstrict"] = true
		m.schemas["outgoing_nonstrict_response"] = true
		m.schemas["someMethod"] = true
		m.schemas["someMethod_response"] = true
		m.schemas["tools/list_response"] = true // Ensure this exists for tests that might use it.
		m.schemas["CallToolResult"] = true
		m.schemas["notifications/progress"] = true
	}
	return m.initErr
}

func (m *MockValidator) IsInitialized() bool {
	args := m.Called()
	// Allow explicit return value or fallback to internal state.
	if len(args) > 0 {
		return args.Bool(0)
	}
	return m.initialized
}

func (m *MockValidator) Validate(ctx context.Context, schemaKey string, data []byte) error {
	args := m.Called(ctx, schemaKey, data)
	if err, ok := m.validationErrors[schemaKey]; ok {
		return err // Return predefined error for this key.
	}
	return args.Error(0) // Return error configured via .Return() if any.
}

func (m *MockValidator) HasSchema(name string) bool {
	args := m.Called(name)
	// Allow dynamic checking based on mock's internal state or specific returns.
	if len(args) > 0 && args.Get(0) != nil { // Check if a specific return was configured.
		retVal, ok := args.Get(0).(bool)
		if ok {
			return retVal
		}
	}
	// Fallback to internal map if no specific return was configured for HasSchema.
	_, exists := m.schemas[name]
	return exists
}

func (m *MockValidator) GetLoadDuration() time.Duration {
	args := m.Called()
	// Return a mock duration, can be configured via .Return().
	if len(args) > 0 {
		if duration, ok := args.Get(0).(time.Duration); ok {
			return duration
		}
	}
	return 1 * time.Millisecond // Default mock duration.
}

func (m *MockValidator) GetCompileDuration() time.Duration {
	args := m.Called()
	// Return a mock duration, can be configured via .Return().
	if len(args) > 0 {
		if duration, ok := args.Get(0).(time.Duration); ok {
			return duration
		}
	}
	return 1 * time.Millisecond // Default mock duration.
}

func (m *MockValidator) GetSchemaVersion() string {
	args := m.Called()
	// Return a mock version, can be configured via .Return().
	if len(args) > 0 && args.String(0) != "" {
		return args.String(0)
	}
	return "mock-schema-v0.0.0" // Default mock version.
}

func (m *MockValidator) Shutdown() error {
	args := m.Called()
	m.initialized = false // Simulate shutdown state.
	m.schemas = nil
	return args.Error(0)
}

// Helper to add schemas to the mock.
func (m *MockValidator) AddSchema(name string) {
	if m.schemas == nil {
		m.schemas = make(map[string]bool)
	}
	m.schemas[name] = true
}

// Helper to set a specific validation error for a schema key.
func (m *MockValidator) SetValidationError(schemaKey string, err error) {
	if m.validationErrors == nil {
		m.validationErrors = make(map[string]error)
	}
	m.validationErrors[schemaKey] = err
}

// MockMessageHandler struct remains the same.
type MockMessageHandler struct {
	mock.Mock
}

// Handle method signature matches the transport.MessageHandler function type.
func (m *MockMessageHandler) Handle(ctx context.Context, message []byte) ([]byte, error) {
	args := m.Called(ctx, message)
	resBytes, _ := args.Get(0).([]byte)
	return resBytes, args.Error(1)
}

// --- Test Setup ---.

// setupTestMiddleware initializes mocks and ensures mock validator is initialized.
func setupTestMiddleware(t *testing.T, options mcptypes.ValidationOptions) (mcptypes.MiddlewareFunc, *MockValidator, *MockMessageHandler) {
	t.Helper()
	logger := logging.GetNoopLogger() // Use NoopLogger for tests unless logging is tested.
	mockValidator := NewMockValidator()
	mockNextHandler := new(MockMessageHandler)

	// Basic mock setup for initialization and schema presence checks.
	mockValidator.On("Initialize", mock.Anything).Return(nil).Maybe()
	// NOTE: IsInitialized expectation will be set *after* Initialize is called below.

	mockValidator.On("HasSchema", mock.AnythingOfType("string")).Maybe().Return(func(name string) bool { // Keep dynamic check based on internal map.
		_, exists := mockValidator.schemas[name]
		return exists
	})
	mockValidator.On("GetSchemaVersion").Return("mock-schema-v0.0.0").Maybe()
	mockValidator.On("GetLoadDuration").Return(1 * time.Millisecond).Maybe()
	mockValidator.On("GetCompileDuration").Return(1 * time.Millisecond).Maybe()
	mockValidator.On("Shutdown").Return(nil).Maybe()

	// --- Add explicit Initialize call ---.
	// Initialize the validator state within the test setup.
	err := mockValidator.Initialize(context.Background())
	require.NoError(t, err, "Mock validator initialization failed in test setup.")
	// --- End Add ---.

	// --- Add explicit IsInitialized expectation *after* Initialize ---.
	// Ensure IsInitialized mock behavior is set correctly after Initialize is called.
	mockValidator.On("IsInitialized").Return(true) // Now expect it to be true.
	// --- End Add ---.

	// Pass the mock (which now implements the interface) to NewValidationMiddleware.
	// Corrected: NewValidationMiddleware now returns mcptypes.MiddlewareFunc.
	mw := middleware.NewValidationMiddleware(mockValidator, options, logger)

	// Corrected: Removed mw.SetNext call as it's deprecated and mw is now a function type.
	// mw.SetNext(mockNextHandler.Handle).

	// Corrected: Return the MiddlewareFunc directly.
	return mw, mockValidator, mockNextHandler
}
