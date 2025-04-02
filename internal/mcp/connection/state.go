// internal/mcp/connection/state.go
package connection

import (
	"github.com/cockroachdb/errors"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// ConnectionState represents the different states a connection can be in.
type ConnectionState string

const (
	// StateUnconnected represents a connection that hasn't been established yet.
	StateUnconnected ConnectionState = "unconnected"

	// StateInitializing represents a connection in the initialization phase.
	StateInitializing ConnectionState = "initializing"

	// StateConnected represents a fully established connection.
	StateConnected ConnectionState = "connected"

	// StateTerminating represents a connection that is being closed gracefully.
	StateTerminating ConnectionState = "terminating"

	// StateError represents a connection that has encountered an error.
	StateError ConnectionState = "error"
)

// isValidTransition checks if a state transition is valid.
func isValidTransition(from, to ConnectionState) bool {
	// Define valid transitions
	validTransitions := map[ConnectionState][]ConnectionState{
		StateUnconnected:  {StateInitializing, StateError},
		StateInitializing: {StateConnected, StateError, StateTerminating},
		StateConnected:    {StateTerminating, StateError},
		StateTerminating:  {StateUnconnected, StateError},
		StateError:        {StateUnconnected},
	}

	// Check if the transition is valid
	for _, validTo := range validTransitions[from] {
		if to == validTo {
			return true
		}
	}

	return false
}

// validateStateTransition validates a state transition.
func validateStateTransition(connectionID string, from, to ConnectionState) error {
	if !isValidTransition(from, to) {
		return cgerr.ErrorWithDetails(
			errors.Newf("invalid state transition: %s -> %s", from, to),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"connection_id": connectionID,
				"old_state":     string(from),
				"new_state":     string(to),
			},
		)
	}

	return nil
}

// isCompatibleProtocolVersion checks if the client's protocol version is compatible.
func isCompatibleProtocolVersion(version string) bool {
	// Currently supported versions
	supportedVersions := []string{"2.0", "2024-11-05"}

	for _, supported := range supportedVersions {
		if version == supported {
			return true
		}
	}

	// Future: implement semantic version checking for better compatibility

	return false
}
