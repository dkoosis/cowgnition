// Package state defines the specific states and events for the MCP connection lifecycle.
// file: internal/mcp/state/machine.go
package state

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/fsm"                      // Import the generic fsm package.
	"github.com/dkoosis/cowgnition/internal/logging"                  // For logging.
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // For MCP-specific errors.
)

// MCPStateMachine represents the state machine for an MCP connection lifecycle.
// It embeds the generic FSM interface to provide core functionality.
type MCPStateMachine struct {
	fsm.FSM // Embed the generic FSM interface.
	logger  logging.Logger
}

// NewMCPStateMachine creates and configures a new state machine for the MCP lifecycle.
func NewMCPStateMachine(logger logging.Logger) (*MCPStateMachine, error) {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	log := logger.WithField("component", "mcp_state_machine")

	// Initialize the base FSM builder with the starting state.
	fsmBuilder := fsm.NewFSM(StateUninitialized, log)

	// Define MCP State Transitions based on the protocol specification.
	// Use the States (StateUninitialized, etc.) and Events (EventInitializeRequest, etc.).
	// defined in states.go and events.go.

	// --- Initialization Flow ---.
	fsmBuilder.AddTransition(fsm.Transition{
		From:  []fsm.State{StateUninitialized},
		Event: EventInitializeRequest,
		To:    StateInitializing,
		// Action: Optional action on receiving initialize request.
	})
	// <<< REMOVED Transition for EventInitializeResponseSent >>>.
	fsmBuilder.AddTransition(fsm.Transition{
		From:  []fsm.State{StateInitializing},
		Event: EventClientInitialized, // Triggered by notifications/initialized from client.
		To:    StateInitialized,
		// Action: Optional action when client confirms initialization.
	})

	// --- Operational Flow (In Initialized State) ---.
	fsmBuilder.AddTransition(fsm.Transition{
		From:  []fsm.State{StateInitialized},
		Event: EventMCPRequest,  // Generic event for most requests in Initialized state.
		To:    StateInitialized, // Typically remain Initialized after processing a request.
		// Action: Handled by the Router/Method Handlers.
	})
	fsmBuilder.AddTransition(fsm.Transition{
		From:  []fsm.State{StateInitialized},
		Event: EventMCPNotification, // Generic event for notifications in Initialized state.
		To:    StateInitialized,     // Remain Initialized.
		// Action: Handled by the Router/Notification Handlers.
	})

	// --- Shutdown Flow ---.
	fsmBuilder.AddTransition(fsm.Transition{
		From:  []fsm.State{StateInitialized}, // Can only shut down if initialized.
		Event: EventShutdownRequest,
		To:    StateShuttingDown,
		// Action: Optional action on receiving shutdown request.
	})
	// <<< REMOVED Transition for EventShutdownResponseSent >>>.
	fsmBuilder.AddTransition(fsm.Transition{
		From:  []fsm.State{StateInitialized, StateShuttingDown}, // Client can send exit in either state.
		Event: EventExitNotification,
		To:    StateShutdown, // Terminal state.
		// Action: Optional action on receiving exit notification (e.g., trigger connection close).
	})

	// --- Error Handling / Reset ---.
	// Allow resetting from any state back to uninitialized (e.g., on transport error).
	// We can use a generic "ErrorOccurred" event or handle this externally by calling Reset().
	// Let's add a transition for transport errors for explicitness.
	fsmBuilder.AddTransition(fsm.Transition{
		From:  []fsm.State{StateUninitialized, StateInitializing, StateInitialized, StateShuttingDown},
		Event: EventTransportErrorOccurred,
		To:    StateShutdown, // Or potentially a distinct Error state? Shutdown seems reasonable.
		// Action: Log the error details passed in the event data.
	})
	// Add a global Reset event if needed, or rely on the Reset() method.

	// Build the underlying FSM.
	err := fsmBuilder.Build()
	if err != nil {
		log.Error("Failed to build MCP state machine.", "error", err)
		return nil, errors.Wrap(err, "failed to build MCP state machine configuration")
	}

	log.Info("MCP state machine built successfully.")
	return &MCPStateMachine{
		FSM:    fsmBuilder,
		logger: log,
	}, nil
}

// ValidateMethod checks if receiving a specific MCP method is valid in the current state.
// It maps the method name to a corresponding lifecycle event and checks if that event.
// can trigger a transition from the current state.
// Returns an ErrRequestSequence protocol error if the method is not allowed.
func (m *MCPStateMachine) ValidateMethod(method string) error {
	currentState := m.CurrentState() // Get current state safely via embedded FSM.

	// Map the method string to a potential lifecycle event.
	event := EventForMethod(method)

	// If the method doesn't correspond to a specific lifecycle event,.
	// assume it's a standard operational request/notification.
	// These are only allowed in the Initialized state.
	if event == "" {
		if currentState == StateInitialized {
			// Standard methods are allowed in Initialized state.
			// The specific method validity (e.g., unknown method) will be checked by the Router.
			return nil
		}
		// If not initialized, standard methods are not allowed.
		m.logger.Warn("Received standard MCP method in non-initialized state.",
			"method", method, "state", currentState)
		return mcperrors.NewProtocolError(
			mcperrors.ErrRequestSequence, // Use the specific MCP error code.
			fmt.Sprintf("Method '%s' not allowed before initialization (state: '%s')", method, currentState),
			nil,
			map[string]interface{}{"method": method, "state": currentState},
		)
	}

	// If it's a specific lifecycle event, check if the FSM allows it from the current state.
	// Note: CanTransition only checks if the event *exists* for the state, not if guards pass.
	// The actual Transition call will handle guards. This check prevents trying.
	// to fire completely undefined events for a state.
	if !m.CanTransition(event) {
		m.logger.Warn("Received out-of-sequence MCP lifecycle method.",
			"method", method, "event", event, "state", currentState)
		return mcperrors.NewProtocolError(
			mcperrors.ErrRequestSequence, // Use the specific MCP error code.
			fmt.Sprintf("Method '%s' (event '%s') not allowed in current state '%s'", method, event, currentState),
			nil,
			map[string]interface{}{"method": method, "event": event, "state": currentState},
		)
	}

	// Method sequence is valid according to FSM definitions.
	return nil
}

// TriggerEvent attempts to transition the state machine based on an internal event,.
// such as successfully sending a response.
// NOTE: This function remains but might be unused if internal FSM events are removed.
func (m *MCPStateMachine) TriggerEvent(ctx context.Context, event fsm.Event, data interface{}) error {
	m.logger.Debug("Triggering internal FSM event.", "event", event, "state", m.CurrentState())
	// Use the embedded Transition method.
	err := m.Transition(ctx, event, data)
	if err != nil {
		// Log errors, especially if guards fail unexpectedly for internal events.
		m.logger.Error("Failed to trigger internal FSM event.", "event", event, "state", m.CurrentState(), "error", err)
		// Decide if this should be fatal or just logged based on the event type.
		return err
	}
	return nil
}
