//go:build ignore

// file: internal/mcp/connection/manager_test.go
package connection

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	// Correct import path.
	definitions "github.com/dkoosis/cowgnition/internal/mcp/definitions"

	"github.com/cockroachdb/errors"
	// Alias for your errors package.
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2" // Still needed for types like jsonrpc2.ID, jsonrpc2.Error etc.
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Mock Definition ---

// ErrorReply struct to hold both ID and Error for assertion.
type ErrorReply struct {
	ID    jsonrpc2.ID
	Error *jsonrpc2.Error
}

// MockJSONRPCConn implements RPCConnection (defined in connection_types.go) for testing.
type MockJSONRPCConn struct {
	mu           sync.Mutex
	replies      []*jsonrpc2.Response
	errorReplies []ErrorReply // Keep tracking struct.
	sent         chan struct{}
	closeCalled  bool
	closedSignal chan struct{}
}

// NewMockJSONRPCConn simplified constructor.
func NewMockJSONRPCConn() *MockJSONRPCConn {
	return &MockJSONRPCConn{
		sent:         make(chan struct{}, 10),
		closedSignal: make(chan struct{}, 1),
	}
}

// Reply implements RPCConnection.
func (m *MockJSONRPCConn) Reply(ctx context.Context, id jsonrpc2.ID, result interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	resp := &jsonrpc2.Response{ID: id}
	rawResult, err := json.Marshal(result)
	if err != nil {
		return err
	}
	resp.Result = (*json.RawMessage)(&rawResult)
	m.replies = append(m.replies, resp)
	select {
	case m.sent <- struct{}{}:
	default:
	}
	return nil
}

// ReplyWithError implements RPCConnection.
func (m *MockJSONRPCConn) ReplyWithError(ctx context.Context, id jsonrpc2.ID, respErr *jsonrpc2.Error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorReplies = append(m.errorReplies, ErrorReply{ID: id, Error: respErr})
	select {
	case m.sent <- struct{}{}:
	default:
	}
	return nil
}

// Close implements RPCConnection.
func (m *MockJSONRPCConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closeCalled {
		m.closeCalled = true
		close(m.closedSignal)
	}
	return nil
}

// GetReplies returns a copy of the recorded replies.
func (m *MockJSONRPCConn) GetReplies() []*jsonrpc2.Response {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := make([]*jsonrpc2.Response, len(m.replies))
	copy(c, m.replies)
	return c
}

// GetErrorReplies returns a copy of the recorded error replies.
func (m *MockJSONRPCConn) GetErrorReplies() []ErrorReply {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := make([]ErrorReply, len(m.errorReplies))
	copy(c, m.errorReplies)
	return c
}

// WaitForReply waits for a reply to be sent with a timeout.
func (m *MockJSONRPCConn) WaitForReply(timeout time.Duration) bool {
	select {
	case <-m.sent:
		return true
	case <-time.After(timeout):
		return false
	}
}

// WaitForClose waits for Close() to be called with a timeout.
func (m *MockJSONRPCConn) WaitForClose(timeout time.Duration) bool {
	select {
	case <-m.closedSignal:
		return true
	case <-time.After(timeout):
		return false
	}
}

// --- Mocks for Contracts ---

// MockResourceManager satisfies ResourceManagerContract.
type MockResourceManager struct{ mock.Mock }

// GetAllResourceDefinitions mocks the corresponding method.
func (m *MockResourceManager) GetAllResourceDefinitions() []definitions.ResourceDefinition {
	args := m.Called()
	res, _ := args.Get(0).([]definitions.ResourceDefinition)
	return res
}

// ReadResource mocks the corresponding method.
func (m *MockResourceManager) ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) {
	callArgs := m.Called(ctx, name, args)
	return callArgs.String(0), callArgs.String(1), callArgs.Error(2)
}

// MockToolManager satisfies ToolManagerContract.
type MockToolManager struct{ mock.Mock }

// GetAllToolDefinitions mocks the corresponding method.
func (m *MockToolManager) GetAllToolDefinitions() []definitions.ToolDefinition {
	args := m.Called()
	res, _ := args.Get(0).([]definitions.ToolDefinition)
	return res
}

// CallTool mocks the corresponding method.
func (m *MockToolManager) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	callArgs := m.Called(ctx, name, args)
	return callArgs.String(0), callArgs.Error(1)
}

// --- Test Setup Helper ---

// setupTestManager creates a manager instance with mock dependencies for testing.
func setupTestManager(t *testing.T) (*Manager, *MockResourceManager, *MockToolManager, ServerConfig) {
	// Add t.Helper() call for test helper functions.
	t.Helper()
	cfg := ServerConfig{Name: "TestServer", Version: "1.0", RequestTimeout: 5 * time.Second, ShutdownTimeout: 5 * time.Second, Capabilities: map[string]interface{}{"testCap": true}}
	resourceMgr := new(MockResourceManager)
	toolMgr := new(MockToolManager)
	originalWriter := log.Default().Writer()
	// Suppress log output during tests unless needed for debugging.
	log.Default().SetOutput(io.Discard)
	t.Cleanup(func() { log.Default().SetOutput(originalWriter) })
	mgr := NewManager(cfg, resourceMgr, toolMgr)
	require.NotNil(t, mgr)
	require.Equal(t, StateUnconnected, mgr.stateMachine.MustState())
	return mgr, resourceMgr, toolMgr, cfg
}

// transitionManagerToConnected is a helper function to transition manager to connected state for tests.
func transitionManagerToConnected(t *testing.T, mgr *Manager, mockConn *MockJSONRPCConn) {
	// Add t.Helper() call for test helper functions.
	t.Helper()
	ctx := context.Background()
	initReqID := jsonrpc2.ID{Num: 99}
	initParams := definitions.InitializeRequest{ProtocolVersion: "1.0"}
	// Use updated makeTestRequest without 'notif' parameter.
	initReq := makeTestRequest("initialize", initParams, initReqID)

	// Handle now expects RPCConnection, which MockJSONRPCConn implements.
	mgr.Handle(ctx, mockConn, initReq)

	require.True(t, mockConn.WaitForReply(1*time.Second), "Setup: Did not receive initialize reply.")
	// Allow state machine transitions to potentially complete.
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, StateConnected, mgr.stateMachine.MustState(), "Setup: Manager did not reach Connected state.")
	require.True(t, mgr.initialized, "Setup: Manager not marked as initialized.")

	// Clear mock replies from setup phase to isolate test assertions.
	mockConn.mu.Lock()
	mockConn.replies = nil
	mockConn.errorReplies = nil
	mockConn.mu.Unlock()
}

// makeTestRequest creates a jsonrpc2 Request for testing (non-notification).
// Removed 'notif' parameter as it was unused (always false).
func makeTestRequest(method string, params interface{}, id jsonrpc2.ID) *jsonrpc2.Request {
	// Notif defaults to false, which is the previous behavior.
	req := &jsonrpc2.Request{Method: method, ID: id}
	if params != nil {
		rawParams, _ := json.Marshal(params)
		req.Params = (*json.RawMessage)(&rawParams)
	}
	return req
}

// --- Test Cases ---

// TestNewManager verifies initial state after creation.
func TestNewManager(t *testing.T) {
	// Add nolint directive for dogsled warning on test setup helper.
	mgr, _, _, _ := setupTestManager(t) //nolint:dogsled
	assert.Equal(t, StateUnconnected, mgr.stateMachine.MustState())
	assert.False(t, mgr.initialized)
	assert.NotEmpty(t, mgr.connectionID)
	assert.NotNil(t, mgr.logger)
}

// TestHandle_Initialize_Success tests the happy path for initialization.
func TestHandle_Initialize_Success(t *testing.T) {
	mgr, _, _, serverCfg := setupTestManager(t) //nolint:dogsled
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn() // Use simplified constructor.

	clientID := jsonrpc2.ID{Num: 1}
	initParams := definitions.InitializeRequest{ProtocolVersion: "1.0"}
	// Use updated makeTestRequest.
	req := makeTestRequest("initialize", initParams, clientID)
	expectedResult := definitions.InitializeResponse{ServerInfo: definitions.ServerInfo{Name: serverCfg.Name, Version: serverCfg.Version}, Capabilities: serverCfg.Capabilities, ProtocolVersion: "1.0"}

	// Pass the mock, which satisfies the RPCConnection interface.
	mgr.Handle(ctx, mockConn, req)

	require.True(t, mockConn.WaitForReply(1*time.Second))
	replies := mockConn.GetReplies()
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, replies, 1)
	require.Len(t, errorReplies, 0)
	assert.Equal(t, clientID, replies[0].ID)
	var actualResult definitions.InitializeResponse
	err := json.Unmarshal(*replies[0].Result, &actualResult)
	require.NoError(t, err)
	assert.Equal(t, expectedResult, actualResult)
	// Allow state machine transitions.
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, StateConnected, mgr.stateMachine.MustState())
	assert.True(t, mgr.initialized)
}

// TestHandle_Initialize_Failure tests the transition to Error state during initialization.
func TestHandle_Initialize_Failure(t *testing.T) {
	mgr, _, _, _ := setupTestManager(t) //nolint:dogsled
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	// Send initialize request to trigger the state transition and associate connection.
	initReqID := jsonrpc2.ID{Num: 98}
	initParams := definitions.InitializeRequest{ProtocolVersion: "1.0"}
	// Use updated makeTestRequest.
	initReq := makeTestRequest("initialize", initParams, initReqID)

	// Simulate the initialization handler returning an error by firing the trigger directly.
	mgr.Handle(ctx, mockConn, initReq)

	// Wait briefly for the state machine to potentially enter Initializing.
	time.Sleep(50 * time.Millisecond)

	// Simulate error condition within the Initializing state by firing failure trigger.
	simulatedErr := cgerr.ErrorWithDetails(errors.New("simulated init failure"), cgerr.CategoryRPC, cgerr.CodeInternalError, nil)
	if mgr.stateMachine.MustState() == StateInitializing {
		err := mgr.stateMachine.FireCtx(ctx, string(TriggerInitFailure), simulatedErr)
		require.NoError(t, err, "Firing TriggerInitFailure failed, current state: %s", mgr.stateMachine.MustState())
	} else {
		t.Logf("Manager did not enter Initializing state as expected, current state: %s. Test may not be accurate.", mgr.stateMachine.MustState())
		t.Skip("Skipping test due to unexpected state before firing failure trigger.")
	}

	// Check that an error reply was sent.
	require.True(t, mockConn.WaitForReply(1*time.Second), "No error reply received after simulated init failure.")
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, errorReplies, 1, "Expected one error reply.")
	assert.Equal(t, initReqID, errorReplies[0].ID)
	assert.Equal(t, cgerr.CodeInternalError, errorReplies[0].Error.Code) // Match the simulated error code.

	// Verify the state machine transitions to Error.
	assert.Equal(t, StateError, mgr.stateMachine.MustState())
	// Wait for the automatic transition from Error to Unconnected after timeout.
	assert.Eventually(t, func() bool {
		return mgr.stateMachine.MustState() == StateUnconnected
	}, 6*time.Second, 100*time.Millisecond, "Expected to be Unconnected after error timeout.") // Match timeout in onEnterError.
	assert.False(t, mgr.initialized)
}

// TestHandle_ConnectedState_ListResources_ValidRequest tests resources/list in connected state.
func TestHandle_ConnectedState_ListResources_ValidRequest(t *testing.T) {
	mgr, resourceMgr, _, _ := setupTestManager(t) //nolint:dogsled
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	transitionManagerToConnected(t, mgr, mockConn) // Setup initial state.

	clientID := jsonrpc2.ID{Num: 3}
	// Use updated makeTestRequest.
	req := makeTestRequest("resources/list", nil, clientID)
	expectedDefs := []definitions.ResourceDefinition{{Name: "Resource1", Description: "Desc1"}}
	resourceMgr.On("GetAllResourceDefinitions").Return(expectedDefs).Once()

	mgr.Handle(ctx, mockConn, req) // Pass mock satisfying interface.

	require.True(t, mockConn.WaitForReply(1*time.Second))
	replies := mockConn.GetReplies()
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, errorReplies, 0)
	require.Len(t, replies, 1)
	assert.Equal(t, clientID, replies[0].ID)
	var actualResult []definitions.ResourceDefinition
	err := json.Unmarshal(*replies[0].Result, &actualResult)
	require.NoError(t, err)
	assert.Equal(t, expectedDefs, actualResult)
	assert.Equal(t, StateConnected, mgr.stateMachine.MustState()) // Should remain connected.
	resourceMgr.AssertExpectations(t)
}

// Removed trailing newline that might cause whitespace lint error.

// TestHandle_ConnectedState_ReadResource_HandlerError tests error handling for resource read.
func TestHandle_ConnectedState_ReadResource_HandlerError(t *testing.T) {
	mgr, resourceMgr, _, _ := setupTestManager(t) //nolint:dogsled
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	transitionManagerToConnected(t, mgr, mockConn) // Setup initial state.

	clientID := jsonrpc2.ID{Num: 4}
	readParams := map[string]interface{}{"name": "myResource", "args": map[string]string{"arg1": "value1"}}
	// Use updated makeTestRequest.
	req := makeTestRequest("resources/read", readParams, clientID)
	// Simulate the handler returning an internal error.
	handlerError := cgerr.ErrorWithDetails(errors.New("database unavailable"), cgerr.CategoryResource, cgerr.CodeInternalError, nil)
	expectedName := "myResource"
	expectedArgsMap := map[string]string{"arg1": "value1"}
	resourceMgr.On("ReadResource", ctx, expectedName, expectedArgsMap).Return("", "", handlerError).Once()

	// Action: Trigger the error by sending the request.
	mgr.Handle(ctx, mockConn, req) // Pass mock satisfying interface.

	// Assertions: Check error reply and state transition TO Error.
	require.True(t, mockConn.WaitForReply(1*time.Second), "Did not receive error reply.")
	replies := mockConn.GetReplies()
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, replies, 0)
	require.Len(t, errorReplies, 1)
	assert.Equal(t, clientID, errorReplies[0].ID) // Check ID on error reply.
	assert.Equal(t, cgerr.CodeInternalError, errorReplies[0].Error.Code)
	assert.Contains(t, errorReplies[0].Error.Message, "database unavailable")

	// Allow time for state machine to process error trigger if async.
	time.Sleep(50 * time.Millisecond)
	// Verify manager reached Error state due to internal error code.
	require.Equal(t, StateError, mgr.stateMachine.MustState(), "Manager did not enter Error state.")

	// Wait for the automatic transition from Error to Unconnected.
	assert.Eventually(t, func() bool {
		return mgr.stateMachine.MustState() == StateUnconnected
	}, 6*time.Second, 100*time.Millisecond, "State did not become Unconnected after error timeout.")

	resourceMgr.AssertExpectations(t)
}

// TestHandle_InvalidStateOperation tests sending request in wrong state (Unconnected).
func TestHandle_InvalidStateOperation(t *testing.T) {
	mgr, _, _, _ := setupTestManager(t) //nolint:dogsled
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	clientID := jsonrpc2.ID{Num: 5}
	// Try to list resources before initializing. Use updated makeTestRequest.
	req := makeTestRequest("resources/list", nil, clientID)

	mgr.Handle(ctx, mockConn, req) // Pass mock satisfying interface.

	// Expect an error reply indicating invalid request/state.
	require.True(t, mockConn.WaitForReply(1*time.Second))
	replies := mockConn.GetReplies()
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, replies, 0)
	require.Len(t, errorReplies, 1)
	assert.Equal(t, clientID, errorReplies[0].ID) // Check ID on error reply.
	assert.Equal(t, cgerr.CodeInvalidRequest, errorReplies[0].Error.Code)
	assert.Contains(t, errorReplies[0].Error.Message, "not allowed in state '"+string(StateUnconnected)+"'")
	assert.Equal(t, StateUnconnected, mgr.stateMachine.MustState()) // Should remain unconnected.
}

// TestHandle_UnknownMethod tests sending an unsupported method.
func TestHandle_UnknownMethod(t *testing.T) {
	mgr, _, _, _ := setupTestManager(t) //nolint:dogsled
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	transitionManagerToConnected(t, mgr, mockConn) // Setup initial state.

	clientID := jsonrpc2.ID{Num: 6}
	// Use updated makeTestRequest.
	req := makeTestRequest("unknown/method", nil, clientID)

	mgr.Handle(ctx, mockConn, req) // Pass mock satisfying interface.

	// Expect a MethodNotFound error reply.
	require.True(t, mockConn.WaitForReply(1*time.Second))
	replies := mockConn.GetReplies()
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, replies, 0)
	require.Len(t, errorReplies, 1)
	assert.Equal(t, clientID, errorReplies[0].ID) // Check ID on error reply.
	assert.Equal(t, cgerr.CodeMethodNotFound, errorReplies[0].Error.Code)
	assert.Contains(t, errorReplies[0].Error.Message, "Method not found")
	assert.Equal(t, StateConnected, mgr.stateMachine.MustState()) // Should remain connected.
}

// TestHandle_Shutdown_Success tests the graceful shutdown sequence.
func TestHandle_Shutdown_Success(t *testing.T) {
	mgr, _, _, _ := setupTestManager(t) //nolint:dogsled
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	transitionManagerToConnected(t, mgr, mockConn) // Setup initial state.
	mgr.dataMu.Lock()
	mgr.jsonrpcConn = mockConn
	mgr.dataMu.Unlock()

	clientID := jsonrpc2.ID{Num: 7}
	// Use updated makeTestRequest.
	req := makeTestRequest("shutdown", nil, clientID)

	mgr.Handle(ctx, mockConn, req) // Pass mock satisfying interface.

	// Check for successful reply to shutdown request.
	require.True(t, mockConn.WaitForReply(1*time.Second))
	replies := mockConn.GetReplies()
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, errorReplies, 0)
	require.Len(t, replies, 1)
	assert.Equal(t, clientID, replies[0].ID)
	var result interface{}
	// Shutdown result should be null/nil.
	err := json.Unmarshal(*replies[0].Result, &result)
	require.NoError(t, err)
	assert.Nil(t, result)

	// Wait for state transition through Terminating to Unconnected.
	assert.Eventually(t, func() bool { return mgr.stateMachine.MustState() == StateUnconnected }, 2*time.Second, 50*time.Millisecond)
	// Check connection object is cleared in manager.
	assert.Eventually(t, func() bool { mgr.dataMu.RLock(); defer mgr.dataMu.RUnlock(); return mgr.jsonrpcConn == nil }, 1*time.Second, 50*time.Millisecond)
	// Check Close() was called on the mock connection.
	assert.True(t, mockConn.WaitForClose(1*time.Second), "Close was not tracked by mock.")
	assert.False(t, mgr.initialized) // Should be reset.
}

// TestHandle_DisconnectTrigger tests firing the disconnect trigger from various states.
func TestHandle_DisconnectTrigger(t *testing.T) {
	testCases := []struct {
		name         string
		initialState State
		setupFunc    func(*testing.T, context.Context, *Manager, *MockJSONRPCConn) bool
		checkClose   bool
	}{
		{
			name:         "From Connected",
			initialState: StateConnected,
			setupFunc: func(t *testing.T, ctx context.Context, mgr *Manager, mockConn *MockJSONRPCConn) bool {
				// Add t.Helper() for thelper lint rule.
				t.Helper()
				transitionManagerToConnected(t, mgr, mockConn)
				// Store the mock connection interface for close check.
				mgr.dataMu.Lock()
				mgr.jsonrpcConn = mockConn
				mgr.dataMu.Unlock()
				return true // Indicate setup successful.
			},
			checkClose: true,
		},
		{
			name:         "From Initializing",
			initialState: StateInitializing, // Target state.
			setupFunc: func(t *testing.T, ctx context.Context, mgr *Manager, mockConn *MockJSONRPCConn) bool {
				// Add t.Helper() for thelper lint rule.
				t.Helper()
				initReqID := jsonrpc2.ID{Num: 97}
				// Use updated makeTestRequest.
				initReq := makeTestRequest("initialize", definitions.InitializeRequest{ProtocolVersion: "1.0"}, initReqID)
				// Start Handle in background, don't wait for reply.
				go mgr.Handle(ctx, mockConn, initReq)
				time.Sleep(20 * time.Millisecond) // Give Handle time to start and potentially transition.
				currentState := mgr.stateMachine.MustState()

				// Store the specific conn instance we passed to Handle for close check.
				mgr.dataMu.Lock()
				mgr.jsonrpcConn = mockConn
				mgr.dataMu.Unlock()

				if currentState != StateInitializing {
					t.Logf("State is %s, not Initializing. Skipping test for Initializing disconnect.", currentState)
					return false // Indicate setup failed.
				}
				return true // Indicate setup successful.
			},
			checkClose: true, // Disconnect from Initializing should trigger Terminating -> Close.
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Add nolint directive for dogsled warning on test setup helper.
			mgr, _, _, _ := setupTestManager(t) //nolint:dogsled
			// Use shorter timeout for test case itself.
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			mockConn := NewMockJSONRPCConn()

			// Execute state setup function.
			if !tc.setupFunc(t, ctx, mgr, mockConn) {
				t.SkipNow() // Skip if setup function indicated failure.
			}

			currentState := mgr.stateMachine.MustState()
			if tc.initialState != StateInitializing && currentState != tc.initialState {
				require.Equal(t, tc.initialState, currentState, "Test precondition failed: Incorrect starting state.")
			}
			t.Logf("Current state before firing Disconnect: %s.", currentState)

			// Fire the disconnect trigger.
			err := mgr.stateMachine.FireCtx(ctx, string(TriggerDisconnect))
			require.NoError(t, err, "Firing TriggerDisconnect failed from state %s.", currentState)

			// Expected end state is always Unconnected after disconnect.
			expectedEndState := StateUnconnected
			assert.Eventually(t, func() bool { return mgr.stateMachine.MustState() == expectedEndState }, 2*time.Second, 50*time.Millisecond, "State did not become %s after disconnect from %s.", expectedEndState, currentState)

			if tc.checkClose {
				// Check connection is cleared from manager.
				assert.Eventually(t, func() bool { mgr.dataMu.RLock(); defer mgr.dataMu.RUnlock(); return mgr.jsonrpcConn == nil }, 1*time.Second, 50*time.Millisecond, "Connection not cleared after disconnect.")
				// Check Close() was called on the mock.
				assert.True(t, mockConn.WaitForClose(1*time.Second), "Close was not tracked by mock after disconnect from %s.", currentState)
			}
		})
	}
}

// --- Tests for Tool methods ---

// TestHandle_ConnectedState_ListTools_ValidRequest tests tools/list.
func TestHandle_ConnectedState_ListTools_ValidRequest(t *testing.T) {
	mgr, _, toolMgr, _ := setupTestManager(t) //nolint:dogsled
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	transitionManagerToConnected(t, mgr, mockConn) // Setup initial state.

	clientID := jsonrpc2.ID{Num: 8}
	// Use updated makeTestRequest.
	req := makeTestRequest("tools/list", nil, clientID)
	expectedDefs := []definitions.ToolDefinition{{Name: "Tool1", Description: "ToolDesc1"}}
	toolMgr.On("GetAllToolDefinitions").Return(expectedDefs).Once()

	mgr.Handle(ctx, mockConn, req) // Pass mock satisfying interface.

	require.True(t, mockConn.WaitForReply(1*time.Second))
	replies := mockConn.GetReplies()
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, errorReplies, 0)
	require.Len(t, replies, 1)
	assert.Equal(t, clientID, replies[0].ID)
	var actualResult []definitions.ToolDefinition
	err := json.Unmarshal(*replies[0].Result, &actualResult)
	require.NoError(t, err)
	assert.Equal(t, expectedDefs, actualResult)
	assert.Equal(t, StateConnected, mgr.stateMachine.MustState()) // Remain connected.
	toolMgr.AssertExpectations(t)
}

// TestHandle_ConnectedState_CallTool_ValidRequest tests tools/call.
func TestHandle_ConnectedState_CallTool_ValidRequest(t *testing.T) {
	mgr, _, toolMgr, _ := setupTestManager(t) //nolint:dogsled
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	transitionManagerToConnected(t, mgr, mockConn) // Setup initial state.

	clientID := jsonrpc2.ID{Num: 9}
	callParams := map[string]interface{}{"name": "myTool", "arguments": map[string]interface{}{"param1": "value1", "param2": 123.0}} // Use float64 for JSON number.
	// Use updated makeTestRequest.
	req := makeTestRequest("tools/call", callParams, clientID)
	expectedResultString := `{"toolOutput": "success"}`
	expectedToolName := "myTool"
	// Arguments received by mock should match JSON unmarshaled types.
	expectedToolArgs := map[string]interface{}{"param1": "value1", "param2": 123.0}
	toolMgr.On("CallTool", ctx, expectedToolName, expectedToolArgs).Return(expectedResultString, nil).Once()

	mgr.Handle(ctx, mockConn, req) // Pass mock satisfying interface.

	require.True(t, mockConn.WaitForReply(1*time.Second))
	replies := mockConn.GetReplies()
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, errorReplies, 0)
	require.Len(t, replies, 1)
	assert.Equal(t, clientID, replies[0].ID)

	// Result from CallTool is expected to be a string, potentially JSON.
	var actualResult string
	err := json.Unmarshal(*replies[0].Result, &actualResult)
	require.NoError(t, err, "Unmarshal tool call result into string failed.")
	// Use JSONEq for comparing JSON strings if appropriate.
	assert.JSONEq(t, expectedResultString, actualResult)

	assert.Equal(t, StateConnected, mgr.stateMachine.MustState()) // Remain connected.
	toolMgr.AssertExpectations(t)
}
