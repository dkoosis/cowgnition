// Package state defines the specific states and events for the MCP connection lifecycle.
// file: internal/mcp/state/events.go
package state

import "github.com/dkoosis/cowgnition/internal/fsm" // Import the generic fsm package.

// MCP Events representing triggers for state transitions.
// These usually correspond to specific MCP requests or notifications being received or sent.
const (
	// Client -> Server Events.
	EventInitializeRequest      fsm.Event = "rcvd_initialize_request"       // Client sent 'initialize'.
	EventClientInitialized      fsm.Event = "rcvd_client_initialized_notif" // Client sent 'notifications/initialized'.
	EventShutdownRequest        fsm.Event = "rcvd_shutdown_request"         // Client sent 'shutdown'.
	EventExitNotification       fsm.Event = "rcvd_exit_notification"        // Client sent 'exit'.
	EventMCPRequest             fsm.Event = "rcvd_mcp_request"              // Any other request received (e.g., tools/list).
	EventMCPNotification        fsm.Event = "rcvd_mcp_notification"         // Any other notification received (e.g., $/cancelRequest).
	EventTransportErrorOccurred fsm.Event = "transport_error"               // An underlying transport error happened.

	// Server -> Client Events (or internal triggers).
	// Note: While the FSM primarily tracks state based on *received* messages,.
	// internal events can be useful for more complex flows if needed later.
	EventInitializeResponseSent fsm.Event = "sent_initialize_response" // Server successfully sent 'initialize' result.
	EventShutdownResponseSent   fsm.Event = "sent_shutdown_response"   // Server successfully sent 'shutdown' result.
	// Add more events as needed, for example:.
	// EventCancelRequest fsm.Event = "rcvd_cancel_request".
)

// EventForMethod maps an incoming MCP method string to a corresponding FSM event.
// Returns an empty event if the method doesn't have a specific lifecycle event.
func EventForMethod(method string) fsm.Event {
	switch method {
	case "initialize":
		return EventInitializeRequest
	case "shutdown":
		return EventShutdownRequest
	case "exit":
		return EventExitNotification
	case "notifications/initialized":
		return EventClientInitialized
	// Add other specific methods if they trigger unique state transitions.
	// e.g., case "$/cancelRequest": return EventCancelRequest.
	default:
		// Return a generic event or empty if the method doesn't directly cause a standard lifecycle transition.
		// The router will handle dispatching methods allowed in the 'Initialized' state.
		// We could return EventMCPRequest/EventMCPNotification here if we wanted the FSM.
		// to explicitly handle *all* message types, but the plan seems to focus on lifecycle.
		// Let's return empty for now, relying on ValidateMethod in the FSM implementation.
		return ""
	}
}

// NOTE: The plan [cite: 21] showed `mcp_state.NewEvent(...)`, but similar to states,.
// we are defining events within this package using the base `fsm.Event` type (string).
// We directly assign string constants. The `EventForMethod` helper maps method strings.
// to these constants.
