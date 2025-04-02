// internal/mcp/connection/manager.go
package connection

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// MessageHandler is a function that handles a JSON-RPC message.
type MessageHandler func(ctx context.Context, req *jsonrpc2.Request) (interface{}, error)

// ServerConfig contains configuration options for the ConnectionManager.
type ServerConfig struct {
	Name            string
	Version         string
	Capabilities    map[string]interface{}
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
}

// ConnectionManager manages the connection state and dispatches messages based on the current state.
type ConnectionManager struct {
	// The current state of the connection
	state ConnectionState

	// Mutex to protect state changes
	mu sync.RWMutex

	// The JSON-RPC connection
	conn *jsonrpc2.Conn

	// Connection ID for logging
	connectionID string

	// Time when the connection was established
	startTime time.Time

	// Last activity timestamp
	lastActivity time.Time

	// Context for the connection
	ctx context.Context

	// Cancel function for the context
	cancel context.CancelFunc

	// Message handlers for different states
	handlers map[ConnectionState]map[string]MessageHandler

	// Server configuration
	config ServerConfig

	// Logger for structured logging
	logger *log.Logger

	// Client capabilities
	clientCapabilities map[string]interface{}

	// Resource manager
	resourceManager *ResourceManager

	// Tool manager
	toolManager *ToolManager
}

// NewConnectionManager creates a new ConnectionManager with the given configuration.
func NewConnectionManager(config ServerConfig, resourceManager *ResourceManager, toolManager *ToolManager) *ConnectionManager {
	ctx, cancel := context.WithCancel(context.Background())

	manager := &ConnectionManager{
		state:              StateUnconnected,
		connectionID:       generateConnectionID(),
		startTime:          time.Now(),
		lastActivity:       time.Now(),
		ctx:                ctx,
		cancel:             cancel,
		handlers:           make(map[ConnectionState]map[string]MessageHandler),
		config:             config,
		logger:             log.New(log.Writer(), "[MCP] ", log.LstdFlags),
		clientCapabilities: make(map[string]interface{}),
		resourceManager:    resourceManager,
		toolManager:        toolManager,
	}

	// Initialize handler maps for each state
	for _, state := range []ConnectionState{
		StateUnconnected,
		StateInitializing,
		StateConnected,
		StateTerminating,
		StateError,
	} {
		manager.handlers[state] = make(map[string]MessageHandler)
	}

	// Register default handlers
	manager.registerDefaultHandlers()

	return manager
}

// registerDefaultHandlers registers the default message handlers for each state.
func (m *ConnectionManager) registerDefaultHandlers() {
	// Register initialize handler for unconnected state
	m.RegisterHandler(StateUnconnected, "initialize", m.handleInitialize)

	// Register handlers for connected state
	m.RegisterHandler(StateConnected, "list_resources", m.handleListResources)
	m.RegisterHandler(StateConnected, "read_resource", m.handleReadResource)
	m.RegisterHandler(StateConnected, "list_tools", m.handleListTools)
	m.RegisterHandler(StateConnected, "call_tool", m.handleCallTool)

	// Register shutdown handler for all states
	for _, state := range []ConnectionState{
		StateInitializing,
		StateConnected,
	} {
		m.RegisterHandler(state, "shutdown", m.handleShutdown)
	}
}

// RegisterHandler registers a message handler for a specific state and method.
func (m *ConnectionManager) RegisterHandler(state ConnectionState, method string, handler MessageHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.handlers[state][method] = handler
	m.logf(LogLevelInfo, "Registered handler for state %s, method %s", state, method)
}

// GetState returns the current connection state.
func (m *ConnectionManager) GetState() ConnectionState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.state
}

// SetState transitions the connection to a new state.
func (m *ConnectionManager) SetState(newState ConnectionState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldState := m.state

	// Validate state transition
	if err := validateStateTransition(m.connectionID, oldState, newState); err != nil {
		m.logf(LogLevelError, "Invalid state transition: %s -> %s: %v", oldState, newState, err)
		return err
	}

	m.state = newState
	m.lastActivity = time.Now()

	m.logf(LogLevelInfo, "Connection state changed: %s -> %s", oldState, newState)

	return nil
}

// Connect establishes the connection and starts the state machine.
func (m *ConnectionManager) Connect(conn *jsonrpc2.Conn) error {
	m.mu.Lock()
	m.conn = conn
	m.mu.Unlock()

	// Start in unconnected state
	m.logf(LogLevelInfo, "Starting connection manager (server: %s, version: %s)",
		m.config.Name, m.config.Version)

	return nil
}

// Shutdown gracefully terminates the connection.
func (m *ConnectionManager) Shutdown() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state == StateTerminating || m.state == StateUnconnected {
		m.logf(LogLevelInfo, "Connection already terminated or not established")
		return nil
	}

	m.logf(LogLevelInfo, "Shutting down connection manager")

	// Set state to terminating
	m.state = StateTerminating

	// Cancel the context
	m.cancel()

	// Close the connection if it exists
	if m.conn != nil {
		if err := m.conn.Close(); err != nil {
			m.logf(LogLevelError, "Error closing connection: %v", err)
		}
	}

	// Set state to unconnected
	m.state = StateUnconnected

	return nil
}

// Handle implements the jsonrpc2.Handler interface.
func (m *ConnectionManager) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	m.mu.Lock()
	m.conn = conn
	m.lastActivity = time.Now()
	currentState := m.state
	m.mu.Unlock()

	m.logf(LogLevelDebug, "Received request: %s (id: %s)", req.Method, req.ID)

	// Find the appropriate handler for the current state and method
	m.mu.RLock()
	handler, ok := m.handlers[currentState][req.Method]
	m.mu.RUnlock()

	if !ok {
		// If no handler is found for the current state, check if there's a default handler
		m.mu.RLock()
		handler, ok = m.handlers[StateConnected][req.Method]
		m.mu.RUnlock()

		if !ok {
			// No handler found for this method
			if req.Notif {
				// Ignore notifications without handlers
				return
			}

			err := cgerr.NewMethodNotFoundError(req.Method, map[string]interface{}{
				"connection_id": m.connectionID,
				"state":         currentState,
			})

			m.logf(LogLevelError, "No handler for method %s in state %s: %v", req.Method, currentState, err)
			conn.ReplyWithError(ctx, req.ID, jsonrpc2.Error{
				Code:    jsonrpc2.CodeMethodNotFound,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
				Data:    map[string]interface{}{"state": string(currentState)},
			})
			return
		}
	}

	// Create a request context with timeout
	reqCtx, cancel := context.WithTimeout(ctx, m.config.RequestTimeout)
	defer cancel()

	// Record the request start time for performance monitoring
	startTime := time.Now()

	// Execute the handler
	result, err := handler(reqCtx, req)

	// Log the execution time
	duration := time.Since(startTime)
	m.logf(LogLevelDebug, "Handler execution time for %s: %s", req.Method, duration)

	// Handle the result or error
	if err != nil {
		m.handleError(ctx, conn, req, err)
		return
	}

	// Skip response for notifications
	if req.Notif {
		return
	}

	// Send the response
	if err := conn.Reply(ctx, req.ID, result); err != nil {
		m.logf(LogLevelError, "Failed to send response: %v", err)
	}
}
