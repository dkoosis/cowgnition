// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// file: internal/mcp/integration_test.go
package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging" // Import for middleware types
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
	"github.com/dkoosis/cowgnition/internal/mcp/router"
	"github.com/dkoosis/cowgnition/internal/mcp/state"
	"github.com/dkoosis/cowgnition/internal/middleware" // Import middleware package
	"github.com/dkoosis/cowgnition/internal/schema"     // Import schema package for ValidatorInterface
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockValidator is a mock schema validator for testing.
// --- MOCK VALIDATOR DEFINITION ---.
type MockValidator struct {
	mock.Mock
}

// Ensure MockValidator implements the schema.ValidatorInterface.
var _ schema.ValidatorInterface = (*MockValidator)(nil)

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
	return time.Millisecond * 1 // Default mock duration
}

func (m *MockValidator) GetLoadDuration() time.Duration {
	args := m.Called()
	if len(args) > 0 && args.Get(0) != nil {
		if duration, ok := args.Get(0).(time.Duration); ok {
			return duration
		}
	}
	return time.Millisecond * 1 // Default mock duration
}

// --- END MOCK VALIDATOR DEFINITION ---

// TestServer_IntegrationFlow tests the flow of messages through the FSM, Router, and Server.
func TestServer_IntegrationFlow(t *testing.T) {
	// Setup
	logger := logging.GetNoopLogger()
	cfg := config.DefaultConfig()

	// Create mock validator
	mockValidator := new(MockValidator)
	// --- Specific mock setup ---
	// Default expectations (can be overridden in subtests)
	mockValidator.On("IsInitialized").Return(true).Maybe() // Use Maybe() for flexibility across tests
	mockValidator.On("Validate", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()
	// Explicitly handle HasSchema calls expected by middleware:
	mockValidator.On("HasSchema", "ping").Return(true).Maybe()
	mockValidator.On("HasSchema", "initialize").Return(true).Maybe()
	mockValidator.On("HasSchema", "notifications/initialized").Return(true).Maybe()
	mockValidator.On("HasSchema", "shutdown").Return(true).Maybe()
	mockValidator.On("HasSchema", "exit").Return(true).Maybe()
	mockValidator.On("HasSchema", "JSONRPCRequest").Return(true).Maybe()
	mockValidator.On("HasSchema", "JSONRPCNotification").Return(true).Maybe()
	mockValidator.On("HasSchema", "JSONRPCResponse").Return(true).Maybe()
	mockValidator.On("HasSchema", "JSONRPCError").Return(true).Maybe()
	mockValidator.On("HasSchema", "base").Return(true).Maybe()
	// Allow any other HasSchema call for flexibility
	mockValidator.On("HasSchema", mock.AnythingOfType("string")).Return(true).Maybe()

	mockValidator.On("GetCompileDuration").Return(time.Millisecond * 10).Maybe()
	mockValidator.On("GetLoadDuration").Return(time.Millisecond * 5).Maybe()
	mockValidator.On("Shutdown").Return(nil).Maybe()
	mockValidator.On("Initialize", mock.Anything).Return(nil).Maybe()
	mockValidator.On("GetSchemaVersion").Return("mock-test-v1").Maybe()
	// --- END Specific mock setup ---

	// Create FSM and Router
	mcpFSM, err := state.NewMCPStateMachine(logger)
	require.NoError(t, err)

	mcpRouter := router.NewRouter(logger)

	// Add essential routes to router
	err = mcpRouter.AddRoute(router.Route{
		Method: "ping",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"pong": true}`), nil
		},
	})
	require.NoError(t, err)

	err = mcpRouter.AddRoute(router.Route{
		Method: "initialize",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
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
			return nil
		},
	})
	require.NoError(t, err)

	err = mcpRouter.AddRoute(router.Route{
		Method: "shutdown",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`null`), nil
		},
	})
	require.NoError(t, err)

	err = mcpRouter.AddRoute(router.Route{
		Method: "exit",
		NotificationHandler: func(_ context.Context, _ json.RawMessage) error {
			return nil
		},
	})
	require.NoError(t, err)

	// Create Server
	opts := ServerOptions{
		RequestTimeout: 5 * time.Second,
		Debug:          true,
	}
	server, err := NewServer(cfg, opts, mockValidator, mcpFSM, mcpRouter, time.Now(), logger)
	require.NoError(t, err)

	// --- Setup Middleware Chain ---
	validationOpts := middleware.DefaultValidationOptions()
	validationOpts.StrictMode = true
	validationOpts.ValidateOutgoing = false
	validationOpts.SkipTypes = map[string]bool{
		"exit": true, // Skip schema validation for exit notification
	}

	validationMiddleware := middleware.NewValidationMiddleware(
		mockValidator,
		validationOpts,
		logger.WithField("subcomponent", "validation_mw"),
	)

	chain := middleware.NewChain(server.handleMessageWithFSM)
	chain.Use(validationMiddleware)

	finalHandler := chain.Handler()
	// --- END Setup Middleware Chain ---

	// --- Subtests (Use finalHandler) ---

	t.Run("Ping_fails_when_not_initialized", func(t *testing.T) {
		// Reset FSM
		err := mcpFSM.Reset()
		require.NoError(t, err, "Failed to reset FSM state for subtest")
		require.Equal(t, state.StateUninitialized, mcpFSM.CurrentState())

		mockValidator.ExpectedCalls = nil // Clear general expectations
		// Set specific expectations for this test:
		// Allow multiple calls to IsInitialized
		mockValidator.On("IsInitialized").Return(true).Maybe()
		mockValidator.On("HasSchema", "ping").Return(true).Once()
		pingMsgBytes := []byte(`{"jsonrpc": "2.0", "id": 1, "method": "ping", "params": {}}`)
		mockValidator.On("Validate", mock.Anything, "ping", pingMsgBytes).Return(nil).Once()

		// Process message
		result, err := finalHandler(context.Background(), pingMsgBytes)

		// Assertions: Expect -32001 error code etc.
		require.NoError(t, err, "Processing should not return a Go error, only error response bytes")
		require.NotNil(t, result, "Should receive error response bytes")
		var errResp struct {
			Error struct {
				Code    int                    `json:"code"`
				Message string                 `json:"message"`
				Data    map[string]interface{} `json:"data"`
			} `json:"error"`
			ID json.RawMessage `json:"id"`
		}
		err = json.Unmarshal(result, &errResp)
		require.NoError(t, err, "Failed to unmarshal error response")

		// --- CORRECTED ASSERTION BLOCK ---
		// 1. Check the JSON-RPC Error Code directly against the expected constant
		assert.EqualValues(t, mcperrors.ErrRequestSequence, errResp.Error.Code, "JSON-RPC error code should be ErrRequestSequence (-32001)")

		// 2. Check the specific message associated with that code
		assert.Equal(t, "Invalid message sequence.", errResp.Error.Message, "JSON-RPC error message mismatch")

		// 3. Check specific fields within the Data payload (assuming createErrorResponse adds them)
		require.Contains(t, errResp.Error.Data, "fsmCode", "Error data missing 'fsmCode' field")
		// Assert the exact FSM error code expected when an event is invalid for the state
		// This value depends on the exact error type from looplab/fsm, likely InvalidEventError
		assert.Equal(t, "InvalidEventError", errResp.Error.Data["fsmCode"], "Expected FSM error code mismatch")

		require.Contains(t, errResp.Error.Data, "detail", "Error data missing 'detail' field")
		// Check the detail field, which might contain the raw FSM error string
		assert.Contains(t, errResp.Error.Data["detail"].(string), "event rcvd_mcp_request inappropriate in current state uninitialized", "Error detail mismatch")
		// --- END CORRECTED BLOCK ---

		// Verify ID remains correct
		assert.Equal(t, json.RawMessage("1"), errResp.ID, "Error response ID should match request ID")

		mockValidator.AssertExpectations(t) // Verify mocks for this subtest
	})

	t.Run("Initialization_flow", func(t *testing.T) {
		// Reset FSM
		err := mcpFSM.Reset()
		require.NoError(t, err, "Failed to reset FSM state for subtest")
		require.Equal(t, state.StateUninitialized, mcpFSM.CurrentState())

		mockValidator.ExpectedCalls = nil // Clear general expectations
		// Set specific expectations for this test:
		// Use Maybe() for IsInitialized as it's called multiple times per message
		mockValidator.On("IsInitialized").Return(true).Maybe()
		// Expect HasSchema checks for each message type
		mockValidator.On("HasSchema", "initialize").Return(true).Once()
		mockValidator.On("HasSchema", "notifications/initialized").Return(true).Once()
		mockValidator.On("HasSchema", "ping").Return(true).Once()
		// Expect Validate calls for each message type
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

		mockValidator.On("Validate", mock.Anything, "initialize", initMsgBytes).Return(nil).Once()
		mockValidator.On("Validate", mock.Anything, "notifications/initialized", notifMsgBytes).Return(nil).Once()
		mockValidator.On("Validate", mock.Anything, "ping", pingMsgBytes).Return(nil).Once()

		// 1. Initialize request using finalHandler
		result, err := finalHandler(context.Background(), initMsgBytes)
		require.NoError(t, err, "Initialization request processing failed")
		require.NotNil(t, result, "Initialization should return response bytes")
		var initResp struct {
			Result struct {
				ProtocolVersion string `json:"protocolVersion"`
			} `json:"result"`
			ID json.RawMessage `json:"id"`
		}
		err = json.Unmarshal(result, &initResp)
		require.NoError(t, err, "Failed to unmarshal initialize response")
		assert.Equal(t, "2024-11-05", initResp.Result.ProtocolVersion, "Protocol version in response mismatch")
		assert.Equal(t, json.RawMessage("2"), initResp.ID, "Initialize response ID mismatch")
		assert.Equal(t, state.StateInitializing, mcpFSM.CurrentState(), "FSM state should be Initializing after init request")

		// 2. Client initialized notification using finalHandler
		result, err = finalHandler(context.Background(), notifMsgBytes)
		require.NoError(t, err, "Initialized notification processing failed")
		require.Nil(t, result, "Notification should not return response bytes")
		assert.Equal(t, state.StateInitialized, mcpFSM.CurrentState(), "FSM state should be Initialized after notification")

		// 3. Ping request using finalHandler
		result, err = finalHandler(context.Background(), pingMsgBytes)
		require.NoError(t, err, "Post-initialization ping processing failed")
		require.NotNil(t, result, "Post-initialization ping should return response bytes")
		var pingResp struct {
			Result struct {
				Pong bool `json:"pong"`
			} `json:"result"`
			ID json.RawMessage `json:"id"`
		}
		err = json.Unmarshal(result, &pingResp)
		require.NoError(t, err, "Failed to unmarshal post-initialization ping response")
		assert.True(t, pingResp.Result.Pong, "Ping response should contain 'pong: true'")
		assert.Equal(t, json.RawMessage("3"), pingResp.ID, "Ping response ID mismatch")

		mockValidator.AssertExpectations(t) // Verify mocks for this subtest
	})

	t.Run("Shutdown_flow", func(t *testing.T) {
		// Reset/Set FSM State
		err := mcpFSM.SetState(state.StateInitialized) // Use SetState for direct control
		require.NoError(t, err)
		require.Equal(t, state.StateInitialized, mcpFSM.CurrentState())

		mockValidator.ExpectedCalls = nil // Clear general expectations
		// Set specific expectations for this test:
		mockValidator.On("IsInitialized").Return(true).Maybe() // Use Maybe()
		mockValidator.On("HasSchema", "shutdown").Return(true).Once()
		// No HasSchema("exit") expected because "exit" is in SkipTypes map
		shutdownMsgBytes := []byte(`{"jsonrpc": "2.0", "id": 4, "method": "shutdown", "params": {}}`)
		mockValidator.On("Validate", mock.Anything, "shutdown", shutdownMsgBytes).Return(nil).Once()
		// No Validate call expected for "exit"

		// 1. Shutdown request using finalHandler
		result, err := finalHandler(context.Background(), shutdownMsgBytes)
		require.NoError(t, err)
		require.NotNil(t, result)
		var shutdownResp struct {
			Result json.RawMessage `json:"result"`
			ID     json.RawMessage `json:"id"`
		}
		err = json.Unmarshal(result, &shutdownResp)
		require.NoError(t, err, "Failed to unmarshal shutdown response")
		assert.Equal(t, json.RawMessage("null"), shutdownResp.Result)
		assert.Equal(t, json.RawMessage("4"), shutdownResp.ID)
		assert.Equal(t, state.StateShuttingDown, mcpFSM.CurrentState())

		// 2. Exit notification using finalHandler
		exitMsgBytes := []byte(`{"jsonrpc": "2.0", "method": "exit", "params": {}}`)
		result, err = finalHandler(context.Background(), exitMsgBytes)
		require.NoError(t, err)
		require.Nil(t, result)
		assert.Equal(t, state.StateShutdown, mcpFSM.CurrentState())

		mockValidator.AssertExpectations(t) // Verify mocks for this subtest
	})
}
