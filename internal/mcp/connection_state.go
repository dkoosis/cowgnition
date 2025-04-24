// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// file: internal/mcp/connection_state.go
// MODIFIED: Added State type, StateInitializing constant, SetInitializing method,
// ensured setState helper exists and is used by setters.
package mcp

import (
	"fmt"
	"sync"

	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // Use alias if needed, or direct path.
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"       // Import mcptypes for shared types.
)

// State represents the MCP connection state as a string type.
type State string // <<< ADDED TYPE DEFINITION

// Constants for connection states.
const (
	StateUninitialized State = "uninitialized"
	StateInitializing  State = "initializing" // <<< ADDED CONSTANT
	StateInitialized   State = "initialized"  // Keep this name for consistency
	StateShuttingDown  State = "shuttingDown"
	StateShutdown      State = "shutdown"
)

// ConnectionState manages the lifecycle state of an MCP connection.
// It ensures state transitions are valid according to the MCP spec.
type ConnectionState struct {
	mu           sync.RWMutex
	currentState State
	clientInfo   *mcptypes.Implementation     // Use mcptypes.Implementation.
	clientCaps   *mcptypes.ClientCapabilities // Use mcptypes.ClientCapabilities.
}

// NewConnectionState creates a new ConnectionState manager, initialized to Uninitialized.
func NewConnectionState() *ConnectionState {
	return &ConnectionState{
		currentState: StateUninitialized,
		// clientInfo and clientCaps start as nil.
	}
}

// setState safely updates the current state.
// This is the internal helper used by public Set* methods.
func (cs *ConnectionState) setState(newState State) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	// TODO: Add logging here if desired to track state changes.
	// cs.logger.Debug("Connection state changing", "from", cs.currentState, "to", newState).
	cs.currentState = newState
}

// CurrentState returns the current connection state safely.
func (cs *ConnectionState) CurrentState() State {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.currentState
}

// SetInitializing sets the state to Initializing.
// Ensures thread-safety.
func (cs *ConnectionState) SetInitializing() {
	cs.setState(StateInitializing) // <<< ADDED METHOD (uses setState)
}

// SetInitialized sets the state to Initialized.
// Ensures thread-safety.
func (cs *ConnectionState) SetInitialized() {
	cs.setState(StateInitialized) // <<< UPDATED to use setState
}

// SetShutdown sets the state to ShuttingDown.
// Ensures thread-safety.
func (cs *ConnectionState) SetShutdown() {
	cs.setState(StateShuttingDown) // <<< UPDATED to use setState
}

// SetClientInfo stores the client information received during initialization.
func (cs *ConnectionState) SetClientInfo(info mcptypes.Implementation) { // Use mcptypes.Implementation.
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.clientInfo = &info
}

// GetClientInfo returns the stored client information.
func (cs *ConnectionState) GetClientInfo() (mcptypes.Implementation, bool) { // Use mcptypes.Implementation.
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.clientInfo == nil {
		return mcptypes.Implementation{}, false
	}
	return *cs.clientInfo, true
}

// SetClientCapabilities stores the client capabilities received during initialization.
func (cs *ConnectionState) SetClientCapabilities(caps mcptypes.ClientCapabilities) { // Use mcptypes.ClientCapabilities.
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.clientCaps = &caps
}

// GetClientCapabilities returns the stored client capabilities.
func (cs *ConnectionState) GetClientCapabilities() (mcptypes.ClientCapabilities, bool) { // Use mcptypes.ClientCapabilities.
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	if cs.clientCaps == nil {
		return mcptypes.ClientCapabilities{}, false
	}
	return *cs.clientCaps, true
}

// ValidateMethodSequence checks if a method is allowed in the current connection state.
// Returns a specific MCP error if the sequence is invalid.
func (cs *ConnectionState) ValidateMethodSequence(method string) error {
	cs.mu.RLock()
	currentState := cs.currentState
	cs.mu.RUnlock()

	// Map methods to the states they are allowed in.
	//nolint:ineffassign // Required default.
	allowed := true // Assume allowed by default unless restricted.

	switch method {
	case "initialize":
		allowed = (currentState == StateUninitialized)
	case "shutdown":
		// Shutdown request should only come when initialized.
		allowed = (currentState == StateInitialized)
	case "exit":
		// Exit notification can come after initialized or during shutdown.
		allowed = (currentState == StateInitialized || currentState == StateShuttingDown)
	case "ping", // Assuming ping requires initialization
		"tools/list", "tools/call",
		"resources/list", "resources/read", "resources/subscribe", "resources/unsubscribe",
		"prompts/list", "prompts/get",
		"completion/complete",
		"logging/setLevel":
		// General operational methods require the initialized state.
		allowed = (currentState == StateInitialized)
	case "notifications/initialized":
		// This notification should only arrive when we are initializing.
		allowed = (currentState == StateInitializing) // <<< UPDATED CHECK
	case "$/cancelRequest":
		// Cancellation logically only makes sense when initialized.
		allowed = (currentState == StateInitialized) // <<< ADDED CHECK (Optional but recommended)
	default:
		// Allow unknown methods for now? Or restrict?
		// If restricted, could use: allowed = false
		// For now, allow unrecognized methods (validation middleware should catch schema issues).
		allowed = true
	}

	if !allowed {
		// Use mcperrors constants for specific errors.
		// Using ErrRequestSequence as a general purpose sequence error.
		return mcperrors.NewProtocolError(mcperrors.ErrRequestSequence,
			fmt.Sprintf("Method '%s' not allowed in current state '%s'", method, currentState),
			nil, map[string]interface{}{"method": method, "state": currentState})
	}

	return nil
}
