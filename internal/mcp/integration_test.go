// file: internal/mcp/integration_test.go
package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging" // Import for logging.
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
	"github.com/dkoosis/cowgnition/internal/mcp/router"
	"github.com/dkoosis/cowgnition/internal/mcp/state"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Import mcptypes package.
	"github.com/dkoosis/cowgnition/internal/middleware"

	// Use mcptypes for ValidatorInterface reference.
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockValidator is a mock schema validator for testing.
// --- MOCK VALIDATOR DEFINITION (Local to this test file) ---.
type MockValidator struct {
	mock.Mock
}

// Ensure MockValidator implements the mcptypes.ValidatorInterface.
var _ mcptypes.ValidatorInterface = (*MockValidator)(nil) // <<< Reference mcptypes.

func (m *MockValidator) Initialize(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockValidator) Validate(ctx context.Context, messageType string, data []byte) error {
	args := m.Called(ctx, messageType, data)
	return args.Error(0)
}

func (m *MockValidator) HasSchema(name string) bool {
	args := m.Called(name)
	return args.Bool(0)
}

func (m *MockValidator) IsInitialized() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockValidator) GetSchemaVersion() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockValidator) Shutdown() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockValidator) GetCompileDuration() time.Duration {
	args := m.Called()
	if len(args) > 0 && args.Get(0) != nil {
		if duration, ok := args.Get(0).(time.Duration); ok {
			return duration
		}
	}
	return time.Millisecond * 1 // Default mock duration.
}

func (m *MockValidator) GetLoadDuration() time.Duration {
	args := m.Called()
	if len(args) > 0 && args.Get(0) != nil {
		if duration, ok := args.Get(0).(time.Duration); ok {
			return duration
		}
	}
	return time.Millisecond * 1 // Default mock duration.
}

// --- CORRECTED: Implementation for the interface method ---
// VerifyMappingsAgainstSchema is the mock implementation for the interface method.
func (m *MockValidator) VerifyMappingsAgainstSchema() []string {
	// Tell the mock framework this method was called.
	args := m.Called()
	// Return the configured return value (or nil/empty slice if not configured).
	if ret := args.Get(0); ret != nil {
		if val, ok := ret.([]string); ok {
			return val
		}
	}
	return nil // Default return for a mock often nil or zero value.
}

// --- END CORRECTED ---

// --- END MOCK VALIDATOR DEFINITION ---

// TestServer_IntegrationFlow tests the flow of messages through the FSM, Router, and Server.
func TestServer_IntegrationFlow(t *testing.T) {
	// Setup logger
	logging.SetupDefaultLogger("debug")
	logger := logging.GetLogger("mcp_integration_test")
	logger.Info(">>> TestServer_IntegrationFlow: Logger configured for DEBUG <<<")

	cfg := config.DefaultConfig()

	// Create mock validator using the local definition
	mockValidator := new(MockValidator)

	// --- Setup mock expectations ---
	mockValidator.On("IsInitialized").Return(true).Maybe()
	mockValidator.On("Validate", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
	mockValidator.On("HasSchema", mock.AnythingOfType("string")).Return(true).Maybe() // Be flexible.
	mockValidator.On("GetCompileDuration").Return(time.Millisecond * 10).Maybe()
	mockValidator.On("GetLoadDuration").Return(time.Millisecond * 5).Maybe()
	mockValidator.On("Shutdown").Return(nil).Maybe()
	mockValidator.On("Initialize", mock.Anything).Return(nil).Maybe()
	mockValidator.On("GetSchemaVersion").Return("mock-test-v1").Maybe()
	// <<< CORRECTED: Use Exported Name >>>
	mockValidator.On("VerifyMappingsAgainstSchema").Return(nil).Maybe() // Expect the corrected method.
	// --- End mock setup ---

	// Create FSM and Router
	mcpFSM, err := state.NewMCPStateMachine(logger.WithField("subcomponent", "fsm"))
	require.NoError(t, err)
	mcpRouter := router.NewRouter(logger.WithField("subcomponent", "router"))

	// Add essential routes (ping, initialize, notifications/initialized, shutdown, exit)
	// (Route implementations are the same as your provided code, omitted here for brevity)
	err = mcpRouter.AddRoute(router.Route{
		Method: "ping",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			logger.Debug(">>> TEST: Executing MOCK ping handler <<<")
			return json.RawMessage(`{"pong": true}`), nil
		},
	})
	require.NoError(t, err)

	err = mcpRouter.AddRoute(router.Route{
		Method: "initialize",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			logger.Debug(">>> TEST: Executing MOCK initialize handler <<<")
			return json.RawMessage(`{
				"protocolVersion": "2024-11-05",
				"serverInfo": {"name": "TestServer", "version": "1.0.0"},
				"capabilities": {}
			}`), nil
		},
	})
	require.NoError(t, err)

	err = mcpRouter.AddRoute(router.Route{
		Method: "notifications/initialized",
		NotificationHandler: func(_ context.Context, _ json.RawMessage) error {
			logger.Debug(">>> TEST: Executing MOCK notifications/initialized handler <<<")
			return nil
		},
	})
	require.NoError(t, err)

	err = mcpRouter.AddRoute(router.Route{
		Method: "shutdown",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			logger.Debug(">>> TEST: Executing MOCK shutdown handler <<<")
			return json.RawMessage(`null`), nil
		},
	})
	require.NoError(t, err)

	err = mcpRouter.AddRoute(router.Route{
		Method: "exit",
		NotificationHandler: func(_ context.Context, _ json.RawMessage) error {
			logger.Debug(">>> TEST: Executing MOCK exit handler <<<")
			return nil
		},
	})
	require.NoError(t, err)

	// Create Server
	opts := ServerOptions{
		RequestTimeout: 5 * time.Second,
		Debug:          true,
	}
	// <<< CORRECTED: Pass mockValidator which now satisfies mcptypes.ValidatorInterface >>>
	server, err := NewServer(cfg, opts, mockValidator, mcpFSM, mcpRouter, time.Now(), logger)
	require.NoError(t, err)

	// Setup Middleware Chain
	// <<< CORRECTED: Use middleware package for DefaultValidationOptions >>>
	validationOpts := middleware.DefaultValidationOptions()
	validationOpts.StrictMode = true
	validationOpts.ValidateOutgoing = false // Adjust as needed for specific tests.
	validationOpts.SkipTypes = map[string]bool{
		"exit": true,
	}
	// <<< CORRECTED: Pass mockValidator >>>
	validationMiddleware := middleware.NewValidationMiddleware(
		mockValidator, // This mock now satisfies the interface.
		validationOpts,
		logger.WithField("subcomponent", "validation_mw"),
	)
	chain := middleware.NewChain(server.handleMessageWithFSM)
	chain.Use(validationMiddleware)
	loggingMiddleware := createLoggingMiddleware(logger.WithField("subcomponent", "logging_mw"))
	chain.Use(loggingMiddleware)
	finalHandler := chain.Handler()

	// --- Subtests ---
	// (Subtest logic remains the same as your provided code)
	t.Run("Ping_fails_when_not_initialized", func(t *testing.T) {
		t.Log(">>> Running subtest: Ping_fails_when_not_initialized")
		err := mcpFSM.Reset()
		require.NoError(t, err)
		require.Equal(t, state.StateUninitialized, mcpFSM.CurrentState())

		// --- Clear expectations and set specific ones ---
		mockValidator.ExpectedCalls = nil // Use direct access.
		mockValidator.Calls = nil         // Use direct access.
		mockValidator.On("IsInitialized").Return(true).Maybe()
		mockValidator.On("HasSchema", "ping").Return(true).Maybe()
		pingMsgBytes := []byte(`{"jsonrpc": "2.0", "id": 1, "method": "ping", "params": {}}`)
		mockValidator.On("Validate", mock.Anything, "ping", pingMsgBytes).Return(nil).Maybe()
		// --- End specific expectations ---

		result, err := finalHandler(context.Background(), pingMsgBytes)

		require.NoError(t, err)
		require.NotNil(t, result)
		var errResp struct {
			Error struct {
				Code    int                    `json:"code"`
				Message string                 `json:"message"`
				Data    map[string]interface{} `json:"data"`
			} `json:"error"`
			ID json.RawMessage `json:"id"`
		}
		err = json.Unmarshal(result, &errResp)
		require.NoError(t, err)

		assert.EqualValues(t, mcperrors.ErrRequestSequence, errResp.Error.Code)
		assert.Equal(t, "Invalid message sequence.", errResp.Error.Message)
		require.Contains(t, errResp.Error.Data, "fsmCode")
		assert.Equal(t, "InvalidEventError", errResp.Error.Data["fsmCode"])
		require.Contains(t, errResp.Error.Data, "detail")
		assert.Contains(t, errResp.Error.Data["detail"].(string), "event rcvd_mcp_request inappropriate in current state uninitialized")
		assert.Equal(t, json.RawMessage("1"), errResp.ID)
	})

	t.Run("Initialization_flow", func(t *testing.T) {
		t.Log(">>> Running subtest: Initialization_flow")
		err := mcpFSM.Reset()
		require.NoError(t, err)
		require.Equal(t, state.StateUninitialized, mcpFSM.CurrentState())

		// --- Clear expectations and set specific ones ---
		mockValidator.ExpectedCalls = nil // Use direct access.
		mockValidator.Calls = nil         // Use direct access.
		mockValidator.On("IsInitialized").Return(true).Maybe()
		mockValidator.On("HasSchema", "initialize").Return(true).Maybe()
		mockValidator.On("HasSchema", "notifications/initialized").Return(true).Maybe()
		mockValidator.On("HasSchema", "ping").Return(true).Maybe()

		initMsgBytes := []byte(`{
			"jsonrpc": "2.0",
			"id": 2,
			"method": "initialize",
			"params": {
				"protocolVersion": "2024-11-05",
				"clientInfo": { "name": "TestClient", "version": "1.0.0" },
				"capabilities": {}
			}
		}`)
		notifMsgBytes := []byte(`{
			"jsonrpc": "2.0",
			"method": "notifications/initialized",
			"params": {}
		}`)
		pingMsgBytes := []byte(`{"jsonrpc": "2.0", "id": 3, "method": "ping", "params": {}}`)

		mockValidator.On("Validate", mock.Anything, "initialize", initMsgBytes).Return(nil).Maybe()
		mockValidator.On("Validate", mock.Anything, "notifications/initialized", notifMsgBytes).Return(nil).Maybe()
		mockValidator.On("Validate", mock.Anything, "ping", pingMsgBytes).Return(nil).Maybe()
		// --- End specific expectations ---

		// 1. Initialize request
		t.Log(">>> Sending initialize request...")
		result, err := finalHandler(context.Background(), initMsgBytes)
		require.NoError(t, err)
		require.NotNil(t, result)
		var initResp struct {
			Result struct {
				ProtocolVersion string `json:"protocolVersion"`
			} `json:"result"`
			ID json.RawMessage `json:"id"`
		}
		err = json.Unmarshal(result, &initResp)
		require.NoError(t, err)
		assert.Equal(t, "2024-11-05", initResp.Result.ProtocolVersion)
		assert.Equal(t, json.RawMessage("2"), initResp.ID)
		assert.Equal(t, state.StateInitializing, mcpFSM.CurrentState())
		t.Log(">>> Initialize request successful.")

		// 2. Client initialized notification
		t.Log(">>> Sending notifications/initialized notification...")
		result, err = finalHandler(context.Background(), notifMsgBytes)
		require.NoError(t, err)
		require.Nil(t, result)
		assert.Equal(t, state.StateInitialized, mcpFSM.CurrentState())
		t.Log(">>> notifications/initialized notification successful.")

		// 3. Ping request
		t.Log(">>> Sending ping request...")
		result, err = finalHandler(context.Background(), pingMsgBytes)
		require.NoError(t, err)
		require.NotNil(t, result)
		t.Logf(">>> Raw ping response bytes received: %s", string(result))
		var pingResp struct {
			Result struct {
				Pong bool `json:"pong"`
			} `json:"result"`
			ID json.RawMessage `json:"id"`
		}
		err = json.Unmarshal(result, &pingResp)
		require.NoError(t, err)
		t.Logf(">>> Unmarshalled ping response struct: %+v", pingResp)
		assert.True(t, pingResp.Result.Pong, "Ping response should contain 'pong: true'")
		assert.Equal(t, json.RawMessage("3"), pingResp.ID)
		t.Log(">>> Ping request successful.")
	})

	t.Run("Shutdown_flow", func(t *testing.T) {
		t.Log(">>> Running subtest: Shutdown_flow")
		err := mcpFSM.SetState(state.StateInitialized)
		require.NoError(t, err)
		require.Equal(t, state.StateInitialized, mcpFSM.CurrentState())

		// --- Clear expectations and set specific ones ---
		mockValidator.ExpectedCalls = nil // Use direct access.
		mockValidator.Calls = nil         // Use direct access.
		mockValidator.On("IsInitialized").Return(true).Maybe()
		mockValidator.On("HasSchema", "shutdown").Return(true).Maybe()
		shutdownMsgBytes := []byte(`{"jsonrpc": "2.0", "id": 4, "method": "shutdown", "params": {}}`)
		mockValidator.On("Validate", mock.Anything, "shutdown", shutdownMsgBytes).Return(nil).Maybe()
		// --- End specific expectations ---

		// 1. Shutdown request
		result, err := finalHandler(context.Background(), shutdownMsgBytes)
		require.NoError(t, err)
		require.NotNil(t, result)
		var shutdownResp struct {
			Result json.RawMessage `json:"result"`
			ID     json.RawMessage `json:"id"`
		}
		err = json.Unmarshal(result, &shutdownResp)
		require.NoError(t, err)
		assert.Equal(t, json.RawMessage("null"), shutdownResp.Result)
		assert.Equal(t, json.RawMessage("4"), shutdownResp.ID)
		assert.Equal(t, state.StateShuttingDown, mcpFSM.CurrentState())

		// 2. Exit notification
		exitMsgBytes := []byte(`{"jsonrpc": "2.0", "method": "exit", "params": {}}`)
		result, err = finalHandler(context.Background(), exitMsgBytes)
		require.NoError(t, err)
		require.Nil(t, result)
		assert.Equal(t, state.StateShutdown, mcpFSM.CurrentState())
	})
}
