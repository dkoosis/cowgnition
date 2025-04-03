// Package connection handles the state management and communication logic
// for a single client connection using the MCP protocol.
package connection

// Import the core MCP types from the mcp package
// IMPORTANT: Replace with the correct import path for your module
import (
	"github.com/dkoosis/cowgnition/internal/mcp"
	// "errors" // Uncomment if errors.Is or similar is needed in MapErrorToStateTrigger
)

// --- Removed duplicate type and constant definitions ---
// ConnectionState, State*, Trigger, Trigger* constants are now defined in the mcp package.
// isCompatibleProtocolVersion is now mcp.IsCompatibleProtocolVersion

// methodToTriggerMap translates known JSON-RPC method strings to their
// corresponding MCP state machine Triggers (defined in the mcp package).
// Ensure this aligns with your actual expected JSON-RPC method names.
var methodToTriggerMap = map[string]mcp.Trigger{
	// Initialization / Lifecycle
	"initialize": mcp.TriggerInitialize,
	"shutdown":   mcp.TriggerShutdown,

	// Resources (Using preferred MCP style, add legacy if needed)
	"resources/list":      mcp.TriggerListResources,
	"resources/read":      mcp.TriggerReadResource,
	"resources/subscribe": mcp.TriggerSubscribe, // Example

	// Tools (Using preferred MCP style, add legacy if needed)
	"tools/list": mcp.TriggerListTools,
	"tools/call": mcp.TriggerCallTool,

	// Other
	"ping": mcp.TriggerPing,

	// Add legacy method names if required for backward compatibility:
	// "list_resources": mcp.TriggerListResources,
	// "read_resource":  mcp.TriggerReadResource,
	// "list_tools":     mcp.TriggerListTools,
	// "call_tool":      mcp.TriggerCallTool,
}

// MapMethodToTrigger translates a method string to a defined mcp.Trigger.
// It returns the corresponding trigger and true if found, otherwise an empty trigger and false.
func MapMethodToTrigger(method string) (mcp.Trigger, bool) {
	t, ok := methodToTriggerMap[method]
	return t, ok
}

// MapErrorToStateTrigger translates specific errors into state machine triggers.
// This version triggers a generic error state change if any error is passed.
// Customize this function if specific errors should lead to different state transitions.
func MapErrorToStateTrigger(err error) mcp.Trigger {
	// Example: Check for specific error types if they should trigger different states
	// if errors.Is(err, SomeSpecificErrorType) {
	//     return mcp.TriggerSomeErrorCondition
	// }

	if err != nil {
		// Signal that a generic error occurred, potentially moving the state machine
		// to the mcp.StateError state via the mcp.TriggerErrorOccurred.
		return mcp.TriggerErrorOccurred
	}

	// Return an empty string Trigger if no error occurred, indicating no
	// error-driven state change is needed based on this function's check.
	return ""
}
