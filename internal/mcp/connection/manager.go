// file: internal/mcp/connection/manager.go
package connection

import (
	"context"
	"fmt"
	"log" // Using standard logger for simplicity, replace with your preferred one
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/google/uuid"
	"github.com/qmuntal/stateless" // State machine library
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

// ConnectionManager orchestrates the state and communication for a single client connection.
type ConnectionManager struct {
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

// NewConnectionManager creates and initializes a new ConnectionManager.
func NewConnectionManager(
	config ServerConfig,
	resourceMgr ResourceManagerContract,
	toolMgr ToolManagerContract,
	// logger *log.Logger, // Optional: Inject logger
) *ConnectionManager {

	connID := uuid.NewString()
	// Configure a simple logger (replace with your actual logger)
	logger := log.New(log.Default().Writer(), fmt.Sprintf("CONN [%s] ", connID), log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	m := &ConnectionManager{
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

	m.logf(definitions.LogLevelDebug, "ConnectionManager created")
	return m
}

// Handle is the main entry point for incoming JSON-RPC requests.
func (m *ConnectionManager) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
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

	// CHANGED: Use LogLevelDebug instead of LogLevelTrace
	m.logf(definitions.LogLevelDebug, "Mapping method '%s' to trigger '%s'", req.Method, trigger)
	currentState := m.stateMachine.MustState().(ConnectionState)

	// Fire the trigger
	err := m.stateMachine.FireCtx(ctx, trigger, req) // Pass request as argument to actions

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
		_ = m.stateMachine.Fire(TriggerErrorOccurred, err)
		return
	}

	// Log successful state transition if it occurred
	newState := m.stateMachine.MustState().(ConnectionState)
	if currentState != newState {
		m.logf(definitions.LogLevelDebug, "State transition: %s -> %s (Trigger: %s)", currentState, newState, trigger)
	} else {
		// CHANGED: Use LogLevelDebug instead of LogLevelTrace
		m.logf(definitions.LogLevelDebug, "Trigger '%s' processed in state '%s' (no state change)", trigger, currentState)
	}
	// Responses are sent by the action handlers (e.g., onEnterInitializing)
}

// --- State Machine Action Handlers ---

func (m *ConnectionManager) onEnterInitializing(ctx context.Context, args ...interface{}) error {
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
		_ = m.stateMachine.Fire(TriggerInitFailure, errors.New("connection closed before init reply"))
		return errors.New("connection closed before init reply")
	}

	if err != nil {
		m.logf(definitions.LogLevelError, "Initialization failed: %v", err)
		// Use the new helper to convert Go error to JSON-RPC error
		respErr := cgerr.ToJSONRPCError(err)
		if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
			m.logf(definitions.LogLevelError, "Error sending initialization failure reply: %v", replyErr)
		}
		// Fire failure trigger, which might transition to Error state
		_ = m.stateMachine.Fire(TriggerInitFailure, err)
		return err // Propagate the original error if needed by state machine internals
	}

	// Success case
	if replyErr := conn.Reply(ctx, req.ID, result); replyErr != nil {
		m.logf(definitions.LogLevelError, "Error sending initialization success reply: %v", replyErr)
		// If reply fails, we consider initialization failed overall
		_ = m.stateMachine.Fire(TriggerInitFailure, replyErr)
		return replyErr // Return the reply error
	}

	// Fire success trigger to move to next state (e.g., Connected)
	if fireErr := m.stateMachine.Fire(TriggerInitSuccess); fireErr != nil {
		m.logf(definitions.LogLevelError, "Error firing TriggerInitSuccess: %v", fireErr)
		// This is an internal state machine issue, maybe transition to Error state
		_ = m.stateMachine.Fire(TriggerErrorOccurred, fireErr)
		return fireErr
	}

	return nil // Indicate success to state machine
}

func (m *ConnectionManager) onEnterConnected(ctx context.Context, args ...interface{}) error {
	m.logf(definitions.LogLevelInfo, "Connection established and initialized")
	// Maybe send a server ready notification if applicable
	// m.sendNotification(ctx, "$/serverReady", nil)
	return nil
}

func (m *ConnectionManager) onEnterTerminating(ctx context.Context, args ...interface{}) error {
	m.logf(definitions.LogLevelInfo, "Connection terminating...")
	// Perform cleanup actions here (e.g., release resources, close subscriptions)

	// Signal completion (could be immediate or after cleanup)
	// This might trigger the transition to Unconnected
	if fireErr := m.stateMachine.Fire(TriggerShutdownComplete); fireErr != nil {
		// If firing fails, maybe force state if needed, or just log
		m.logf(definitions.LogLevelWarn, "Could not fire ShutdownComplete, already disconnected? %v", fireErr)
		// Could manually transition if absolutely necessary: m.stateMachine.ForceState(StateUnconnected)
	}
	return nil
}

func (m *ConnectionManager) onEnterError(ctx context.Context, args ...interface{}) error {
	errMsg := "Unknown internal error"
	if len(args) > 0 {
		if err, ok := args[0].(error); ok {
			errMsg = err.Error()
		} else {
			errMsg = fmt.Sprintf("%+v", args[0]) // More detail for non-error args
		}
	}
	m.logf(definitions.LogLevelError, "Connection entered error state: %s", errMsg)
	// Optionally notify the client if the connection is still viable
	// m.sendNotification(ctx, "$/protocolError", map[string]interface{}{"message": errMsg})
	return nil // Don't return errors from entering the error state itself
}

// logf is a helper for logging with connection ID prefix.
func (m *ConnectionManager) logf(level definitions.LogLevel, format string, v ...interface{}) {
	// Add level check if your logger supports it
	message := fmt.Sprintf(format, v...)
	m.logger.Printf("[%s] %s", level, message) // Basic logging
}

// sendNotification sends a JSON-RPC notification to the client.
func (m *ConnectionManager) sendNotification(ctx context.Context, method string, params interface{}) {
	m.dataMu.RLock()
	conn := m.jsonrpcConn
	m.dataMu.RUnlock()

	if conn == nil {
		m.logf(definitions.LogLevelWarn, "Cannot send notification, no active connection (method: %s)", method)
		return
	}

	// Use a background context potentially, as the original request context might be done
	notifyCtx := context.Background() // Or manage context differently if needed

	if err := conn.Notify(notifyCtx, method, params); err != nil {
		// Check for common closed connection errors
		if errors.Is(err, jsonrpc2.ErrClosed) || errors.Is(err, jsonrpc2.ErrNotConnected) {
			m.logf(definitions.LogLevelWarn, "Could not send notification, connection closed (method: %s)", method)
		} else {
			m.logf(definitions.LogLevelError, "Error sending notification (method: %s): %v", method, err)
		}
		// If sending fails, maybe trigger disconnect state
		_ = m.stateMachine.Fire(TriggerDisconnect)
	}
}
