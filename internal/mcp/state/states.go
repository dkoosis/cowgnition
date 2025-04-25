// Package state defines the specific states and events for the MCP connection lifecycle.
// file: internal/mcp/state/states.go
package state

import "github.com/dkoosis/cowgnition/internal/fsm" // Import the generic fsm package.

// MCP States based on the protocol lifecycle.
// These constants represent the defined states using the generic fsm.State type.
const (
	StateUninitialized fsm.State = "uninitialized" // Connection established, pre-initialization.
	StateInitializing  fsm.State = "initializing"  // Initialize request sent, awaiting client initialized notification.
	StateInitialized   fsm.State = "initialized"   // Handshake complete, ready for general requests.
	StateShuttingDown  fsm.State = "shuttingDown"  // Shutdown request received/sent, awaiting exit.
	StateShutdown      fsm.State = "shutdown"      // Exit notification received, connection effectively closed.
)

// IsTerminal returns true if the state represents a terminal state from which
// no further transitions should normally occur (excluding potential resets).
func IsTerminal(s fsm.State) bool {
	return s == StateShutdown
}

// NOTE: The plan [cite: 21] showed `mcp_state.NewState(...)`, but since we are defining
// states *within* this package and using the base `fsm.State` type (which is just a string),
// we directly assign the string constants here. This avoids needing a local `NewState` helper.
