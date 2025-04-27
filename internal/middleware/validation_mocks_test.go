// file: internal/middleware/validation_mocks_test.go
package middleware_test

import (
	"context"
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/logging"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Import mcptypes package.
	"github.com/dkoosis/cowgnition/internal/middleware"

	// Use mcptypes for ValidatorInterface reference
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

// Ensure mock implements the interface (using the imported mcptypes).
var _ mcptypes.ValidatorInterface = (*MockValidator)(nil) // <<< Reference mcptypes

func NewMockValidator() *MockValidator {
	return &MockValidator{
		initialized:      false,
		schemas:          make(map[string]bool),
		validationErrors: make(map[string]error),
	}
}

func (m *MockValidator) Initialize(ctx context.Context) error {
	args := m.Called(ctx)
	configErr := args.Error(0)
	if configErr != nil {
		m.initErr = configErr
	}
	if m.initErr == nil {
		m.initialized = true
	}
	if m.initialized {
		// Add expected schemas... (same as before)
		m.schemas["base"] = true
		m.schemas["JSONRPCRequest"] = true
		m.schemas["JSONRPCResponse"] = true
		// ... other schemas ...
	}
	return m.initErr
}

func (m *MockValidator) IsInitialized() bool {
	args := m.Called()
	if len(args) > 0 {
		return args.Bool(0)
	}
	return m.initialized
}

func (m *MockValidator) Validate(ctx context.Context, schemaKey string, data []byte) error {
	args := m.Called(ctx, schemaKey, data)
	if err, ok := m.validationErrors[schemaKey]; ok {
		return err
	}
	return args.Error(0)
}

func (m *MockValidator) HasSchema(name string) bool {
	args := m.Called(name)
	if len(args) > 0 && args.Get(0) != nil {
		retVal, ok := args.Get(0).(bool)
		if ok {
			return retVal
		}
	}
	_, exists := m.schemas[name]
	return exists
}

func (m *MockValidator) GetLoadDuration() time.Duration {
	args := m.Called()
	if len(args) > 0 {
		if duration, ok := args.Get(0).(time.Duration); ok {
			return duration
		}
	}
	return 1 * time.Millisecond
}

func (m *MockValidator) GetCompileDuration() time.Duration {
	args := m.Called()
	if len(args) > 0 {
		if duration, ok := args.Get(0).(time.Duration); ok {
			return duration
		}
	}
	return 1 * time.Millisecond
}

func (m *MockValidator) GetSchemaVersion() string {
	args := m.Called()
	if len(args) > 0 && args.String(0) != "" {
		return args.String(0)
	}
	return "mock-schema-v0.0.0"
}

func (m *MockValidator) Shutdown() error {
	args := m.Called()
	m.initialized = false
	m.schemas = nil
	return args.Error(0)
}

// VerifyMappingsAgainstSchema is the mock implementation for the interface method. <<< CORRECTED: Exported Name >>>
// <<< RENAMED METHOD TO EXPORTED VERSION >>>
func (m *MockValidator) VerifyMappingsAgainstSchema() []string {
	args := m.Called()
	if ret := args.Get(0); ret != nil {
		if val, ok := ret.([]string); ok {
			return val
		}
	}
	return nil
}

// Helper methods (AddSchema, SetValidationError) remain the same.
func (m *MockValidator) AddSchema(name string) {
	if m.schemas == nil {
		m.schemas = make(map[string]bool)
	}
	m.schemas[name] = true
}
func (m *MockValidator) SetValidationError(schemaKey string, err error) {
	if m.validationErrors == nil {
		m.validationErrors = make(map[string]error)
	}
	m.validationErrors[schemaKey] = err
}

// MockMessageHandler struct remains the same.
type MockMessageHandler struct{ mock.Mock }

func (m *MockMessageHandler) Handle(ctx context.Context, message []byte) ([]byte, error) {
	args := m.Called(ctx, message)
	resBytes, _ := args.Get(0).([]byte)
	return resBytes, args.Error(1)
}

// setupTestMiddleware initializes mocks and ensures mock validator is initialized.
func setupTestMiddleware(t *testing.T, options mcptypes.ValidationOptions) (mcptypes.MiddlewareFunc, *MockValidator, *MockMessageHandler) {
	t.Helper()
	logger := logging.GetNoopLogger()
	mockValidator := NewMockValidator()
	mockNextHandler := new(MockMessageHandler)

	// Basic mock setup... (same as before, ensure VerifyMappingsAgainstSchema is included)
	mockValidator.On("Initialize", mock.Anything).Return(nil).Maybe()
	mockValidator.On("HasSchema", mock.AnythingOfType("string")).Maybe().Return(func(name string) bool {
		_, exists := mockValidator.schemas[name]
		return exists
	})
	mockValidator.On("GetSchemaVersion").Return("mock-schema-v0.0.0").Maybe()
	mockValidator.On("GetLoadDuration").Return(1 * time.Millisecond).Maybe()
	mockValidator.On("GetCompileDuration").Return(1 * time.Millisecond).Maybe()
	mockValidator.On("Shutdown").Return(nil).Maybe()
	// <<< CORRECTED: Use Exported Name in Expectation >>>
	mockValidator.On("VerifyMappingsAgainstSchema").Return(nil).Maybe()

	err := mockValidator.Initialize(context.Background())
	require.NoError(t, err, "Mock validator initialization failed in test setup.")
	mockValidator.On("IsInitialized").Return(true).Maybe()

	mw := middleware.NewValidationMiddleware(mockValidator, options, logger)
	return mw, mockValidator, mockNextHandler
}
