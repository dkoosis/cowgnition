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
	"github.com/sourcegraph/jsonrpc2" // Still needed for request/response types.
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
	connectionID    string
	config          ServerConfig
	resourceManager ResourceManagerContract
	toolManager     ToolManagerContract
	stateMachine    *stateless.StateMachine
	// Use the RPCConnection interface to allow for different connection implementations (real/mock).
	jsonrpcConn        RPCConnection
	clientCapabilities map[string]interface{}
	dataMu             sync.RWMutex // Protects clientCapabilities and jsonrpcConn.
	logger             *log.Logger
	initialized        bool // Tracks if the client has been initialized properly.
}

// NewManager creates and initializes a new Manager.
func NewManager(
	config ServerConfig,
	resourceMgr ResourceManagerContract,
	toolMgr ToolManagerContract,
) *Manager {
	connID := uuid.NewString()
	logger := log.New(log.Default().Writer(), fmt.Sprintf("CONN [%s] ", connID), log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	m := &Manager{
		connectionID:       connID,
		config:             config,
		resourceManager:    resourceMgr,
		toolManager:        toolMgr,
		logger:             logger,
		clientCapabilities: make(map[string]interface{}),
		// jsonrpcConn is initially nil.
	}

	// State Machine Setup.
	m.stateMachine = stateless.NewStateMachine(StateUnconnected)

	m.stateMachine.Configure(StateUnconnected).
		OnEntry(m.onEnterUnconnected).
		Permit(TriggerInitialize, StateInitializing)

	m.stateMachine.Configure(StateInitializing).
		OnEntryFrom(TriggerInitialize, m.onEnterInitializing). // Pass req via args.
		Permit(TriggerInitSuccess, StateConnected).
		Permit(TriggerInitFailure, StateError).
		Permit(TriggerDisconnect, StateTerminating) // Handle disconnect during init.

	m.stateMachine.Configure(StateConnected).
		OnEntry(m.onEnterConnected).
		PermitReentry(TriggerListResources). // Pass req via args.
		PermitReentry(TriggerReadResource).  // Pass req via args.
		PermitReentry(TriggerListTools).     // Pass req via args.
		PermitReentry(TriggerCallTool).      // Pass req via args.
		PermitReentry(TriggerPing).          // Pass req via args.
		PermitReentry(TriggerSubscribe).     // Pass req via args.
		Permit(TriggerShutdown, StateTerminating).
		Permit(TriggerDisconnect, StateTerminating).
		Permit(TriggerErrorOccurred, StateError) // Pass error via args.

	m.stateMachine.Configure(StateTerminating).
		OnEntry(m.onEnterTerminating).
		Permit(TriggerShutdownComplete, StateUnconnected).
		Permit(TriggerDisconnect, StateUnconnected) // Allow direct disconnect.

	m.stateMachine.Configure(StateError).
		OnEntry(m.onEnterError). // Pass error via args.
		Permit(TriggerDisconnect, StateUnconnected)

	m.logf(definitions.LogLevelDebug, "Connection manager created.")
	return m
}

// onEnterUnconnected is called when entering the Unconnected state.
func (m *Manager) onEnterUnconnected(ctx context.Context, args ...interface{}) error {
	m.logf(definitions.LogLevelDebug, "Connection reset to unconnected state.")
	m.initialized = false
	// Clear connection reference on disconnect.
	m.dataMu.Lock()
	m.jsonrpcConn = nil
	m.dataMu.Unlock()
	return nil
}

// Handle is the main entry point for incoming JSON-RPC requests.
// It accepts any connection type that satisfies the RPCConnection interface.
func (m *Manager) Handle(ctx context.Context, conn RPCConnection, req *jsonrpc2.Request) {
	// Store the connection for future use within this manager instance.
	m.dataMu.Lock()
	if m.jsonrpcConn == nil {
		m.jsonrpcConn = conn
	} else {
		// If Handle is called again with a new connection instance, log a warning.
		// Behavior depends on whether Manager instances are reused.
		// Assuming one connection per manager lifecycle for now.
		if m.jsonrpcConn != conn {
			m.logf(definitions.LogLevelWarn, "Handle called with a new connection instance while one already exists.")
			// Optionally update to the new connection: m.jsonrpcConn = conn
		}
	}
	m.dataMu.Unlock()

	// Map the method to a state machine trigger.
	trigger, ok := MapMethodToTrigger(req.Method)
	if !ok {
		m.handleUnknownMethod(ctx, conn, req) // Pass the interface.
		return
	}

	currentState := m.stateMachine.MustState().(State)
	m.logf(definitions.LogLevelDebug, "Processing method '%s' (trigger '%s') in state '%s'.", req.Method, trigger, currentState)

	// Process the request based on the current state and determined trigger.
	if err := m.processStateAndTrigger(ctx, conn, req, trigger, currentState); err != nil { // Pass the interface.
		m.logf(definitions.LogLevelError, "Error processing request: %v.", err)
		// If this isn't a notification, send an error response using the interface method.
		if !req.Notif {
			respErr := cgerr.ToJSONRPCError(err)
			if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
				m.logf(definitions.LogLevelError, "Error sending error response: %v.", replyErr)
			}
		}

		// Fire error trigger if the error warrants a state change.
		m.handleErrorOccurrence(err) // This internal method doesn't need conn.
	}
}

// processStateAndTrigger routes the request based on the current state and trigger.
// It accepts the connection as an RPCConnection interface.
func (m *Manager) processStateAndTrigger(ctx context.Context, conn RPCConnection, req *jsonrpc2.Request,
	trigger Trigger, currentState State) error {
	// Special case: initialize is allowed only from unconnected state.
	if trigger == TriggerInitialize && currentState == StateUnconnected {
		// Pass request to the state machine's OnEntryFrom handler via FireCtx args.
		return m.stateMachine.FireCtx(ctx, string(TriggerInitialize), req)
	}

	// Special case: shutdown is handled differently in connected state.
	if trigger == TriggerShutdown && currentState == StateConnected {
		return m.handleShutdownInConnectedState(ctx, conn, req) // Pass the interface.
	}

	// Handle normal request/response methods in connected state.
	if currentState == StateConnected && !req.Notif {
		return m.handleConnectedStateRequest(ctx, conn, req, trigger) // Pass the interface.
	}

	// Handle notifications (no response expected) in connected state.
	if currentState == StateConnected && req.Notif {
		// Note: handleConnectedStateNotification does not use conn to reply.
		return m.handleConnectedStateNotification(ctx, req, trigger)
	}

	// If none of the above conditions match, the operation is not allowed in the current state.
	return cgerr.ErrorWithDetails(
		errors.Newf("operation '%s' not allowed in state '%s'", req.Method, currentState),
		cgerr.CategoryRPC,
		cgerr.CodeInvalidRequest,
		map[string]interface{}{
			"connection_id": m.connectionID,
			"current_state": string(currentState),
			"method":        req.Method,
			"trigger":       string(trigger),
		},
	)
}

// handleErrorOccurrence potentially transitions the state machine to the Error state.
// This does not directly interact with the connection.
func (m *Manager) handleErrorOccurrence(err error) {
	// Only transition to error state for certain severe error codes.
	if cgerr.GetErrorCode(err) <= cgerr.CodeInternalError {
		// Pass the error details to the Error state's OnEntry handler.
		if fireErr := m.stateMachine.Fire(string(TriggerErrorOccurred), err); fireErr != nil {
			m.logf(definitions.LogLevelWarn, "Failed to fire error trigger: %v.", fireErr)
		}
	}
}

// handleUnknownMethod handles requests with methods not mapped to triggers.
// It accepts the connection as an RPCConnection interface to send replies.
func (m *Manager) handleUnknownMethod(ctx context.Context, conn RPCConnection, req *jsonrpc2.Request) {
	m.logf(definitions.LogLevelWarn, "Received unknown method: %s.", req.Method)

	// Silently ignore unknown notifications per JSON-RPC spec.
	if req.Notif {
		return
	}

	// Create a "MethodNotFound" error.
	err := cgerr.NewMethodNotFoundError(req.Method, map[string]interface{}{
		"connection_id": m.connectionID,
		"request_id":    fmt.Sprintf("%v", req.ID),
	})

	// Convert to JSON-RPC error format and send reply using the interface method.
	respErr := cgerr.ToJSONRPCError(err)
	if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
		m.logf(definitions.LogLevelError, "Error sending MethodNotFound reply: %v.", replyErr)
	}
}

// handleShutdownInConnectedState handles the shutdown request specifically.
// It accepts the connection as an RPCConnection interface to send the reply before transitioning state.
func (m *Manager) handleShutdownInConnectedState(ctx context.Context, conn RPCConnection, req *jsonrpc2.Request) error {
	// Call the internal handler logic for shutdown.
	result, err := m.handleShutdownRequest(ctx, req) // This handler likely doesn't need conn.
	if err != nil {
		// If the handler itself errors, report it and potentially enter error state.
		m.logf(definitions.LogLevelError, "Shutdown handler failed: %v.", err)
		m.handleErrorOccurrence(err)
		// Send error reply if it's not a notification.
		if !req.Notif {
			respErr := cgerr.ToJSONRPCError(err)
			if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
				m.logf(definitions.LogLevelError, "Error sending shutdown handler error reply: %v.", replyErr)
			}
		}
		return err // Return the error from the handler.
	}

	// Send success response first if required.
	if !req.Notif {
		// Use the interface method to send the reply.
		if replyErr := conn.Reply(ctx, req.ID, result); replyErr != nil {
			m.logf(definitions.LogLevelError, "Error sending shutdown success reply: %v.", replyErr)
			// Treat failure to send reply as an error condition.
			m.handleErrorOccurrence(errors.Wrap(replyErr, "failed sending shutdown reply"))
			// Continue to fire shutdown trigger despite reply error.
		}
	}

	// Fire the state machine trigger asynchronously to allow the reply to be sent.
	go func() {
		time.Sleep(50 * time.Millisecond) // Short delay.
		if fireErr := m.stateMachine.Fire(string(TriggerShutdown)); fireErr != nil {
			m.logf(definitions.LogLevelError, "Error firing shutdown trigger post-reply: %v.", fireErr)
			// If firing fails (e.g., wrong state), force disconnect as fallback.
			_ = m.stateMachine.Fire(string(TriggerDisconnect))
		}
	}()

	return nil // Indicates the handler logic (handleShutdownRequest) succeeded.
}

// handleConnectedStateRequest handles standard request/response calls in the Connected state.
// It accepts the connection as an RPCConnection interface to send replies.
func (m *Manager) handleConnectedStateRequest(ctx context.Context, conn RPCConnection, req *jsonrpc2.Request,
	trigger Trigger) error {
	var result interface{}
	var handlerErr error

	// Fire the reentry trigger associated with the method.
	// Passing the request might be useful for OnEntry/OnExit logging within the state machine config.
	if fireErr := m.stateMachine.Fire(string(trigger), req); fireErr != nil {
		m.logf(definitions.LogLevelWarn, "Failed to fire reentry trigger %s: %v.", trigger, fireErr)
		// Decide if failing to fire a reentry trigger is fatal; currently, we proceed.
	}

	// Call the specific internal handler based on the trigger.
	// These handlers likely don't need the connection object itself.
	switch trigger {
	case TriggerListResources:
		result, handlerErr = m.handleListResources(ctx, req)
	case TriggerReadResource:
		result, handlerErr = m.handleReadResource(ctx, req)
	case TriggerListTools:
		result, handlerErr = m.handleListTools(ctx, req)
	case TriggerCallTool:
		result, handlerErr = m.handleCallTool(ctx, req)
	case TriggerPing:
		result, handlerErr = m.handlePing(ctx, req)
	case TriggerSubscribe:
		result, handlerErr = m.handleSubscribe(ctx, req)
	default:
		// This case should ideally not be reached if MapMethodToTrigger is comprehensive.
		handlerErr = cgerr.NewMethodNotFoundError(req.Method, map[string]interface{}{"state": "connected"})
	}

	// Handle errors returned from the specific handler.
	if handlerErr != nil {
		m.logf(definitions.LogLevelError, "Handler for %s failed: %v.", req.Method, handlerErr)
		// Send an error reply using the interface method.
		if !req.Notif {
			respErr := cgerr.ToJSONRPCError(handlerErr)
			if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
				m.logf(definitions.LogLevelError, "Error sending handler error reply for %s: %v.", req.Method, replyErr)
			}
		}
		// Potentially transition to Error state based on the handler error severity.
		m.handleErrorOccurrence(handlerErr)
		return handlerErr // Propagate the handler error.
	}

	// Send the successful result if it's not a notification.
	if !req.Notif {
		// Use the interface method to send the reply.
		if replyErr := conn.Reply(ctx, req.ID, result); replyErr != nil {
			wrappedErr := cgerr.ErrorWithDetails(
				errors.Wrapf(replyErr, "failed to send success response for %s", req.Method),
				cgerr.CategoryRPC,
				cgerr.CodeInternalError,
				map[string]interface{}{"method": req.Method},
			)
			m.logf(definitions.LogLevelError, "%v.", wrappedErr)
			// Trigger error state if sending the reply fails.
			m.handleErrorOccurrence(wrappedErr)
			return wrappedErr // Return error if reply fails.
		}
	}

	return nil // Success.
}

// handleConnectedStateNotification handles notifications received in the Connected state.
// It does not use the connection object to send replies.
func (m *Manager) handleConnectedStateNotification(ctx context.Context, req *jsonrpc2.Request,
	trigger Trigger) error {
	// Fire the trigger to allow state machine logging or actions if needed.
	if fireErr := m.stateMachine.Fire(string(trigger), req); fireErr != nil {
		m.logf(definitions.LogLevelWarn, "Failed to fire notification trigger %s: %v.", trigger, fireErr)
	}

	// Process the notification by calling the relevant handler.
	// Ignore the result and errors per JSON-RPC spec for notifications.
	var handlerErr error
	switch trigger {
	case TriggerListResources:
		_, handlerErr = m.handleListResources(ctx, req)
	case TriggerReadResource:
		_, handlerErr = m.handleReadResource(ctx, req)
	case TriggerListTools:
		_, handlerErr = m.handleListTools(ctx, req)
	case TriggerCallTool:
		_, handlerErr = m.handleCallTool(ctx, req)
	case TriggerPing:
		_, handlerErr = m.handlePing(ctx, req)
	case TriggerSubscribe:
		_, handlerErr = m.handleSubscribe(ctx, req)
	default:
		m.logf(definitions.LogLevelDebug, "Ignoring unhandled notification: %s.", req.Method)
	}

	// Log errors from notification handlers but take no further action.
	if handlerErr != nil {
		m.logf(definitions.LogLevelWarn, "Error processing notification %s: %v.", req.Method, handlerErr)
	}

	return nil // Always return nil for notifications.
}

// onEnterInitializing is called when entering the Initializing state via the TriggerInitialize trigger.
// It retrieves the connection via the interface stored in the manager.
func (m *Manager) onEnterInitializing(ctx context.Context, args ...interface{}) error {
	if len(args) == 0 {
		return errors.New("missing request argument for onEnterInitializing")
	}
	req, ok := args[0].(*jsonrpc2.Request)
	if !ok {
		return errors.New("invalid request argument type for onEnterInitializing")
	}

	// Get the active connection (interface type) stored during Handle().
	m.dataMu.RLock()
	conn := m.jsonrpcConn
	m.dataMu.RUnlock()

	// Check if the connection is somehow nil (e.g., race condition or logic error).
	if conn == nil {
		err := errors.New("connection is nil during initialization trigger")
		m.logf(definitions.LogLevelError, "%v.", err)
		// Fire failure immediately if no connection is stored.
		// Use background context if original ctx might be cancelled.
		_ = m.stateMachine.FireCtx(context.Background(), string(TriggerInitFailure), err)
		return err
	}

	// Call the specific handler logic for initialization.
	result, err := m.handleInitialize(ctx, req) // Handler likely doesn't need conn.

	if err != nil {
		// Initialization handler failed.
		m.logf(definitions.LogLevelError, "Initialization handler failed: %v.", err)
		respErr := cgerr.ToJSONRPCError(err)
		// Send error reply using the interface method.
		if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
			m.logf(definitions.LogLevelError, "Error sending initialization failure reply: %v.", replyErr)
		}
		// Fire failure trigger, passing the original handler error.
		// Use background context if original ctx might be cancelled.
		if fireErr := m.stateMachine.FireCtx(context.Background(), string(TriggerInitFailure), err); fireErr != nil {
			m.logf(definitions.LogLevelError, "Error firing TriggerInitFailure: %v.", fireErr)
		}
		return err // Return the original handler error.
	}

	// Initialization handler succeeded, send success reply.
	// Use the interface method to send the reply.
	if replyErr := conn.Reply(ctx, req.ID, result); replyErr != nil {
		m.logf(definitions.LogLevelError, "Error sending initialization success reply: %v.", replyErr)
		// If reply fails, treat this as an overall initialization failure.
		// Fire failure trigger, passing the reply error.
		// Use background context if original ctx might be cancelled.
		if fireErr := m.stateMachine.FireCtx(context.Background(), string(TriggerInitFailure), replyErr); fireErr != nil {
			m.logf(definitions.LogLevelError, "Error firing TriggerInitFailure after reply error: %v.", fireErr)
		}
		return replyErr // Return the reply error.
	}

	// Mark manager as initialized and fire success trigger.
	m.initialized = true
	if fireErr := m.stateMachine.FireCtx(ctx, string(TriggerInitSuccess)); fireErr != nil {
		m.logf(definitions.LogLevelError, "Error firing TriggerInitSuccess: %v.", fireErr)
		return fireErr // Return the trigger fire error.
	}

	return nil // Initialization successful.
}

// onEnterConnected is called when entering the Connected state.
func (m *Manager) onEnterConnected(ctx context.Context, args ...interface{}) error {
	m.logf(definitions.LogLevelInfo, "Connection established and initialized.")
	return nil
}

// onEnterTerminating is called when entering the Terminating state.
// It retrieves the connection via the interface and calls Close().
func (m *Manager) onEnterTerminating(ctx context.Context, args ...interface{}) error {
	m.logf(definitions.LogLevelInfo, "Connection terminating...")

	// Get the connection interface and clear the field under lock.
	m.dataMu.Lock()
	conn := m.jsonrpcConn
	m.jsonrpcConn = nil // Clear the reference immediately.
	m.dataMu.Unlock()

	// Close the connection if it exists, using the interface method.
	if conn != nil {
		if err := conn.Close(); err != nil {
			m.logf(definitions.LogLevelWarn, "Error closing connection during termination: %v.", err)
			// Continue shutdown process even if close fails.
		}
	} else {
		m.logf(definitions.LogLevelWarn, "Termination started but connection was already nil.")
	}

	// Asynchronously fire the completion trigger to allow state machine to settle.
	go func() {
		time.Sleep(50 * time.Millisecond) // Allow time for Close() potentially.
		// Try ShutdownComplete first.
		if fireErr := m.stateMachine.Fire(string(TriggerShutdownComplete)); fireErr != nil {
			m.logf(definitions.LogLevelWarn, "Could not fire ShutdownComplete (%v), forcing disconnect.", fireErr)
			// Force disconnect if ShutdownComplete fails (e.g., wrong state).
			if disconnectErr := m.stateMachine.Fire(string(TriggerDisconnect)); disconnectErr != nil {
				m.logf(definitions.LogLevelError, "Failed to force disconnect after termination: %v.", disconnectErr)
			}
		}
	}()

	return nil
}

// onEnterError is called when entering the Error state.
// It attempts to close the connection and schedules a transition back to Unconnected.
func (m *Manager) onEnterError(ctx context.Context, args ...interface{}) error {
	errMsg := "Unknown internal error."
	var originalErr error
	if len(args) > 0 {
		if err, ok := args[0].(error); ok {
			originalErr = err
			errMsg = err.Error()
		} else {
			errMsg = fmt.Sprintf("%+v", args[0])
		}
	}
	m.logf(definitions.LogLevelError, "Connection entered error state: %s.", errMsg)

	// Attempt to close the connection immediately when an error occurs.
	m.dataMu.RLock()
	conn := m.jsonrpcConn
	m.dataMu.RUnlock()

	if conn != nil {
		// Use the interface method to close.
		if closeErr := conn.Close(); closeErr != nil {
			m.logf(definitions.LogLevelWarn, "Error closing connection during error state entry: %v.", closeErr)
		}
	} else {
		m.logf(definitions.LogLevelWarn, "Error state entered but connection was already nil.")
	}

	// After a delay, trigger disconnect to move to Unconnected, allowing cleanup/reconnection.
	go func() {
		time.Sleep(1 * time.Second) // Reduced delay from 5s.
		// Pass original error if available for context on disconnect trigger.
		if fireErr := m.stateMachine.Fire(string(TriggerDisconnect), originalErr); fireErr != nil {
			m.logf(definitions.LogLevelError, "Failed to auto-disconnect after error state timeout: %v.", fireErr)
		}
	}()

	// Returning an error from OnEntry can halt state machine processing, so return nil.
	return nil
}

// logf is a helper for logging with connection ID and state prefix.
func (m *Manager) logf(level definitions.LogLevel, format string, v ...interface{}) {
	// Check state machine is initialized before trying to get state.
	var currentState State = "UNKNOWN" // Default if state machine not ready or state retrieval fails.
	if m.stateMachine != nil {
		s, err := m.stateMachine.State(context.Background())
		if err == nil {
			if stateTyped, ok := s.(State); ok {
				currentState = stateTyped
			}
		}
	}
	message := fmt.Sprintf(format, v...)
	// Format: LEVEL [CONN_ID] [STATE] Message
	m.logger.Printf("%-5s [%s] [%s] %s", level, m.connectionID, currentState, message) // Added level alignment.
}

// NewConnectionServer provides a simple factory function if needed elsewhere.
func NewConnectionServer(serverConfig ServerConfig, resourceMgr ResourceManagerContract, toolMgr ToolManagerContract) (*Manager, error) {
	// Could add validation for config/managers here if necessary.
	return NewManager(serverConfig, resourceMgr, toolMgr), nil
}

// Ensure implementation details like State, Trigger types, MapMethodToTrigger,
// and specific handlers (handleInitialize, handleListResources etc.) are correctly
// defined elsewhere in the package or imported.
