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
	dataMu             sync.RWMutex // Protects clientCapabilities and jsonrpcConn
	logger             *log.Logger
	initialized        bool // Tracks if the client has been initialized properly
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
	}

	// State Machine Setup
	m.stateMachine = stateless.NewStateMachine(StateUnconnected)

	// Configure states and transitions
	m.stateMachine.Configure(StateUnconnected).
		OnEntry(m.onEnterUnconnected).
		Permit(TriggerInitialize, StateInitializing)

	m.stateMachine.Configure(StateInitializing).
		OnEntryFrom(TriggerInitialize, m.onEnterInitializing).
		Permit(TriggerInitSuccess, StateConnected).
		Permit(TriggerInitFailure, StateError).
		Permit(TriggerDisconnect, StateTerminating)

	m.stateMachine.Configure(StateConnected).
		OnEntry(m.onEnterConnected).
		PermitReentry(TriggerListResources).
		PermitReentry(TriggerReadResource).
		PermitReentry(TriggerListTools).
		PermitReentry(TriggerCallTool).
		PermitReentry(TriggerPing).
		PermitReentry(TriggerSubscribe).
		Permit(TriggerShutdown, StateTerminating).
		Permit(TriggerDisconnect, StateTerminating).
		Permit(TriggerErrorOccurred, StateError)

	m.stateMachine.Configure(StateTerminating).
		OnEntry(m.onEnterTerminating).
		Permit(TriggerShutdownComplete, StateUnconnected).
		Permit(TriggerDisconnect, StateUnconnected)

	m.stateMachine.Configure(StateError).
		OnEntry(m.onEnterError).
		Permit(TriggerDisconnect, StateUnconnected)

	m.logf(definitions.LogLevelDebug, "Connection manager created")
	return m
}

// onEnterUnconnected is called when entering the Unconnected state.
func (m *Manager) onEnterUnconnected(ctx context.Context, args ...interface{}) error {
	m.logf(definitions.LogLevelDebug, "Connection reset to unconnected state")
	m.initialized = false
	return nil
}

// Handle is the main entry point for incoming JSON-RPC requests.
func (m *Manager) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	// Store the connection for future use
	m.dataMu.Lock()
	m.jsonrpcConn = conn
	m.dataMu.Unlock()

	// Map the method to a trigger
	trigger, ok := MapMethodToTrigger(req.Method)
	if !ok {
		m.handleUnknownMethod(ctx, conn, req)
		return
	}

	currentState := m.stateMachine.MustState().(State)
	m.logf(definitions.LogLevelDebug, "Processing method '%s' (trigger '%s') in state '%s'", req.Method, trigger, currentState)

	// Process based on state and trigger
	if err := m.processStateAndTrigger(ctx, conn, req, trigger, currentState); err != nil {
		m.logf(definitions.LogLevelError, "Error processing request: %v", err)
		// If this isn't a notification, send an error response
		if !req.Notif {
			respErr := cgerr.ToJSONRPCError(err)
			if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
				m.logf(definitions.LogLevelError, "Error sending error response: %v", replyErr)
			}
		}

		// Fire error trigger if appropriate
		m.handleErrorOccurrence(err)
	}
}

// processStateAndTrigger handles the request based on the current state and trigger.
func (m *Manager) processStateAndTrigger(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request,
	trigger Trigger, currentState State) error {

	// Special case: initialize can be processed in unconnected state
	if trigger == TriggerInitialize && currentState == StateUnconnected {
		return m.stateMachine.FireCtx(ctx, string(TriggerInitialize), req)
	}

	// Special case: shutdown in connected state
	if trigger == TriggerShutdown && currentState == StateConnected {
		return m.handleShutdownInConnectedState(ctx, conn, req)
	}

	// Handle normal operations in connected state
	if currentState == StateConnected && !req.Notif {
		return m.handleConnectedStateRequest(ctx, conn, req, trigger)
	}

	// Handle notifications in connected state
	if currentState == StateConnected && req.Notif {
		// Process notification without response
		return m.handleConnectedStateNotification(ctx, req, trigger)
	}

	// If we reach here, the operation is not valid in current state
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

// handleErrorOccurrence handles an error by firing the error trigger if appropriate.
func (m *Manager) handleErrorOccurrence(err error) {
	// Only transition to error state for certain error conditions
	if cgerr.GetErrorCode(err) <= cgerr.CodeInternalError {
		if fireErr := m.stateMachine.Fire(string(TriggerErrorOccurred), err); fireErr != nil {
			m.logf(definitions.LogLevelWarn, "Failed to fire error trigger: %v", fireErr)
		}
	}
}

// handleUnknownMethod handles requests with unknown methods.
func (m *Manager) handleUnknownMethod(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	m.logf(definitions.LogLevelWarn, "Received unknown method: %s", req.Method)

	if req.Notif {
		// Silently ignore unknown notifications per JSON-RPC spec
		return
	}

	// Create method not found error
	err := cgerr.NewMethodNotFoundError(req.Method, map[string]interface{}{
		"connection_id": m.connectionID,
		"request_id":    fmt.Sprintf("%v", req.ID), // Format as string instead of direct conversion
	})

	// Convert to JSON-RPC error and send
	respErr := cgerr.ToJSONRPCError(err)
	if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
		m.logf(definitions.LogLevelError, "Error sending MethodNotFound reply: %v", replyErr)
	}
}

// handleShutdownInConnectedState handles a shutdown request in the Connected state.
func (m *Manager) handleShutdownInConnectedState(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) error {
	// Call the handler to get the result
	result, err := m.handleShutdownRequest(ctx, req)
	if err != nil {
		return err
	}

	// Send success response first
	if !req.Notif {
		if replyErr := conn.Reply(ctx, req.ID, result); replyErr != nil {
			m.logf(definitions.LogLevelError, "Error sending shutdown reply: %v", replyErr)
			return replyErr
		}
	}

	// Then fire trigger (using a separate goroutine to avoid blocking)
	go func() {
		// Short delay to ensure the response is sent
		time.Sleep(100 * time.Millisecond)
		if err := m.stateMachine.Fire(string(TriggerShutdown)); err != nil {
			m.logf(definitions.LogLevelError, "Error firing shutdown trigger: %v", err)
		}
	}()

	return nil
}

// handleConnectedStateRequest handles normal requests in the Connected state.
func (m *Manager) handleConnectedStateRequest(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request,
	trigger Trigger) error {

	var result interface{}
	var handlerErr error

	// We're about to fire a reentry trigger, which means we'll stay in the Connected state
	if fireErr := m.stateMachine.Fire(string(trigger), req); fireErr != nil {
		m.logf(definitions.LogLevelWarn, "Failed to fire trigger %s: %v", trigger, fireErr)
		// Continue processing even if the trigger fails, as we're staying in the same state
	}

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
		handlerErr = cgerr.ErrorWithDetails(
			errors.Newf("no handler implemented for method: %s", req.Method),
			cgerr.CategoryRPC,
			cgerr.CodeMethodNotFound,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"request_id":    fmt.Sprintf("%v", req.ID), // Format as string instead of direct conversion
				"method":        req.Method,
			},
		)
	}

	// Handle errors from the handler
	if handlerErr != nil {
		return handlerErr
	}

	// Send the successful result
	if result != nil && !req.Notif {
		if replyErr := conn.Reply(ctx, req.ID, result); replyErr != nil {
			return cgerr.ErrorWithDetails(
				errors.Wrap(replyErr, "failed to send success response"),
				cgerr.CategoryRPC,
				cgerr.CodeInternalError,
				map[string]interface{}{
					"connection_id": m.connectionID,
					"request_id":    fmt.Sprintf("%v", req.ID), // Format as string instead of direct conversion
					"method":        req.Method,
				},
			)
		}
	}

	return nil
}

// handleConnectedStateNotification handles notifications in the Connected state.
func (m *Manager) handleConnectedStateNotification(ctx context.Context, req *jsonrpc2.Request,
	trigger Trigger) error {

	// Fire the trigger to let state machine know about the notification
	if fireErr := m.stateMachine.Fire(string(trigger), req); fireErr != nil {
		m.logf(definitions.LogLevelWarn, "Failed to fire notification trigger %s: %v", trigger, fireErr)
		// Continue even if trigger fails, as we're ignoring the result for notifications
	}

	// Process the notification but ignore the result
	switch trigger {
	case TriggerListResources:
		_, _ = m.handleListResources(ctx, req)
	case TriggerReadResource:
		_, _ = m.handleReadResource(ctx, req)
	case TriggerListTools:
		_, _ = m.handleListTools(ctx, req)
	case TriggerCallTool:
		_, _ = m.handleCallTool(ctx, req)
	case TriggerPing:
		_, _ = m.handlePing(ctx, req)
	case TriggerSubscribe:
		_, _ = m.handleSubscribe(ctx, req)
	default:
		// Silently ignore unhandled notifications per JSON-RPC spec
		m.logf(definitions.LogLevelDebug, "Ignoring unhandled notification: %s", req.Method)
	}

	return nil
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

	// Get the active connection
	m.dataMu.RLock()
	conn := m.jsonrpcConn
	m.dataMu.RUnlock()
	if conn == nil {
		m.logf(definitions.LogLevelError, "Initialization started but connection is nil")
		return errors.New("connection is nil")
	}

	// Call the specific handler logic
	result, err := m.handleInitialize(ctx, req)

	if err != nil {
		m.logf(definitions.LogLevelError, "Initialization failed: %v", err)

		// Send error response to client
		respErr := cgerr.ToJSONRPCError(err)
		if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
			m.logf(definitions.LogLevelError, "Error sending initialization failure reply: %v", replyErr)
		}

		// Fire failure trigger, which transitions to Error state
		if fireErr := m.stateMachine.Fire(string(TriggerInitFailure), err); fireErr != nil {
			m.logf(definitions.LogLevelError, "Error firing TriggerInitFailure: %v", fireErr)
		}

		return err // Propagate the original error for state machine internals
	}

	// Success case - send response
	if replyErr := conn.Reply(ctx, req.ID, result); replyErr != nil {
		m.logf(definitions.LogLevelError, "Error sending initialization success reply: %v", replyErr)

		// If reply fails, treat as initialization failure
		if fireErr := m.stateMachine.Fire(string(TriggerInitFailure), replyErr); fireErr != nil {
			m.logf(definitions.LogLevelError, "Error firing TriggerInitFailure: %v", fireErr)
		}

		return replyErr
	}

	// Mark as initialized and fire success trigger
	m.initialized = true
	if fireErr := m.stateMachine.Fire(string(TriggerInitSuccess)); fireErr != nil {
		m.logf(definitions.LogLevelError, "Error firing TriggerInitSuccess: %v", fireErr)
		return fireErr
	}

	return nil
}

// onEnterConnected is called when entering the Connected state.
func (m *Manager) onEnterConnected(ctx context.Context, args ...interface{}) error {
	m.logf(definitions.LogLevelInfo, "Connection established and initialized")
	return nil
}

// onEnterTerminating is called when entering the Terminating state.
func (m *Manager) onEnterTerminating(ctx context.Context, args ...interface{}) error {
	m.logf(definitions.LogLevelInfo, "Connection terminating...")

	// Perform orderly cleanup
	m.dataMu.Lock()
	conn := m.jsonrpcConn
	m.jsonrpcConn = nil
	m.dataMu.Unlock()

	// Close connection if it exists
	if conn != nil {
		if err := conn.Close(); err != nil {
			m.logf(definitions.LogLevelWarn, "Error closing connection: %v", err)
		}
	}

	// Signal completion after a short delay to allow any pending messages to be sent
	go func() {
		time.Sleep(100 * time.Millisecond)
		if fireErr := m.stateMachine.Fire(string(TriggerShutdownComplete)); fireErr != nil {
			m.logf(definitions.LogLevelWarn, "Could not fire ShutdownComplete: %v", fireErr)

			// If shutdown complete fails, force disconnect
			if disconnectErr := m.stateMachine.Fire(string(TriggerDisconnect)); disconnectErr != nil {
				m.logf(definitions.LogLevelError, "Failed to disconnect after termination: %v", disconnectErr)
			}
		}
	}()

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

	// After a delay, move back to unconnected state to allow reconnection attempts
	go func() {
		time.Sleep(5 * time.Second)
		if fireErr := m.stateMachine.Fire(string(TriggerDisconnect)); fireErr != nil {
			m.logf(definitions.LogLevelError, "Failed to disconnect after error state: %v", fireErr)
		}
	}()

	return nil
}

// logf is a helper for logging with connection ID prefix.
func (m *Manager) logf(level definitions.LogLevel, format string, v ...interface{}) {
	currentState := m.stateMachine.MustState().(State)
	message := fmt.Sprintf(format, v...)
	m.logger.Printf("[%s] [%s] %s", level, currentState, message)
}

// NewConnectionServer creates a new connection manager styled as a server.
func NewConnectionServer(serverConfig ServerConfig, resourceMgr ResourceManagerContract, toolMgr ToolManagerContract) (*Manager, error) {
	return NewManager(serverConfig, resourceMgr, toolMgr), nil
}
