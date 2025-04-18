// Package middleware_test tests the middleware components.
package middleware_test

// file: internal/middleware/validation_mocks_test.go.

import (
	"context"
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/middleware"

	// Corrected: Import schema package for the interface.
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---.

// Corrected: Renamed MockSchemaValidator to MockValidator.
// MockValidator is a mock implementation of ValidatorInterface.
type MockValidator struct {
	mock.Mock
	initialized bool
	initErr     error
	// Corrected: Removed unused fields.
	// loadDuration     time.Duration.
	// compileDuration  time.Duration.
	// shutdownErr      error.
	schemas          map[string]bool
	validationErrors map[string]error // Map schemaKey -> error to return.
}

// Ensure mock implements the interface (using the imported schema package).
// Corrected: Use schema.ValidatorInterface and *MockValidator.
var _ schema.ValidatorInterface = (*MockValidator)(nil)

// Corrected: Renamed NewMockSchemaValidator to NewMockValidator.
func NewMockValidator() *MockValidator {
	// Corrected: Return *MockValidator.
	return &MockValidator{
		initialized:      false, // Start uninitialized.
		schemas:          make(map[string]bool),
		validationErrors: make(map[string]error),
	}
}

// Corrected: Method receiver changed to *MockValidator.
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
		m.schemas["request"] = true
		m.schemas["notification"] = true
		m.schemas["success_response"] = true
		m.schemas["error_response"] = true
		// Add schemas used in tests (relevant ones moved to specific test files if needed).
		m.schemas["initialize"] = true
		m.schemas["ping"] = true
		m.schemas["ping_notification"] = true
		m.schemas["ping_response"] = true
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
		m.schemas["tools/list"] = true
		m.schemas["tools/list_response"] = true
		m.schemas["CallToolResult"] = true
		m.schemas["notifications/progress"] = true
	}
	return m.initErr
}

// Corrected: Method receiver changed to *MockValidator.
func (m *MockValidator) IsInitialized() bool {
	args := m.Called()
	// Allow explicit return value or fallback to internal state.
	if len(args) > 0 {
		return args.Bool(0)
	}
	return m.initialized
}

// Corrected: Method receiver changed to *MockValidator.
func (m *MockValidator) Validate(ctx context.Context, schemaKey string, data []byte) error {
	args := m.Called(ctx, schemaKey, data)
	if err, ok := m.validationErrors[schemaKey]; ok {
		return err // Return predefined error for this key.
	}
	return args.Error(0) // Return error configured via .Return() if any.
}

// Corrected: Method receiver changed to *MockValidator.
func (m *MockValidator) HasSchema(name string) bool {
	args := m.Called(name)
	// Allow dynamic checking based on mock's internal state or specific returns.
	if len(args) > 0 {
		return args.Bool(0)
	}
	// Fallback to internal map if no specific return was configured for HasSchema.
	_, exists := m.schemas[name]
	return exists
}

// Corrected: Method receiver changed to *MockValidator.
func (m *MockValidator) GetLoadDuration() time.Duration {
	args := m.Called()
	// Return a mock duration, can be configured via .Return().
	if len(args) > 0 {
		// Safely attempt type assertion.
		if duration, ok := args.Get(0).(time.Duration); ok {
			return duration
		}
	}
	return 1 * time.Millisecond // Default mock duration.
}

// Corrected: Method receiver changed to *MockValidator.
func (m *MockValidator) GetCompileDuration() time.Duration {
	args := m.Called()
	// Return a mock duration, can be configured via .Return().
	if len(args) > 0 {
		// Safely attempt type assertion.
		if duration, ok := args.Get(0).(time.Duration); ok {
			return duration
		}
	}
	return 1 * time.Millisecond // Default mock duration.
}

// Corrected: Method receiver changed to *MockValidator.
func (m *MockValidator) GetSchemaVersion() string {
	args := m.Called()
	// Return a mock version, can be configured via .Return().
	if len(args) > 0 && args.String(0) != "" {
		return args.String(0)
	}
	return "mock-schema-v0.0.0" // Default mock version.
}

// Corrected: Method receiver changed to *MockValidator.
func (m *MockValidator) Shutdown() error {
	args := m.Called()
	m.initialized = false // Simulate shutdown state.
	m.schemas = nil
	return args.Error(0)
}

// Helper to add schemas to the mock.
// Corrected: Method receiver changed to *MockValidator.
func (m *MockValidator) AddSchema(name string) {
	if m.schemas == nil {
		m.schemas = make(map[string]bool)
	}
	m.schemas[name] = true
}

// Helper to set a specific validation error for a schema key.
// Corrected: Method receiver changed to *MockValidator.
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

// Corrected: Return *MockValidator.
func setupTestMiddleware(t *testing.T, options middleware.ValidationOptions) (*middleware.ValidationMiddleware, *MockValidator, *MockMessageHandler) {
	t.Helper()
	logger := logging.GetNoopLogger() // Use NoopLogger for tests unless logging is tested.
	// Corrected: Use NewMockValidator.
	mockValidator := NewMockValidator()
	mockNextHandler := new(MockMessageHandler)

	// Basic mock setup for initialization and schema presence checks.
	mockValidator.On("Initialize", mock.Anything).Return(nil).Maybe()                                    // Default successful init.
	mockValidator.On("IsInitialized").Return(true).Maybe()                                               // Assume initialized unless specified otherwise.
	mockValidator.On("HasSchema", mock.AnythingOfType("string")).Maybe().Return(func(name string) bool { // Use dynamic check.
		_, exists := mockValidator.schemas[name]
		return exists
	})
	mockValidator.On("GetSchemaVersion").Return("mock-schema-v0.0.0").Maybe() // Mock the new method.
	mockValidator.On("GetLoadDuration").Return(1 * time.Millisecond).Maybe()
	mockValidator.On("GetCompileDuration").Return(1 * time.Millisecond).Maybe()
	mockValidator.On("Shutdown").Return(nil).Maybe()

	// Initialize the validator state within the test setup.
	err := mockValidator.Initialize(context.Background())
	require.NoError(t, err, "Mock validator initialization failed.")
	// Ensure IsInitialized mock behavior is set correctly after Initialize is called.
	// Note: We configure IsInitialized mock above to return true by default.

	// Pass the mock (which now implements the interface) to NewValidationMiddleware.
	mw := middleware.NewValidationMiddleware(mockValidator, options, logger)
	mw.SetNext(mockNextHandler.Handle)

	return mw, mockValidator, mockNextHandler
}
