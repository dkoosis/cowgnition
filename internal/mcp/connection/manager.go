// file: internal/mcp/connection/manager.go
package connection

import (
	"context"
	"encoding/json"
	"fmt"
	"log" // Needed for SetTriggerParameters example
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	// Assuming your custom error package exists:
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	stateless "github.com/qmuntal/stateless"
	"github.com/sourcegraph/jsonrpc2"
)

// --- Assumed Interfaces (Ensure these are defined elsewhere) ---
type ResourceManager interface {
	GetAllResourceDefinitions() []interface{} // Replace 'interface{}' with actual type
	ReadResource(ctx context.Context, name string, args map[string]string) (content []byte, mimeType string, err error)
}
type ToolManager interface {
	GetAllToolDefinitions() []interface{} // Replace 'interface{}' with actual type
	CallTool(ctx context.Context, name string, args map[string]interface{}) (result []byte, err error)
}

// --- MessageHandler & ServerConfig (as before) ---
type MessageHandler func(ctx context.Context, req *jsonrpc2.Request) (interface{}, error)

type ServerConfig struct {
	Name            string
	Version         string
	Capabilities    map[string]interface{}
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
}

// --- ConnectionManager Struct (Refactored) ---
type ConnectionManager struct {
	// State Machine
	fsm *stateless.StateMachine

	// Dependencies & Config
	config          ServerConfig
	resourceManager ResourceManager
	toolManager     ToolManager
	logger          *log.Logger

	// Connection Specific Data (Protected by dataMu)
	dataMu             sync.RWMutex
	currentState       ConnectionState // For FSM external storage persistence
	conn               *jsonrpc2.Conn
	connectionID       string
	startTime          time.Time
	lastActivity       time.Time
	clientCapabilities map[string]interface{}

	// Request Handling
	triggerHandlers map[Trigger]MessageHandler // Maps Triggers to handler funcs

	// Context Management
	baseCtx   context.Context    // Base context for the manager's lifetime
	cancelCtx context.CancelFunc // To cancel operations on shutdown
}

// --- Constructor and FSM Setup ---

// NewConnectionManager creates a new ConnectionManager.
func NewConnectionManager(config ServerConfig, resourceManager ResourceManager, toolManager ToolManager) *ConnectionManager {
	baseCtx, cancel := context.WithCancel(context.Background())

	manager := &ConnectionManager{
		config:             config,
		resourceManager:    resourceManager,
		toolManager:        toolManager,
		logger:             log.New(log.Writer(), "[MCP] ", log.LstdFlags|log.Lshortfile), // Added Lshortfile
		connectionID:       generateConnectionID(),
		startTime:          time.Now(),
		lastActivity:       time.Now(),
		clientCapabilities: make(map[string]interface{}),
		triggerHandlers:    make(map[Trigger]MessageHandler),
		baseCtx:            baseCtx,
		cancelCtx:          cancel,
		// Initialize state for FSM storage
		currentState: StateUnconnected,
	}

	// Initialize the FSM using external storage functions
	manager.fsm = stateless.NewStateMachineWithExternalStorage(manager.getState, manager.setState, stateless.FiringQueued)

	// Define the state machine structure
	manager.configureStateMachine()

	// Map triggers to the actual handler implementations
	manager.registerTriggerHandlers()

	manager.logf(LogLevelInfo, "ConnectionManager created (id: %s, initial state: %s)", manager.connectionID, manager.currentState)
	return manager
}

// getState provides the FSM current state (for external storage).
func (m *ConnectionManager) getState(ctx context.Context) (stateless.State, error) {
	m.dataMu.RLock()
	defer m.dataMu.RUnlock()
	return m.currentState, nil
}

// setState updates the FSM state (for external storage).
func (m *ConnectionManager) setState(ctx context.Context, state stateless.State) error {
	m.dataMu.Lock()
	defer m.dataMu.Unlock()

	oldState := m.currentState
	newState, ok := state.(ConnectionState) // Assert to your specific type
	if !ok {
		err := errors.Newf("internal error: invalid state type provided by FSM: %T", state)
		m.logf(LogLevelError, "CRITICAL: Failed to set state: %v", err)
		m.currentState = StateError // Force error state on programming error
		return err
	}

	// Only log if state actually changes
	if oldState != newState {
		m.currentState = newState
		m.lastActivity = time.Now()
		m.logf(LogLevelInfo, "Connection state changed: %s -> %s (id: %s)", oldState, newState, m.connectionID)
	}
	return nil
}

// configureStateMachine defines the FSM transitions and actions.
func (m *ConnectionManager) configureStateMachine() {
	// Example: Provide type hints for trigger parameters (useful for reflection-based features if ever needed)
	// m.fsm.SetTriggerParameters(TriggerInitialize, reflect.TypeOf(InitializeRequest{}))

	m.fsm.Configure(StateUnconnected).
		Permit(TriggerInitialize, StateInitializing)

	m.fsm.Configure(StateInitializing).
		Permit(TriggerInitSuccess, StateConnected).
		Permit(TriggerInitFailure, StateError).
		Permit(TriggerShutdown, StateTerminating)

	m.fsm.Configure(StateConnected).
		PermitReentry(TriggerListResources).
		PermitReentry(TriggerReadResource).
		PermitReentry(TriggerListTools).
		PermitReentry(TriggerCallTool).
		Permit(TriggerShutdown, StateTerminating).
		Permit(TriggerErrorOccurred, StateError).
		Permit(TriggerDisconnect, StateTerminating) // Assume unexpected disconnect leads to termination

	m.fsm.Configure(StateTerminating).
		OnEntry(m.performGracefulShutdown).                // Action executed upon entering this state
		Permit(TriggerShutdownComplete, StateUnconnected). // Transition after shutdown logic finishes
		Ignore(TriggerShutdown).                           // Ignore redundant shutdown triggers
		Ignore(TriggerDisconnect)                          // Ignore disconnects while already terminating

	m.fsm.Configure(StateError).
		OnEntry(func(ctx context.Context, args ...interface{}) error {
			m.logf(LogLevelError, "Connection entered ERROR state (id: %s)", m.connectionID)
			// Force immediate shutdown from error state? Or wait for explicit shutdown?
			// Let's trigger termination automatically from error state for cleanup.
			// Use FireAsync for safety within action handler if not using FiringQueued
			go m.fsm.Fire(TriggerShutdown) // Trigger shutdown asynchronously
			return nil
		}).
		Permit(TriggerShutdown, StateTerminating) // Allow explicit shutdown from error state

	// Handle triggers that aren't configured for the current state
	m.fsm.OnUnhandledTrigger(func(ctx context.Context, state stateless.State, trigger stateless.Trigger, unhandledTriggerArgs ...interface{}) error {
		m.logf(LogLevelWarning, "Unhandled trigger '%s' in state '%s' (id: %s)", trigger, state, m.connectionID)
		// Return nil to ignore, or return an error to propagate
		return errors.Newf("trigger %s not permitted in state %s", trigger, state)
	})
}

// registerTriggerHandlers maps triggers to their corresponding handler functions.
func (m *ConnectionManager) registerTriggerHandlers() {
	m.triggerHandlers[TriggerInitialize] = m.handleInitialize
	m.triggerHandlers[TriggerListResources] = m.handleListResources
	m.triggerHandlers[TriggerReadResource] = m.handleReadResource
	m.triggerHandlers[TriggerListTools] = m.handleListTools
	m.triggerHandlers[TriggerCallTool] = m.handleCallTool
	m.triggerHandlers[TriggerShutdown] = m.handleShutdownRequest
	// Add other handlers...
}

// --- Public Methods ---

// Connect stores the connection; state is managed by FSM based on subsequent 'initialize'.
func (m *ConnectionManager) Connect(conn *jsonrpc2.Conn) error {
	m.dataMu.Lock()
	if m.conn != nil {
		m.dataMu.Unlock()
		m.logf(LogLevelWarning, "Connect called on already connected manager (id: %s)", m.connectionID)
		// Optionally close old connection or return error
		// For now, just log and update conn
	}
	m.conn = conn
	m.startTime = time.Now() // Reset start time on new connection
	m.lastActivity = time.Now()
	m.dataMu.Unlock()

	m.logf(LogLevelInfo, "Connection transport established (id: %s)", m.connectionID)
	// Connection remains in StateUnconnected until successful Initialize call
	return nil
}

// Shutdown initiates the shutdown sequence by firing the trigger.
func (m *ConnectionManager) Shutdown() error {
	m.logf(LogLevelInfo, "Shutdown requested externally (id: %s)", m.connectionID)
	// Use FireCtx to potentially pass context if actions need it
	err := m.fsm.FireCtx(m.baseCtx, TriggerShutdown)
	// Ignore ErrNoTransition, as it means we are already shutting down or terminated
	if err != nil && !errors.Is(err, stateless.ErrNoTransition) {
		m.logf(LogLevelError, "Error firing shutdown trigger: %v (id: %s)", err, m.connectionID)
		return errors.Wrap(err, "failed to initiate shutdown")
	}
	m.logf(LogLevelDebug, "Shutdown trigger fired successfully or was ignored (id: %s)", m.connectionID)
	return nil
}

// performGracefulShutdown is the OnEntry action for StateTerminating.
func (m *ConnectionManager) performGracefulShutdown(ctx context.Context, args ...interface{}) error {
	m.logf(LogLevelInfo, "Performing graceful shutdown action (id: %s)", m.connectionID)

	// 1. Cancel context for any dependent operations
	m.cancelCtx() // Cancel the manager's base context

	// 2. Close the underlying connection safely
	m.dataMu.Lock()
	connToClose := m.conn
	m.conn = nil // Prevent further use
	m.dataMu.Unlock()

	if connToClose != nil {
		m.logf(LogLevelDebug, "Closing underlying jsonrpc2 connection (id: %s)", m.connectionID)
		// Consider using CloseContext with a timeout from config
		// closeCtx, closeCancel := context.WithTimeout(context.Background(), m.config.ShutdownTimeout)
		// defer closeCancel()
		// err := connToClose.CloseContext(closeCtx)
		err := connToClose.Close() // Simple close
		if err != nil {
			m.logf(LogLevelError, "Error closing underlying connection: %v (id: %s)", err, m.connectionID)
			// Log error but continue the shutdown process
		}
	} else {
		m.logf(LogLevelDebug, "No active connection to close during shutdown (id: %s)", m.connectionID)
	}

	// 3. Fire completion trigger to move to the final state (StateUnconnected)
	// Use FireCtx if context is needed by subsequent actions/guards
	// Since this is the end, context might not matter as much.
	err := m.fsm.Fire(TriggerShutdownComplete)
	if err != nil {
		// This indicates a configuration error or unexpected issue
		m.logf(LogLevelError, "CRITICAL: Error firing ShutdownComplete trigger: %v (id: %s)", err, m.connectionID)
	} else {
		m.logf(LogLevelInfo, "Graceful shutdown action complete (id: %s)", m.connectionID)
	}
	// Return nil from OnEntry action unless you want to block further state processing
	return nil
}

// --- jsonrpc2.Handler Implementation ---

// Handle implements the jsonrpc2.Handler interface using the state machine.
func (m *ConnectionManager) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	// Ensure conn is set and update last activity atomically
	m.dataMu.Lock()
	if m.conn == nil {
		m.conn = conn
		m.logf(LogLevelInfo, "jsonrpc2.Conn associated on first Handle call (id: %s)", m.connectionID)
	}
	currentConn := m.conn // Use the stored connection for replies
	m.lastActivity = time.Now()
	m.dataMu.Unlock()

	m.logf(LogLevelDebug, "Handle request: %s (id: %s, req.ID: %s)", req.Method, m.connectionID, req.ID)

	// 1. Map method string to Trigger
	trigger, methodKnown := MapMethodToTrigger(req.Method)
	if !methodKnown {
		if !req.Notif {
			err := cgerr.NewMethodNotFoundError(req.Method, map[string]interface{}{"connection_id": m.connectionID})
			m.handleError(ctx, currentConn, req, err) // Use stored conn for reply
		} else {
			m.logf(LogLevelDebug, "Ignoring notification for unknown method %s (id: %s)", req.Method, m.connectionID)
		}
		return
	}

	// Use FireCtx to pass context down to actions/guards if needed
	fireCtx := m.baseCtx // Use manager's base context by default for FSM operations

	// 2. Check if the trigger is permitted *before* executing handler
	if !m.fsm.CanFireCtx(fireCtx, trigger) {
		currentState := m.fsm.MustStateCtx(fireCtx)
		if !req.Notif {
			err := cgerr.ErrorWithDetails(
				errors.Newf("method '%s' (trigger '%s') not permitted in state '%s'", req.Method, trigger, currentState),
				cgerr.CategoryRPC,
				cgerr.CodeInvalidRequest,
				map[string]interface{}{
					"connection_id": m.connectionID,
					"current_state": currentState,
					"method":        req.Method,
					"trigger":       trigger,
				},
			)
			m.handleError(ctx, currentConn, req, err)
		} else {
			m.logf(LogLevelDebug, "Ignoring notification '%s' (trigger '%s') in state '%s' (id: %s)", req.Method, trigger, currentState, m.connectionID)
		}
		return
	}

	// 3. Find the associated handler logic
	handlerFunc, handlerExists := m.triggerHandlers[trigger]
	if !handlerExists {
		m.logf(LogLevelError, "Internal Error: No handler registered for known trigger %s (method %s) (id: %s)", trigger, req.Method, m.connectionID)
		if !req.Notif {
			err := cgerr.NewInternalError(errors.New("handler not registered for trigger"), map[string]interface{}{"trigger": trigger})
			m.handleError(ctx, currentConn, req, err)
		}
		return
	}

	// 4. Execute the handler
	// Use request's context for handler execution, with timeout
	reqCtx, cancel := context.WithTimeout(ctx, m.config.RequestTimeout)
	defer cancel()
	startTime := time.Now()

	result, handlerErr := handlerFunc(reqCtx, req) // Execute the business logic

	duration := time.Since(startTime)
	m.logf(LogLevelDebug, "Handler execution time for %s: %s (id: %s)", req.Method, duration, m.connectionID)

	// 5. Handle result/error and fire subsequent triggers if needed
	if handlerErr != nil {
		// Check if error should cause a major state change
		if stateChangeTrigger := MapErrorToStateTrigger(handlerErr); stateChangeTrigger != "" {
			// Use FireAsync or rely on FiringQueued to avoid deadlocks if action also uses manager
			_ = m.fsm.FireCtx(fireCtx, stateChangeTrigger)
		}
		// Specifically handle initialization failure trigger
		if trigger == TriggerInitialize {
			_ = m.fsm.FireCtx(fireCtx, TriggerInitFailure)
		}
		m.handleError(ctx, currentConn, req, handlerErr) // Send JSON-RPC error response
		return
	}

	// Fire success triggers where applicable
	var fsmErr error
	if trigger == TriggerInitialize {
		fsmErr = m.fsm.FireCtx(fireCtx, TriggerInitSuccess)
		if fsmErr == nil {
			m.logf(LogLevelInfo, "Initialization successful, connection now active (id: %s)", m.connectionID)
		}
	} else if trigger == TriggerShutdown {
		// Shutdown trigger was checked with CanFire, but actual firing happens
		// either here after success, or via the separate Shutdown() method.
		// Let's assume the handler `handleShutdownRequest` just returns success,
		// and the actual trigger firing should happen via the `Shutdown()` method call.
		// So, no fsm.Fire here for shutdown triggered by RPC message.
	} else {
		// For simple reentry triggers, explicitly fire them if needed for logging/actions
		// Or rely on the fact that CanFire passed. Let's assume reentry is implicit.
		// fsmErr = m.fsm.FireCtx(fireCtx, trigger) // Optional: Fire reentry triggers if actions depend on it
	}

	// Handle FSM errors that might occur during success triggers
	if fsmErr != nil {
		m.logf(LogLevelError, "CRITICAL: State transition failed after successful handler %s: %v (id: %s)", trigger, fsmErr, m.connectionID)
		_ = m.fsm.FireCtx(fireCtx, TriggerErrorOccurred) // Attempt to force error state
		m.handleError(ctx, currentConn, req, errors.Wrapf(fsmErr, "state transition failed post-%s", trigger))
		return
	}

	// 6. Send reply (if not notification)
	if !req.Notif {
		if replyErr := currentConn.Reply(ctx, req.ID, result); replyErr != nil {
			m.logf(LogLevelError, "Failed to send response: %v (id: %s)", replyErr, m.connectionID)
			// Consider if failure to reply should trigger an error state
			// _ = m.fsm.FireCtx(fireCtx, TriggerErrorOccurred)
		}
	}
}

// --- Error Handling ---

// handleError processes an error from a handler and sends an appropriate error response.
// (This can remain mostly the same as your original, potentially adding FSM state)
func (m *ConnectionManager) handleError(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request, err error) {
	// Skip error responses for notifications
	if req.Notif {
		m.logf(LogLevelError, "Error handling notification %s: %v (id: %s)", req.Method, err, m.connectionID)
		return
	}

	// Ensure we have a connection to reply on
	if conn == nil {
		m.logf(LogLevelError, "Cannot send error reply, connection is nil (req: %s, err: %v, id: %s)", req.Method, err, m.connectionID)
		return
	}

	// Determine RPC error code, message, data from the incoming error
	code := jsonrpc2.CodeInternalError
	message := "Internal error"
	var data map[string]interface{} // Initialize data as nil initially

	// Use your custom error details if available
	errorCode := cgerr.GetErrorCode(err) // Assumes this function exists
	if errorCode != 0 {
		code = int64(errorCode)
		message = cgerr.UserFacingMessage(errorCode) // Assumes this exists
		data = cgerr.GetErrorProperties(err)         // Assumes this exists
	} else {
		// Fallback for generic errors
		message = err.Error() // Use raw error message if no specific code
	}

	// Log the detailed error including FSM state
	fsmStateStr := "unknown"
	if m.fsm != nil {
		fsmStateStr = m.fsm.MustStateCtx(m.baseCtx).String()
	}
	m.logf(LogLevelError, "Error handling request %s in state %s: %+v (id: %s, req.ID: %s)", req.Method, fsmStateStr, err, m.connectionID, req.ID)

	// Construct the JSON-RPC error
	rpcErr := &jsonrpc2.Error{
		Code:    code,
		Message: message,
	}
	// Add data only if it's not nil/empty
	if len(data) > 0 {
		// Add connection ID and state if not already present
		if _, ok := data["connection_id"]; !ok {
			data["connection_id"] = m.connectionID
		}
		if _, ok := data["state"]; !ok {
			data["state"] = fsmStateStr
		}
		jsonData, marshalErr := json.Marshal(data)
		if marshalErr == nil {
			rpcErr.Data = (*json.RawMessage)(&jsonData)
		} else {
			m.logf(LogLevelError, "Failed to marshal error data: %v", marshalErr)
			// Send error without data
		}
	} else {
		// Even if no specific data, add connection_id and state for context
		jsonData, _ := json.Marshal(map[string]interface{}{
			"connection_id": m.connectionID,
			"state":         fsmStateStr,
		})
		rpcErr.Data = (*json.RawMessage)(&jsonData)
	}

	// Send the error reply
	if replyErr := conn.ReplyWithError(ctx, req.ID, *rpcErr); replyErr != nil {
		m.logf(LogLevelError, "Failed to send error response: %v (id: %s)", replyErr, m.connectionID)
	}
}

// --- Utility ---

// logf is a helper for logging.
func (m *ConnectionManager) logf(level LogLevel, format string, v ...interface{}) {
	// Add log level filtering if necessary
	// Example: if level < configuredLevel { return }
	m.logger.Printf(format, v...)
}

// generateConnectionID is a placeholder.
func generateConnectionID() string {
	// Consider using a more robust UUID library
	return fmt.Sprintf("conn-%d", time.Now().UnixNano())
}

// --- DELETED METHODS ---
// func (m *ConnectionManager) SetState(...) // No longer needed
// func (m *ConnectionManager) GetState() ConnectionState // No longer needed (use m.fsm.MustState())
// func (m *ConnectionManager) registerDefaultHandlers() // No longer needed (replaced by registerTriggerHandlers)
// func (m *ConnectionManager) validateStateTransition(...) // No longer needed
