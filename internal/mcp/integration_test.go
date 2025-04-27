// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// file: internal/mcp/integration_test.go
package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/mcp/router"
	"github.com/dkoosis/cowgnition/internal/mcp/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockValidator is a mock schema validator for testing.
type MockValidator struct {
	mock.Mock
}

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
	return args.Get(0).(time.Duration)
}

// Add the missing GetLoadDuration method required by schema.ValidatorInterface.
func (m *MockValidator) GetLoadDuration() time.Duration {
	args := m.Called()
	return args.Get(0).(time.Duration)
}

// TestServer_IntegrationFlow tests the flow of messages through the FSM, Router, and Server.
func TestServer_IntegrationFlow(t *testing.T) {
	// Setup
	logger := logging.GetNoopLogger()
	cfg := config.DefaultConfig()

	// Create mock validator
	mockValidator := new(MockValidator)
	mockValidator.On("IsInitialized").Return(true)
	mockValidator.On("Validate", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mockValidator.On("GetCompileDuration").Return(time.Millisecond * 10)
	mockValidator.On("GetLoadDuration").Return(time.Millisecond * 5)
	mockValidator.On("Shutdown").Return(nil)

	// Create FSM and Router
	mcpFSM, err := state.NewMCPStateMachine(logger)
	require.NoError(t, err)

	mcpRouter := router.NewRouter(logger)

	// Add test routes to router
	err = mcpRouter.AddRoute(router.Route{
		Method: "ping",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) { // FIX: Renamed params to _
			return json.RawMessage(`{"pong": true}`), nil
		},
	})
	require.NoError(t, err)

	err = mcpRouter.AddRoute(router.Route{
		Method: "initialize",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) { // FIX: Renamed params to _
			// Simple response for testing
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
		NotificationHandler: func(_ context.Context, _ json.RawMessage) error { // FIX: Renamed params to _
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

	// Test message processing for ping (should fail if not initialized)
	t.Run("Ping fails when not initialized", func(t *testing.T) {
		pingMsg := []byte(`{"jsonrpc": "2.0", "id": 1, "method": "ping", "params": {}}`)

		// Process message
		result, err := server.handleMessageWithFSM(context.Background(), pingMsg)

		// Should fail with a sequence error
		require.Nil(t, err)

		// Parse the error response
		var errResp struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		err = json.Unmarshal(result, &errResp)
		require.NoError(t, err)

		// Check for sequence error
		assert.Equal(t, -32001, errResp.Error.Code) // ErrRequestSequence
		assert.Contains(t, errResp.Error.Message, "sequence")
	})

	// Test initialization flow
	t.Run("Initialization flow", func(t *testing.T) {
		// 1. Initialize request
		initMsg := []byte(`{
			"jsonrpc": "2.0",
			"id": 2,
			"method": "initialize",
			"params": {
				"protocolVersion": "2024-11-05",
				"clientInfo": {
					"name": "TestClient",
					"version": "1.0.0"
				},
				"capabilities": {}
			}
		}`)

		result, err := server.handleMessageWithFSM(context.Background(), initMsg)
		require.Nil(t, err)
		require.NotNil(t, result)

		// Check that we got a valid response
		var initResp struct {
			Result struct {
				ProtocolVersion string `json:"protocolVersion"`
			} `json:"result"`
		}
		err = json.Unmarshal(result, &initResp)
		require.NoError(t, err)
		assert.Equal(t, "2024-11-05", initResp.Result.ProtocolVersion)

		// 2. Client initialized notification
		notifMsg := []byte(`{
			"jsonrpc": "2.0",
			"method": "notifications/initialized",
			"params": {}
		}`)

		result, err = server.handleMessageWithFSM(context.Background(), notifMsg)
		require.Nil(t, err)
		require.Nil(t, result) // No response for notification

		// Check FSM state is now Initialized
		assert.Equal(t, state.StateInitialized, mcpFSM.CurrentState())

		// 3. Now ping should work
		pingMsg := []byte(`{"jsonrpc": "2.0", "id": 3, "method": "ping", "params": {}}`)

		result, err = server.handleMessageWithFSM(context.Background(), pingMsg)
		require.Nil(t, err)
		require.NotNil(t, result)

		var pingResp struct {
			Result struct {
				Pong bool `json:"pong"`
			} `json:"result"`
		}
		err = json.Unmarshal(result, &pingResp)
		require.NoError(t, err)
		assert.True(t, pingResp.Result.Pong)
	})

	// Test shutdown flow
	t.Run("Shutdown flow", func(t *testing.T) {
		// Add shutdown and exit routes for this test
		err = mcpRouter.AddRoute(router.Route{
			Method: "shutdown",
			Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) { // FIX: Renamed params to _
				return json.RawMessage(`null`), nil
			},
		})
		require.NoError(t, err)

		err = mcpRouter.AddRoute(router.Route{
			Method: "exit",
			NotificationHandler: func(_ context.Context, _ json.RawMessage) error { // FIX: Renamed params to _
				return nil
			},
		})
		require.NoError(t, err)

		// 1. Shutdown request
		shutdownMsg := []byte(`{"jsonrpc": "2.0", "id": 4, "method": "shutdown", "params": {}}`)

		result, err := server.handleMessageWithFSM(context.Background(), shutdownMsg)
		require.Nil(t, err)
		require.NotNil(t, result)

		// Check FSM state is now ShuttingDown
		assert.Equal(t, state.StateShuttingDown, mcpFSM.CurrentState())

		// 2. Exit notification
		exitMsg := []byte(`{"jsonrpc": "2.0", "method": "exit", "params": {}}`)

		result, err = server.handleMessageWithFSM(context.Background(), exitMsg)
		require.Nil(t, err)
		require.Nil(t, result) // No response for notification

		// Check FSM state is now Shutdown
		assert.Equal(t, state.StateShutdown, mcpFSM.CurrentState())
	})
}
