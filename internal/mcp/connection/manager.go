// file: internal/mcp/connection/manager.go
package connection

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/google/uuid"
	"github.com/qmuntal/stateless"
	"github.com/sourcegraph/jsonrpc2"
)

// ServerConfig holds configuration specific to the connection manager's behavior.
type ServerConfig struct {
	Name            string
	Version         string
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	Capabilities    map[string]interface{}
}

// Manager orchestrates the state and communication for a single client connection.
type Manager struct {
	connectionID       string
	config             ServerConfig
	resourceManager    ResourceManagerContract
	toolManager        ToolManagerContract
	stateMachine       *stateless.StateMachine
	jsonrpcConn        *jsonrpc2.Conn
	clientCapabilities map[string]interface{}
	dataMu             sync.RWMutex // Protects clientCapabilities or other shared data
	logger             *log.Logger  // Example logger
	// shutdownFunc     context.CancelFunc // Consider if needed for graceful shutdown coordination
}

// NewManager creates and initializes a new Manager.
func NewManager(
	config ServerConfig,
	resourceMgr ResourceManagerContract,
	toolMgr ToolManagerContract,
) *Manager {
	connID := uuid.NewString()
	// Configure a simple logger (replace with your actual logger)
	logger := log.New(log.Default().Writer(), fmt.Sprintf("CONN [%s] ", connID), log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	m := &Manager{
		connectionID:       connID,
		config:             config,
		resourceManager:    resourceMgr,
		toolManager:        toolMgr,
		logger:             logger,
		clientCapabilities: make(map[string]interface{}),
	}

	// State Machine Setup
	m.stateMachine = stateless.NewStateMachine(StateUnconnected)

	// Configure states and transitions
	m.stateMachine.Configure(StateUnconnected).
		Permit(TriggerInitialize, StateInitializing)

	m.stateMachine.Configure(StateInitializing).
		OnEntryFrom(TriggerInitialize, m.onEnterInitializing). // Use action handler
		Permit(TriggerInitSuccess, StateConnected).
		Permit(TriggerInitFailure, StateError).
		Permit(TriggerDisconnect, StateTerminating) // Allow disconnect during init

	m.stateMachine.Configure(StateConnected).
		OnEntry(m.onEnterConnected).
		// Permit requests that keep the connection in the Connected state
		PermitReentry(TriggerListResources). // PermitReentry might be suitable if action needed
		PermitReentry(TriggerReadResource).
		PermitReentry(TriggerListTools).
		PermitReentry(TriggerCallTool).
		PermitReentry(TriggerPing).
		PermitReentry(TriggerSubscribe).
		// Define actions for these re-entry triggers if needed using .OnEntryFrom()
		Permit(TriggerShutdown, StateTerminating).
		Permit(TriggerDisconnect, StateTerminating).
		Permit(TriggerErrorOccurred, StateError)

	m.stateMachine.Configure(StateTerminating).
		OnEntry(m.onEnterTerminating). // Handle cleanup
		Permit(TriggerShutdownComplete, StateUnconnected).
		Permit(TriggerDisconnect, StateUnconnected) // Ensure disconnect leads back

	m.stateMachine.Configure(StateError).
		OnEntry(m.onEnterError).                    // Log/handle error state entry
		Permit(TriggerDisconnect, StateUnconnected) // Allow disconnect from error state

	m.logf(definitions.LogLevelDebug, "Manager created")
	return m
}

// Handle is the main entry point for incoming JSON-RPC requests.
func (m *Manager) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	m.dataMu.Lock() // Lock if modifying shared state like jsonrpcConn
	m.jsonrpcConn = conn
	m.dataMu.Unlock()

	trigger, ok := MapMethodToTrigger(req.Method)
	if !ok {
		m.logf(definitions.LogLevelWarn, "Received unknown method: %s", req.Method)
		respErr := &jsonrpc2.Error{
			Code:    jsonrpc2.CodeMethodNotFound,
			Message: fmt.Sprintf("Method not found: %s", req.Method),
		}
		if err := conn.ReplyWithError(ctx, req.ID, respErr); err != nil {
			m.logf(definitions.LogLevelError, "Error sending MethodNotFound reply: %v", err)
		}
		return
	}

	m.logf(definitions.LogLevelDebug, "Mapping method '%s' to trigger '%s'", req.Method, trigger)
	currentState := m.stateMachine.MustState().(State)

	// Fire the trigger
	err := m.stateMachine.FireCtx(ctx, string(trigger), req) // Pass request as argument to actions

	if err != nil {
		// Simplified check: Any error firing means invalid state/transition for this trigger
		m.logf(definitions.LogLevelError, "Error firing trigger '%s' from state '%s': %v", trigger, currentState, err)
		respErr := &jsonrpc2.Error{
			Code:    jsonrpc2.CodeInvalidRequest, // Standard code for invalid request in current context
			Message: fmt.Sprintf("Operation '%s' not allowed in current state '%s'", req.Method, currentState),
		}
		if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
			m.logf(definitions.LogLevelError, "Error sending state transition error reply: %v", replyErr)
		}
		// Optionally transition to error state on firing errors
		_ = m.stateMachine.Fire(string(TriggerErrorOccurred), err)
		return
	}

	// Log successful state transition if it occurred
	newState := m.stateMachine.MustState().(State)
	if currentState != newState {
		m.logf(definitions.LogLevelDebug, "State transition: %s -> %s (Trigger: %s)", currentState, newState, trigger)
	} else {
		m.logf(definitions.LogLevelDebug, "Trigger '%s' processed in state '%s' (no state change)", trigger, currentState)
	}
	// Responses are sent by the action handlers (e.g., onEnterInitializing)
}

// onEnterInitializing is called when entering the Initializing state.
func (m *Manager) onEnterInitializing(ctx context.Context, args ...interface{}) error {
	if len(args) == 0 {
		return errors.New("missing request argument for onEnterInitializing")
	}
	req, ok := args[0].(*jsonrpc2.Request)
	if !ok {
		return errors.New("invalid request argument type for onEnterInitializing")
	}

	// Call the specific handler logic from handlers.go
	result, err := m.handleInitialize(ctx, req)

	// Get the active connection (might have been updated)
	m.dataMu.RLock()
	conn := m.jsonrpcConn
	m.dataMu.RUnlock()
	if conn == nil {
		m.logf(definitions.LogLevelError, "Initialization completed but connection is nil, cannot reply")
		// Fire failure even though logic succeeded, as we can't communicate back
		_ = m.stateMachine.Fire(string(TriggerInitFailure), errors.New("connection closed before init reply"))
		return errors.New("connection closed before init reply")
	}

	if err != nil {
		m.logf(definitions.LogLevelError, "Initialization failed: %v", err)
		// Convert Go error to JSON-RPC error
		respErr := cgerr.ToJSONRPCError(err)
		if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
			m.logf(definitions.LogLevelError, "Error sending initialization failure reply: %v", replyErr)
		}
		// Fire failure trigger, which might transition to Error state
		_ = m.stateMachine.Fire(string(TriggerInitFailure), err)
		return err // Propagate the original error if needed by state machine internals
	}

	// Success case
	if replyErr := conn.Reply(ctx, req.ID, result); replyErr != nil {
		m.logf(definitions.LogLevelError, "Error sending initialization success reply: %v", replyErr)
		// If reply fails, we consider initialization failed overall
		_ = m.stateMachine.Fire(string(TriggerInitFailure), replyErr)
		return replyErr // Return the reply error
	}

	// Fire success trigger to move to next state (e.g., Connected)
	if fireErr := m.stateMachine.Fire(string(TriggerInitSuccess)); fireErr != nil {
		m.logf(definitions.LogLevelError, "Error firing TriggerInitSuccess: %v", fireErr)
		// This is an internal state machine issue, maybe transition to Error state
		_ = m.stateMachine.Fire(string(TriggerErrorOccurred), fireErr)
		return fireErr
	}

	return nil // Indicate success to state machine
}

// onEnterConnected is called when entering the Connected state.
func (m *Manager) onEnterConnected(ctx context.Context, args ...interface{}) error {
	m.logf(definitions.LogLevelInfo, "Connection established and initialized")
	return nil
}

// onEnterTerminating is called when entering the Terminating state.
func (m *Manager) onEnterTerminating(ctx context.Context, args ...interface{}) error {
	m.logf(definitions.LogLevelInfo, "Connection terminating...")
	// Perform cleanup actions here

	// Signal completion
	if fireErr := m.stateMachine.Fire(string(TriggerShutdownComplete)); fireErr != nil {
		m.logf(definitions.LogLevelWarn, "Could not fire ShutdownComplete, already disconnected? %v", fireErr)
	}
	return nil
}

// onEnterError is called when entering the Error state.
func (m *Manager) onEnterError(ctx context.Context, args ...interface{}) error {
	errMsg := "Unknown internal error"
	if len(args) > 0 {
		if err, ok := args[0].(error); ok {
			errMsg = err.Error()
		} else {
			errMsg = fmt.Sprintf("%+v", args[0])
		}
	}
	m.logf(definitions.LogLevelError, "Connection entered error state: %s", errMsg)
	return nil
}

// logf is a helper for logging with connection ID prefix.
func (m *Manager) logf(level definitions.LogLevel, format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	m.logger.Printf("[%s] %s", level, message)
}

// NewConnectionServer creates a new connection manager styled as a server.
func NewConnectionServer(serverConfig ServerConfig, resourceMgr ResourceManagerContract, toolMgr ToolManagerContract) (*Manager, error) {
	return NewManager(serverConfig, resourceMgr, toolMgr), nil
}
