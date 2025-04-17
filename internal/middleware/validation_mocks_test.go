// Package middleware_test tests the middleware components.
package middleware_test

// file: internal/middleware/validation_mocks_test.go

import (
	"context"
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/middleware"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

// MockSchemaValidator is a mock implementation of SchemaValidatorInterface.
type MockSchemaValidator struct {
	mock.Mock
	initialized      bool
	initErr          error
	loadDuration     time.Duration
	compileDuration  time.Duration
	shutdownErr      error
	schemas          map[string]bool
	validationErrors map[string]error // Map schemaKey -> error to return.
}

// Ensure mock implements the interface (using the imported package).
var _ middleware.SchemaValidatorInterface = (*MockSchemaValidator)(nil)

func NewMockSchemaValidator() *MockSchemaValidator {
	return &MockSchemaValidator{
		initialized:      false, // Start uninitialized.
		schemas:          make(map[string]bool),
		validationErrors: make(map[string]error),
	}
}

func (m *MockSchemaValidator) Initialize(ctx context.Context) error {
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
		// Add schemas used in tests (relevant ones moved to specific test files if needed)
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

func (m *MockSchemaValidator) IsInitialized() bool {
	return m.initialized
}

func (m *MockSchemaValidator) Validate(ctx context.Context, schemaKey string, data []byte) error {
	args := m.Called(ctx, schemaKey, data)
	if err, ok := m.validationErrors[schemaKey]; ok {
		return err // Return predefined error for this key.
	}
	return args.Error(0) // Return error configured via .Return() if any.
}

func (m *MockSchemaValidator) HasSchema(name string) bool {
	m.Called(name)
	_, exists := m.schemas[name]
	return exists
}

func (m *MockSchemaValidator) GetLoadDuration() time.Duration {
	m.Called()
	return m.loadDuration
}

func (m *MockSchemaValidator) GetCompileDuration() time.Duration {
	m.Called()
	return m.compileDuration
}

func (m *MockSchemaValidator) Shutdown() error {
	m.Called()
	return m.shutdownErr
}

// Helper to add schemas to the mock.
func (m *MockSchemaValidator) AddSchema(name string) {
	m.schemas[name] = true
}

// Helper to set a specific validation error for a schema key.
func (m *MockSchemaValidator) SetValidationError(schemaKey string, err error) {
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

// --- Test Setup ---

func setupTestMiddleware(t *testing.T, options middleware.ValidationOptions) (*middleware.ValidationMiddleware, *MockSchemaValidator, *MockMessageHandler) {
	t.Helper()
	logger := logging.GetNoopLogger() // Use NoopLogger for tests unless logging is tested.
	mockValidator := NewMockSchemaValidator()
	mockNextHandler := new(MockMessageHandler)

	// Basic mock setup for initialization and schema presence checks.
	mockValidator.On("Initialize", mock.Anything).Return(nil).Maybe() // Default successful init.
	mockValidator.On("IsInitialized").Return(true).Maybe()            // Assume initialized unless specified otherwise.
	// IMPORTANT: HasSchema needs to reflect schemas added via AddSchema or during Initialize.
	// Use a function for more dynamic checking based on the mock's internal state.
	mockValidator.On("HasSchema", mock.AnythingOfType("string")).Maybe().Return(func(name string) bool {
		_, exists := mockValidator.schemas[name]
		return exists
	})

	// Initialize the validator state within the test setup.
	err := mockValidator.Initialize(context.Background())
	require.NoError(t, err, "Mock validator initialization failed")
	// Ensure IsInitialized reflects the successful initialization.
	mockValidator.initialized = true // Explicitly set state after successful Initialize call.

	mw := middleware.NewValidationMiddleware(mockValidator, options, logger)
	mw.SetNext(mockNextHandler.Handle)

	return mw, mockValidator, mockNextHandler
}
