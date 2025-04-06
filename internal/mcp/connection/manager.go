// file: internal/mcp/connection/manager.go
// Package connection manages the state and communication for individual MCP client connections.
// Terminate all comments with a period.
package connection

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	// Use the corrected definitions package.
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
	// This should now ideally use definitions.ServerCapabilities struct.
	// Using map for flexibility as passed from adapter, but ensure content matches spec.
	Capabilities map[string]interface{}
}

// Manager orchestrates the state and communication for a single client connection.
type Manager struct {
	connectionID string
	config       ServerConfig
	// Use the contracts defined in connection_types.go, which MUST match mcp interfaces.
	resourceManager    ResourceManagerContract
	toolManager        ToolManagerContract
	stateMachine       *stateless.StateMachine
	jsonrpcConn        RPCConnection                  // Use the RPCConnection interface.
	clientCapabilities definitions.ClientCapabilities // Store parsed client capabilities struct.
	dataMu             sync.RWMutex                   // Protects clientCapabilities and jsonrpcConn.
	logger             *log.Logger
	initialized        bool // Tracks if the client has been initialized properly.
}

// NewManager creates and initializes a new Manager.
func NewManager(
	config ServerConfig,
	resourceMgr ResourceManagerContract, // Use contract interface.
	toolMgr ToolManagerContract, // Use contract interface.
) *Manager {
	connID := uuid.NewString()
	// TODO: Consider injecting a structured logger (like slog) instead of stdlib log.
	logger := log.New(log.Default().Writer(), fmt.Sprintf("CONN [%s] ", connID), log.LstdFlags|log.Lmicroseconds|log.Lshortfile)

	m := &Manager{
		connectionID:    connID,
		config:          config,
		resourceManager: resourceMgr,
		toolManager:     toolMgr,
		logger:          logger,
		// clientCapabilities initialized in handleInitialize.
		// jsonrpcConn is initially nil.
	}

	// State Machine Setup (Assuming State/Trigger types are defined correctly, e.g., in connection_types.go).
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
		PermitReentry(TriggerListResources). // Args: req *jsonrpc2.Request.
		PermitReentry(TriggerReadResource).  // Args: req *jsonrpc2.Request.
		PermitReentry(TriggerListTools).     // Args: req *jsonrpc2.Request.
		PermitReentry(TriggerCallTool).      // Args: req *jsonrpc2.Request.
		PermitReentry(TriggerPing).          // Args: req *jsonrpc2.Request.
		PermitReentry(TriggerSubscribe).     // Args: req *jsonrpc2.Request.
		Permit(TriggerShutdown, StateTerminating).
		Permit(TriggerDisconnect, StateTerminating).
		Permit(TriggerErrorOccurred, StateError) // Args: err error.

	m.stateMachine.Configure(StateTerminating).
		OnEntry(m.onEnterTerminating).
		Permit(TriggerShutdownComplete, StateUnconnected).
		Permit(TriggerDisconnect, StateUnconnected) // Allow direct disconnect.

	m.stateMachine.Configure(StateError).
		OnEntry(m.onEnterError). // Args: err error.
		Permit(TriggerDisconnect, StateUnconnected)

	m.logf(definitions.LogLevelDebug, "Connection manager created.") // Use definitions constant.
	return m
}

// onEnterUnconnected is called when entering the Unconnected state.
func (m *Manager) onEnterUnconnected(ctx context.Context, args ...interface{}) error {
	m.logf(definitions.LogLevelDebug, "Connection reset to unconnected state.")
	m.initialized = false
	// Clear connection reference on disconnect.
	m.dataMu.Lock()
	m.jsonrpcConn = nil
	// Clear client capabilities on disconnect.
	m.clientCapabilities = definitions.ClientCapabilities{} // Re-initialize or set to nil.
	m.dataMu.Unlock()
	return nil
}

// Handle is the main entry point for incoming JSON-RPC requests.
func (m *Manager) Handle(ctx context.Context, conn RPCConnection, req *jsonrpc2.Request) {
	// Store/verify the connection.
	m.dataMu.Lock()
	if m.jsonrpcConn == nil {
		m.jsonrpcConn = conn
	} else if m.jsonrpcConn != conn {
		m.logf(definitions.LogLevelWarning, "Handle called with a new connection instance while one already exists.") // Fixed LogLevel constant.
	}
	m.dataMu.Unlock()

	// Map method to trigger.
	trigger, ok := MapMethodToTrigger(req.Method)
	if !ok {
		m.handleUnknownMethod(ctx, conn, req)
		return
	}

	currentState := m.stateMachine.MustState().(State)
	m.logf(definitions.LogLevelDebug, "Processing method '%s' (trigger '%s') in state '%s'.", req.Method, trigger, currentState)

	// Process based on state and trigger.
	if err := m.processStateAndTrigger(ctx, conn, req, trigger, currentState); err != nil {
		m.logf(definitions.LogLevelError, "Error processing request: %+v.", err) // Use %+v for detailed error.
		if !req.Notif {
			respErr := cgerr.ToJSONRPCError(err)
			if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
				m.logf(definitions.LogLevelError, "Error sending error response: %+v.", replyErr)
			}
		}
		// Consider if all processing errors should trigger StateError.
		// The handlers might already do this via handleErrorOccurrence.
	}
}

// processStateAndTrigger routes the request.
func (m *Manager) processStateAndTrigger(ctx context.Context, conn RPCConnection, req *jsonrpc2.Request,
	trigger Trigger, currentState State) error {
	// Initialize only allowed from unconnected.
	if trigger == TriggerInitialize && currentState == StateUnconnected {
		// Pass request via FireCtx args for OnEntryFrom handler.
		return m.stateMachine.FireCtx(ctx, trigger, req)
	}

	// Allow ping regardless of initialization state (useful for diagnostics).
	// But requires a connection.
	if trigger == TriggerPing && (currentState == StateConnected || currentState == StateInitializing) {
		// Ping is simple request/response, handle directly or via connected handler.
		return m.handleConnectedStateRequest(ctx, conn, req, trigger)
	}

	// Check if initialized before allowing other operations.
	if !m.initialized && trigger != TriggerInitialize && trigger != TriggerPing {
		return cgerr.ErrorWithDetails(
			errors.Newf("operation '%s' not allowed before initialization", req.Method),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidRequest,
			map[string]interface{}{
				"connection_id": m.connectionID,
				"current_state": string(currentState),
				"method":        req.Method,
			},
		)
	}

	// Handle shutdown trigger.
	if trigger == TriggerShutdown && currentState == StateConnected {
		return m.handleShutdownInConnectedState(ctx, conn, req)
	}

	// Handle standard request/response methods in connected state.
	if currentState == StateConnected && !req.Notif {
		return m.handleConnectedStateRequest(ctx, conn, req, trigger)
	}

	// Handle notifications in connected state.
	if currentState == StateConnected && req.Notif {
		return m.handleConnectedStateNotification(ctx, req, trigger)
	}

	// Fallback: operation not allowed in current state.
	return cgerr.ErrorWithDetails(
		errors.Newf("operation '%s' not allowed in state '%s'", req.Method, currentState),
		cgerr.CategoryRPC,
		cgerr.CodeInvalidRequest,
		map[string]interface{}{
			"connection_id": m.connectionID,
			"current_state": string(currentState),
			"method":        req.Method,
		},
	)
}

// handleErrorOccurrence potentially transitions the state machine to the Error state.
func (m *Manager) handleErrorOccurrence(err error) {
	// Only transition for severe errors. Customize this logic as needed.
	// Consider using errors.Is to check for specific error types if necessary.
	if cgerr.GetErrorCode(err) <= cgerr.CodeInternalError || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		if fireErr := m.stateMachine.Fire(TriggerErrorOccurred, err); fireErr != nil {
			// Use LogLevelWarning as per original code intent. Corrected constant name.
			m.logf(definitions.LogLevelWarning, "Failed to fire error trigger: %v.", fireErr)
		}
	}
}

// handleUnknownMethod handles requests with methods not mapped to triggers.
func (m *Manager) handleUnknownMethod(ctx context.Context, conn RPCConnection, req *jsonrpc2.Request) {
	m.logf(definitions.LogLevelWarning, "Received unknown method: %s.", req.Method) // Corrected LogLevel constant.
	if req.Notif {
		return // Ignore unknown notifications.
	}
	err := cgerr.NewMethodNotFoundError(req.Method, map[string]interface{}{"connection_id": m.connectionID})
	respErr := cgerr.ToJSONRPCError(err)
	if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
		m.logf(definitions.LogLevelError, "Error sending MethodNotFound reply: %+v.", replyErr)
	}
}

// handleShutdownInConnectedState handles the shutdown request.
func (m *Manager) handleShutdownInConnectedState(ctx context.Context, conn RPCConnection, req *jsonrpc2.Request) error {
	// Call internal handler (doesn't need conn).
	result, err := m.handleShutdownRequest(ctx, req)
	if err != nil {
		m.logf(definitions.LogLevelError, "Shutdown handler failed: %+v.", err)
		m.handleErrorOccurrence(err)
		if !req.Notif {
			respErr := cgerr.ToJSONRPCError(err)
			if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
				m.logf(definitions.LogLevelError, "Error sending shutdown handler error reply: %+v.", replyErr)
			}
		}
		return err
	}

	// Send success reply first if needed.
	if !req.Notif {
		if replyErr := conn.Reply(ctx, req.ID, result); replyErr != nil {
			m.logf(definitions.LogLevelError, "Error sending shutdown success reply: %+v.", replyErr)
			m.handleErrorOccurrence(errors.Wrap(replyErr, "failed sending shutdown reply"))
			// Continue shutdown even if reply fails.
		}
	}

	// Fire state change async.
	go func() {
		time.Sleep(50 * time.Millisecond)
		if fireErr := m.stateMachine.Fire(TriggerShutdown); fireErr != nil {
			m.logf(definitions.LogLevelError, "Error firing shutdown trigger post-reply: %v.", fireErr)
			_ = m.stateMachine.Fire(TriggerDisconnect) // Force disconnect on trigger fail.
		}
	}()
	return nil // Handler succeeded.
}

// onEnterTerminating closes the connection.
func (m *Manager) onEnterTerminating(ctx context.Context, args ...interface{}) error {
	m.logf(definitions.LogLevelInfo, "Connection terminating...")
	m.dataMu.Lock()
	conn := m.jsonrpcConn
	m.jsonrpcConn = nil
	m.dataMu.Unlock()

	if conn != nil {
		if err := conn.Close(); err != nil {
			m.logf(definitions.LogLevelWarning, "Error closing connection during termination: %+v.", err) // Corrected LogLevel, use %+v.
		}
	} else {
		m.logf(definitions.LogLevelWarning, "Termination started but connection was already nil.") // Corrected LogLevel.
	}

	// Async trigger completion.
	go func() {
		time.Sleep(50 * time.Millisecond)
		if fireErr := m.stateMachine.Fire(TriggerShutdownComplete); fireErr != nil {
			m.logf(definitions.LogLevelWarning, "Could not fire ShutdownComplete (%+v), forcing disconnect.", fireErr) // Corrected LogLevel, use %+v.
			if disconnectErr := m.stateMachine.Fire(TriggerDisconnect); disconnectErr != nil {
				m.logf(definitions.LogLevelError, "Failed to force disconnect after termination: %+v.", disconnectErr)
			}
		}
	}()
	return nil
}

// onEnterError logs error and attempts disconnect.
func (m *Manager) onEnterError(ctx context.Context, args ...interface{}) error {
	errMsg := "Unknown internal error."
	var originalErr error
	if len(args) > 0 {
		if err, ok := args[0].(error); ok {
			originalErr = err
			errMsg = fmt.Sprintf("%+v", err) // Use %+v for detailed error logging.
		} else {
			errMsg = fmt.Sprintf("%+v", args[0])
		}
	}
	m.logf(definitions.LogLevelError, "Connection entered error state: %s.", errMsg)

	// Close connection immediately on error.
	m.dataMu.RLock()
	conn := m.jsonrpcConn
	m.dataMu.RUnlock()
	if conn != nil {
		if closeErr := conn.Close(); closeErr != nil {
			m.logf(definitions.LogLevelWarning, "Error closing connection during error state entry: %+v.", closeErr) // Corrected LogLevel, use %+v.
		}
	} else {
		m.logf(definitions.LogLevelWarning, "Error state entered but connection was already nil.") // Corrected LogLevel.
	}

	// Schedule disconnect.
	go func() {
		time.Sleep(1 * time.Second)
		if fireErr := m.stateMachine.Fire(TriggerDisconnect, originalErr); fireErr != nil {
			m.logf(definitions.LogLevelError, "Failed to auto-disconnect after error state timeout: %+v.", fireErr)
		}
	}()
	return nil
}

// onEnterInitializing handles the initialization logic.
func (m *Manager) onEnterInitializing(ctx context.Context, args ...interface{}) error {
	if len(args) == 0 {
		return errors.New("missing request argument for onEnterInitializing.")
	}
	req, ok := args[0].(*jsonrpc2.Request)
	if !ok {
		return errors.New("invalid request argument type for onEnterInitializing.")
	}

	m.dataMu.RLock()
	conn := m.jsonrpcConn
	m.dataMu.RUnlock()

	if conn == nil {
		err := errors.New("connection is nil during initialization trigger.")
		m.logf(definitions.LogLevelError, "%+v.", err)
		_ = m.stateMachine.FireCtx(context.Background(), TriggerInitFailure, err)
		return err
	}

	// Call the specific handler logic for initialization.
	// handleInitialize returns interface{} which should be definitions.InitializeResult.
	result, err := m.handleInitialize(ctx, req)

	if err != nil {
		// Initialization handler failed.
		m.logf(definitions.LogLevelError, "Initialization handler failed: %+v.", err)
		respErr := cgerr.ToJSONRPCError(err)
		if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
			m.logf(definitions.LogLevelError, "Error sending initialization failure reply: %+v.", replyErr)
		}
		// Fire failure trigger.
		if fireErr := m.stateMachine.FireCtx(context.Background(), TriggerInitFailure, err); fireErr != nil {
			m.logf(definitions.LogLevelError, "Error firing TriggerInitFailure: %+v.", fireErr)
		}
		return err
	}

	// Initialization handler succeeded, send success reply.
	if replyErr := conn.Reply(ctx, req.ID, result); replyErr != nil {
		m.logf(definitions.LogLevelError, "Error sending initialization success reply: %+v.", replyErr)
		// Treat reply failure as init failure.
		if fireErr := m.stateMachine.FireCtx(context.Background(), TriggerInitFailure, replyErr); fireErr != nil {
			m.logf(definitions.LogLevelError, "Error firing TriggerInitFailure after reply error: %+v.", fireErr)
		}
		return replyErr
	}

	// Mark as initialized and fire success trigger.
	m.initialized = true // Set initialized flag only after successful reply.
	if fireErr := m.stateMachine.FireCtx(ctx, TriggerInitSuccess); fireErr != nil {
		m.logf(definitions.LogLevelError, "Error firing TriggerInitSuccess: %+v.", fireErr)
		return fireErr
	}

	return nil // Initialization successful.
}

// onEnterConnected logs entry to connected state.
func (m *Manager) onEnterConnected(ctx context.Context, args ...interface{}) error {
	m.logf(definitions.LogLevelInfo, "Connection established and initialized.")
	return nil
}

// logf logs with connection context.
func (m *Manager) logf(level definitions.LogLevel, format string, v ...interface{}) {
	var currentState State = "UNKNOWN"
	if m.stateMachine != nil {
		s, err := m.stateMachine.State(context.Background()) // Use background context for logging state.
		if err == nil {
			if stateTyped, ok := s.(State); ok {
				currentState = stateTyped
			}
		}
	}
	message := fmt.Sprintf(format, v...)
	// Format: LEVEL [CONN_ID] [STATE] Message
	// Ensure level is valid before logging. Consider adding a check or default.
	m.logger.Printf("%-9s [%s] [%s] %s", level, m.connectionID, currentState, message) // Adjusted padding for longer level names.
}

// NewConnectionServer factory function.
// Renamed from original to avoid conflict if Manager.New was intended.
func NewConnectionServerFactory(serverConfig ServerConfig, resourceMgr ResourceManagerContract, toolMgr ToolManagerContract) (*Manager, error) {
	// Add validation if needed.
	return NewManager(serverConfig, resourceMgr, toolMgr), nil
}

// handleConnectedStateRequest handles standard calls in Connected state.
func (m *Manager) handleConnectedStateRequest(ctx context.Context, conn RPCConnection, req *jsonrpc2.Request,
	trigger Trigger) error {
	var result interface{} // Will hold complex result structs now.
	var handlerErr error

	// Fire reentry trigger.
	if fireErr := m.stateMachine.Fire(trigger, req); fireErr != nil {
		m.logf(definitions.LogLevelWarning, "Failed to fire reentry trigger %s: %v.", trigger, fireErr) // Corrected LogLevel.
	}

	// Call the specific internal handler based on the trigger.
	// Note: Handlers now return complex results or errors.
	switch trigger {
	case TriggerListResources:
		result, handlerErr = m.handleListResources(ctx, req)
	case TriggerReadResource:
		// handleReadResource now returns (definitions.ReadResourceResult, error).
		// The result variable will hold this struct.
		result, handlerErr = m.handleReadResource(ctx, req)
	case TriggerListTools:
		result, handlerErr = m.handleListTools(ctx, req)
	case TriggerCallTool:
		// handleCallTool now returns (definitions.CallToolResult, error).
		result, handlerErr = m.handleCallTool(ctx, req)
	case TriggerPing:
		result, handlerErr = m.handlePing(ctx, req)
	case TriggerSubscribe:
		result, handlerErr = m.handleSubscribe(ctx, req)
	default:
		handlerErr = cgerr.NewMethodNotFoundError(req.Method, map[string]interface{}{"state": "connected"})
	}

	// Handle errors from the specific handler.
	if handlerErr != nil {
		m.logf(definitions.LogLevelError, "Handler for %s failed: %+v.", req.Method, handlerErr) // Use %+v.
		respErr := cgerr.ToJSONRPCError(handlerErr)
		if replyErr := conn.ReplyWithError(ctx, req.ID, respErr); replyErr != nil {
			m.logf(definitions.LogLevelError, "Error sending handler error reply for %s: %+v.", req.Method, replyErr)
		}
		m.handleErrorOccurrence(handlerErr)
		return handlerErr
	}

	// Send the successful result (which might be a complex struct).
	if replyErr := conn.Reply(ctx, req.ID, result); replyErr != nil {
		wrappedErr := cgerr.ErrorWithDetails(
			errors.Wrapf(replyErr, "failed to send success response for %s.", req.Method), // Added period.
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{"method": req.Method},
		)
		m.logf(definitions.LogLevelError, "%+v.", wrappedErr) // Use %+v.
		m.handleErrorOccurrence(wrappedErr)
		return wrappedErr
	}

	return nil // Success.
}

func (m *Manager) handleConnectedStateNotification(ctx context.Context, req *jsonrpc2.Request,
	trigger Trigger) error {
	// Fire trigger.
	if fireErr := m.stateMachine.Fire(trigger, req); fireErr != nil {
		m.logf(definitions.LogLevelWarning, "Failed to fire notification trigger %s: %v.", trigger, fireErr)
	}

	// Process the notification, ignore result/errors per spec.
	var handlerErr error
	switch trigger {
	// Add case for handling the initialized notification
	case TriggerInitializedNotification:
		handlerErr = m.handleInitialized(ctx, req)
	// Existing cases for other notification handlers
	case TriggerListResources, TriggerReadResource, TriggerListTools, TriggerCallTool, TriggerPing, TriggerSubscribe:
		m.logf(definitions.LogLevelDebug, "Received notification for method %s, no specific handler implemented.", req.Method)
	default:
		m.logf(definitions.LogLevelDebug, "Ignoring unhandled notification: %s.", req.Method)
	}

	// Log errors from handlers but take no further action for notifications.
	if handlerErr != nil {
		m.logf(definitions.LogLevelWarning, "Error processing notification %s: %+v.", req.Method, handlerErr)
	}
	return nil // Always return nil for notifications.
}
