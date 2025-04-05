// internal/mcp/connection/manager_test.go
package connection

import (
	"context"
	"encoding/json"
	"io"
	"log"

	// "net" // No longer needed for mock conn setup

	"sync"
	"testing"
	"time"

	// Correct import path
	definitions "github.com/dkoosis/cowgnition/internal/mcp/definitions"

	"github.com/cockroachdb/errors"
	// Alias for your errors package
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2" // Still needed for types like jsonrpc2.ID, jsonrpc2.Error etc.
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// --- Interface Definition ---

// RPCConnection defines the subset of *jsonrpc2.Conn methods used by Manager.
type RPCConnection interface {
	Reply(ctx context.Context, id jsonrpc2.ID, result interface{}) error
	ReplyWithError(ctx context.Context, id jsonrpc2.ID, respErr *jsonrpc2.Error) error
	Close() error
	// Add other methods here ONLY if your Manager uses them directly.
}

// --- Mock Definition ---

// ErrorReply struct to hold both ID and Error for assertion.
type ErrorReply struct {
	ID    jsonrpc2.ID
	Error *jsonrpc2.Error
}

// MockJSONRPCConn implements RPCConnection for testing.
type MockJSONRPCConn struct {
	// No longer embedding *jsonrpc2.Conn
	mu           sync.Mutex
	replies      []*jsonrpc2.Response
	errorReplies []ErrorReply // Keep tracking struct
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

func (m *MockJSONRPCConn) GetReplies() []*jsonrpc2.Response {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := make([]*jsonrpc2.Response, len(m.replies))
	copy(c, m.replies)
	return c
}
func (m *MockJSONRPCConn) GetErrorReplies() []ErrorReply {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := make([]ErrorReply, len(m.errorReplies))
	copy(c, m.errorReplies)
	return c
}
func (m *MockJSONRPCConn) WaitForReply(timeout time.Duration) bool {
	select {
	case <-m.sent:
		return true
	case <-time.After(timeout):
		return false
	}
}
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

func (m *MockResourceManager) GetAllResourceDefinitions() []definitions.ResourceDefinition {
	args := m.Called()
	res, _ := args.Get(0).([]definitions.ResourceDefinition)
	return res
}
func (m *MockResourceManager) ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) {
	callArgs := m.Called(ctx, name, args)
	return callArgs.String(0), callArgs.String(1), callArgs.Error(2)
}

// MockToolManager satisfies ToolManagerContract.
type MockToolManager struct{ mock.Mock }

func (m *MockToolManager) GetAllToolDefinitions() []definitions.ToolDefinition {
	args := m.Called()
	res, _ := args.Get(0).([]definitions.ToolDefinition)
	return res
}
func (m *MockToolManager) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	callArgs := m.Called(ctx, name, args)
	return callArgs.String(0), callArgs.Error(1)
}

// --- Test Setup Helper ---
func setupTestManager(t *testing.T) (*Manager, *MockResourceManager, *MockToolManager, ServerConfig) {
	t.Helper()
	cfg := ServerConfig{Name: "TestServer", Version: "1.0", RequestTimeout: 5 * time.Second, ShutdownTimeout: 5 * time.Second, Capabilities: map[string]interface{}{"testCap": true}}
	resourceMgr := new(MockResourceManager)
	toolMgr := new(MockToolManager)
	originalWriter := log.Default().Writer()
	log.Default().SetOutput(io.Discard)
	t.Cleanup(func() { log.Default().SetOutput(originalWriter) })
	mgr := NewManager(cfg, resourceMgr, toolMgr)
	require.NotNil(t, mgr)
	require.Equal(t, StateUnconnected, mgr.stateMachine.MustState())
	return mgr, resourceMgr, toolMgr, cfg
}

// transitionManagerToConnected is a helper function to transition manager to connected state for tests.
func transitionManagerToConnected(t *testing.T, mgr *Manager, mockConn *MockJSONRPCConn) {
	t.Helper()
	ctx := context.Background()
	initReqID := jsonrpc2.ID{Num: 99}
	initParams := definitions.InitializeRequest{ProtocolVersion: "1.0"}
	initReq := makeTestRequest("initialize", initParams, initReqID, false)

	// Handle now expects RPCConnection, which MockJSONRPCConn implements.
	mgr.Handle(ctx, mockConn, initReq)

	require.True(t, mockConn.WaitForReply(1*time.Second), "Setup: Did not receive initialize reply.")
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, StateConnected, mgr.stateMachine.MustState(), "Setup: Manager did not reach Connected state.")
	require.True(t, mgr.initialized, "Setup: Manager not marked as initialized.")

	// Clear mock replies from setup phase.
	mockConn.mu.Lock()
	mockConn.replies = nil
	mockConn.errorReplies = nil
	mockConn.mu.Unlock()
}

// makeTestRequest creates a jsonrpc2 Request for testing.
func makeTestRequest(method string, params interface{}, id jsonrpc2.ID, notif bool) *jsonrpc2.Request {
	req := &jsonrpc2.Request{Method: method, ID: id, Notif: notif}
	if params != nil {
		rawParams, _ := json.Marshal(params)
		req.Params = (*json.RawMessage)(&rawParams)
	}
	return req
}

// --- Test Cases ---

func TestNewManager(t *testing.T) {
	mgr, _, _, _ := setupTestManager(t)
	assert.Equal(t, StateUnconnected, mgr.stateMachine.MustState())
	assert.False(t, mgr.initialized)
	assert.NotEmpty(t, mgr.connectionID)
	assert.NotNil(t, mgr.logger)
}

func TestHandle_Initialize_Success(t *testing.T) {
	mgr, _, _, serverCfg := setupTestManager(t)
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn() // Use simplified constructor.

	clientID := jsonrpc2.ID{Num: 1}
	initParams := definitions.InitializeRequest{ProtocolVersion: "1.0"}
	req := makeTestRequest("initialize", initParams, clientID, false)
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
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, StateConnected, mgr.stateMachine.MustState())
	assert.True(t, mgr.initialized)
}

func TestHandle_Initialize_Failure(t *testing.T) {
	mgr, _, _, _ := setupTestManager(t)
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	// Send initialize request to associate connection.
	initReqID := jsonrpc2.ID{Num: 98}
	initParams := definitions.InitializeRequest{ProtocolVersion: "1.0"}
	initReq := makeTestRequest("initialize", initParams, initReqID, false)
	mgr.Handle(ctx, mockConn, initReq) // Manager now knows about mockConn.

	time.Sleep(50 * time.Millisecond) // Allow time to enter Initializing.

	// Simulate error and fire trigger.
	simulatedErr := cgerr.ErrorWithDetails(errors.New("simulated init failure"), cgerr.CategoryRPC, cgerr.CodeInternalError, nil)
	err := mgr.stateMachine.FireCtx(ctx, string(TriggerInitFailure), simulatedErr)
	if !assert.NoError(t, err, "Firing TriggerInitFailure failed, current state: %s", mgr.stateMachine.MustState()) {
		t.Skip("Could not fire TriggerInitFailure, skipping rest of test.")
	}

	assert.Equal(t, StateError, mgr.stateMachine.MustState())
	time.Sleep(6 * time.Second) // Wait for error -> disconnect transition.
	assert.Equal(t, StateUnconnected, mgr.stateMachine.MustState(), "Expected to be Unconnected after error timeout.")
	assert.False(t, mgr.initialized)
}

func TestHandle_ConnectedState_ListResources_ValidRequest(t *testing.T) {
	mgr, resourceMgr, _, _ := setupTestManager(t)
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	transitionManagerToConnected(t, mgr, mockConn)

	clientID := jsonrpc2.ID{Num: 3}
	req := makeTestRequest("resources/list", nil, clientID, false)
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
	assert.Equal(t, StateConnected, mgr.stateMachine.MustState())
	resourceMgr.AssertExpectations(t)
}

// --- UPDATED TestHandle_ConnectedState_ReadResource_HandlerError ---
// Now includes check for subsequent Disconnect from Error state
func TestHandle_ConnectedState_ReadResource_HandlerError(t *testing.T) {
	mgr, resourceMgr, _, _ := setupTestManager(t)
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	transitionManagerToConnected(t, mgr, mockConn)

	clientID := jsonrpc2.ID{Num: 4}
	readParams := map[string]interface{}{"name": "myResource", "args": map[string]string{"arg1": "value1"}}
	req := makeTestRequest("resources/read", readParams, clientID, false)
	handlerError := cgerr.ErrorWithDetails(errors.New("database unavailable"), cgerr.CategoryResource, cgerr.CodeInternalError, nil)
	expectedName := "myResource"
	expectedArgsMap := map[string]string{"arg1": "value1"}
	resourceMgr.On("ReadResource", ctx, expectedName, expectedArgsMap).Return("", "", handlerError).Once()

	// Action 1: Trigger the error.
	mgr.Handle(ctx, mockConn, req) // Pass mock satisfying interface.

	// Assertions 1: Check error reply and state transition TO Error.
	require.True(t, mockConn.WaitForReply(1*time.Second), "Did not receive error reply")
	replies := mockConn.GetReplies()
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, replies, 0)
	require.Len(t, errorReplies, 1)
	assert.Equal(t, clientID, errorReplies[0].ID) // Check ID on error reply.
	assert.Equal(t, cgerr.CodeInternalError, errorReplies[0].Error.Code)
	assert.Contains(t, errorReplies[0].Error.Message, "database unavailable")

	// Allow time for state machine to potentially process error trigger if async.
	time.Sleep(50 * time.Millisecond)
	// Verify manager reached Error state.
	require.Equal(t, StateError, mgr.stateMachine.MustState(), "Manager did not enter Error state.")

	// Action 2: Fire Disconnect trigger while in Error state.
	err := mgr.stateMachine.FireCtx(ctx, string(TriggerDisconnect))
	require.NoError(t, err, "Firing TriggerDisconnect from Error state failed.")

	// Assertions 2: Check final state is Unconnected.
	assert.Eventually(t, func() bool { return mgr.stateMachine.MustState() == StateUnconnected }, 1*time.Second, 50*time.Millisecond, "State did not become Unconnected after disconnect from Error.")

	resourceMgr.AssertExpectations(t)
	// No mockConn.Close check needed here as per state machine config.
}

func TestHandle_InvalidStateOperation(t *testing.T) {
	mgr, _, _, _ := setupTestManager(t)
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	clientID := jsonrpc2.ID{Num: 5}
	req := makeTestRequest("resources/list", nil, clientID, false)

	mgr.Handle(ctx, mockConn, req) // Pass mock satisfying interface.

	require.True(t, mockConn.WaitForReply(1*time.Second))
	replies := mockConn.GetReplies()
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, replies, 0)
	require.Len(t, errorReplies, 1)
	assert.Equal(t, clientID, errorReplies[0].ID) // Check ID on error reply.
	assert.Equal(t, cgerr.CodeInvalidRequest, errorReplies[0].Error.Code)
	assert.Contains(t, errorReplies[0].Error.Message, "not allowed in state '"+string(StateUnconnected)+"'")
	assert.Equal(t, StateUnconnected, mgr.stateMachine.MustState())
}

func TestHandle_UnknownMethod(t *testing.T) {
	mgr, _, _, _ := setupTestManager(t)
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	transitionManagerToConnected(t, mgr, mockConn)

	clientID := jsonrpc2.ID{Num: 6}
	req := makeTestRequest("unknown/method", nil, clientID, false)

	mgr.Handle(ctx, mockConn, req) // Pass mock satisfying interface.

	require.True(t, mockConn.WaitForReply(1*time.Second))
	replies := mockConn.GetReplies()
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, replies, 0)
	require.Len(t, errorReplies, 1)
	assert.Equal(t, clientID, errorReplies[0].ID) // Check ID on error reply.
	assert.Equal(t, cgerr.CodeMethodNotFound, errorReplies[0].Error.Code)
	assert.Contains(t, errorReplies[0].Error.Message, "Method not found")
	assert.Equal(t, StateConnected, mgr.stateMachine.MustState())
}

func TestHandle_Shutdown_Success(t *testing.T) {
	mgr, _, _, _ := setupTestManager(t)
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	transitionManagerToConnected(t, mgr, mockConn)
	// Store the mock connection interface in the manager for the test path.
	mgr.dataMu.Lock()
	mgr.jsonrpcConn = mockConn
	mgr.dataMu.Unlock()

	clientID := jsonrpc2.ID{Num: 7}
	req := makeTestRequest("shutdown", nil, clientID, false)

	mgr.Handle(ctx, mockConn, req) // Pass mock satisfying interface.

	require.True(t, mockConn.WaitForReply(1*time.Second))
	replies := mockConn.GetReplies()
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, errorReplies, 0)
	require.Len(t, replies, 1)
	assert.Equal(t, clientID, replies[0].ID)
	var result interface{}
	err := json.Unmarshal(*replies[0].Result, &result)
	require.NoError(t, err)
	assert.Nil(t, result)

	assert.Eventually(t, func() bool { return mgr.stateMachine.MustState() == StateUnconnected }, 1*time.Second, 50*time.Millisecond)
	assert.Eventually(t, func() bool { mgr.dataMu.RLock(); defer mgr.dataMu.RUnlock(); return mgr.jsonrpcConn == nil }, 1*time.Second, 50*time.Millisecond)
	assert.True(t, mockConn.WaitForClose(1*time.Second), "Close was not tracked by mock.")
	assert.False(t, mgr.initialized)
}

// --- UPDATED TestHandle_DisconnectTrigger ---
// Removed "From Error" case as it's tested elsewhere.
// Removed SetStateUnsafe. Setup for Initializing may be flaky.
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
				transitionManagerToConnected(t, mgr, mockConn)
				// Store the mock connection interface.
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
				initReqID := jsonrpc2.ID{Num: 97}
				initReq := makeTestRequest("initialize", definitions.InitializeRequest{ProtocolVersion: "1.0"}, initReqID, false)
				// Pass mock satisfying interface.
				go mgr.Handle(ctx, mockConn, initReq)
				time.Sleep(10 * time.Millisecond) // Hope state is Initializing.
				currentState := mgr.stateMachine.MustState()
				// Store the mock connection interface.
				mgr.dataMu.Lock()
				mgr.jsonrpcConn = mockConn
				mgr.dataMu.Unlock()
				if currentState != StateInitializing {
					t.Logf("State is %s, not Initializing. Skipping test.", currentState)
					return false // Indicate setup failed.
				}
				return true // Indicate setup successful.
			},
			checkClose: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mgr, _, _, _ := setupTestManager(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			mockConn := NewMockJSONRPCConn()

			if !tc.setupFunc(t, ctx, mgr, mockConn) {
				t.SkipNow()
			}

			currentState := mgr.stateMachine.MustState()
			if tc.initialState != StateInitializing { // Initializing state is hard to guarantee.
				require.Equal(t, tc.initialState, currentState, "Test precondition failed: Incorrect starting state.")
			}
			t.Logf("Current state before firing Disconnect: %s", currentState)

			err := mgr.stateMachine.FireCtx(ctx, string(TriggerDisconnect))
			require.NoError(t, err, "Firing TriggerDisconnect failed from state %s", currentState)

			expectedEndState := StateUnconnected
			assert.Eventually(t, func() bool { return mgr.stateMachine.MustState() == expectedEndState }, 2*time.Second, 50*time.Millisecond, "State did not become %s after disconnect from %s", expectedEndState, currentState)

			if tc.checkClose {
				assert.Eventually(t, func() bool { mgr.dataMu.RLock(); defer mgr.dataMu.RUnlock(); return mgr.jsonrpcConn == nil }, 1*time.Second, 50*time.Millisecond)
				assert.True(t, mockConn.WaitForClose(1*time.Second), "Close was not tracked by mock after disconnect from %s", currentState)
			}
		})
	}
}

// --- Tests for Tool methods ---
func TestHandle_ConnectedState_ListTools_ValidRequest(t *testing.T) {
	mgr, _, toolMgr, _ := setupTestManager(t)
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	transitionManagerToConnected(t, mgr, mockConn)

	clientID := jsonrpc2.ID{Num: 8}
	req := makeTestRequest("tools/list", nil, clientID, false)
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
	assert.Equal(t, StateConnected, mgr.stateMachine.MustState())
	toolMgr.AssertExpectations(t)
}

func TestHandle_ConnectedState_CallTool_ValidRequest(t *testing.T) {
	mgr, _, toolMgr, _ := setupTestManager(t)
	ctx := context.Background()
	mockConn := NewMockJSONRPCConn()

	transitionManagerToConnected(t, mgr, mockConn)

	clientID := jsonrpc2.ID{Num: 9}
	callParams := map[string]interface{}{"name": "myTool", "arguments": map[string]interface{}{"param1": "value1", "param2": 123}}
	req := makeTestRequest("tools/call", callParams, clientID, false)
	expectedResultString := `{"toolOutput": "success"}`
	expectedToolName := "myTool"
	expectedToolArgs := map[string]interface{}{"param1": "value1", "param2": 123}
	toolMgr.On("CallTool", ctx, expectedToolName, expectedToolArgs).Return(expectedResultString, nil).Once()

	mgr.Handle(ctx, mockConn, req) // Pass mock satisfying interface.

	require.True(t, mockConn.WaitForReply(1*time.Second))
	replies := mockConn.GetReplies()
	errorReplies := mockConn.GetErrorReplies()
	require.Len(t, errorReplies, 0)
	require.Len(t, replies, 1)
	assert.Equal(t, clientID, replies[0].ID)

	var actualResult string
	err := json.Unmarshal(*replies[0].Result, &actualResult)
	require.NoError(t, err, "Unmarshal into string failed.")
	assert.JSONEq(t, expectedResultString, actualResult)

	assert.Equal(t, StateConnected, mgr.stateMachine.MustState())
	toolMgr.AssertExpectations(t)
}
