// file: internal/mcp/connection/state.go

// Package connection handles the state management and communication logic
// for a single client connection using the MCP protocol.
package connection

// "errors" // Uncomment if errors.Is or similar is used in MapErrorToStateTrigger

// ConnectionState represents the different states a connection can be in.
type ConnectionState string

const (
	StateUnconnected  ConnectionState = "unconnected"
	StateInitializing ConnectionState = "initializing"
	StateConnected    ConnectionState = "connected"
	StateTerminating  ConnectionState = "terminating"
	StateError        ConnectionState = "error"
)

// String provides string representation for logging/debugging.
func (s ConnectionState) String() string { return string(s) }

// Trigger represents events that can cause state transitions.
type Trigger string

const (
	TriggerInitialize       Trigger = "Initialize"
	TriggerInitSuccess      Trigger = "InitSuccess"
	TriggerInitFailure      Trigger = "InitFailure"
	TriggerListResources    Trigger = "ListResources"
	TriggerReadResource     Trigger = "ReadResource"
	TriggerListTools        Trigger = "ListTools"
	TriggerCallTool         Trigger = "CallTool"
	TriggerShutdown         Trigger = "Shutdown"
	TriggerShutdownComplete Trigger = "ShutdownComplete"
	TriggerErrorOccurred    Trigger = "ErrorOccurred"
	TriggerDisconnect       Trigger = "Disconnect"
	// Added from original types.go
	TriggerPing      Trigger = "Ping"
	TriggerSubscribe Trigger = "Subscribe" // Example if needed
)

// String provides string representation for logging/debugging.
func (t Trigger) String() string { return string(t) }

// Helper map to translate JSON-RPC method strings to state machine Triggers.
// Note: Ensure this aligns with your actual JSON-RPC method names and uses defined constants.
var methodToTriggerMap = map[string]Trigger{
	// Initialization / Lifecycle
	"initialize": TriggerInitialize,
	"shutdown":   TriggerShutdown,
	// Resources
	"resources/list":      TriggerListResources, // Preferred MCP style
	"resources/read":      TriggerReadResource,  // Preferred MCP style
	"list_resources":      TriggerListResources, // Legacy support?
	"read_resource":       TriggerReadResource,  // Legacy support?
	"resources/subscribe": TriggerSubscribe,     // Example
	// Tools
	"tools/list": TriggerListTools, // Preferred MCP style
	"tools/call": TriggerCallTool,  // Preferred MCP style
	"list_tools": TriggerListTools, // Legacy support?
	"call_tool":  TriggerCallTool,  // Legacy support?
	// Other
	"ping": TriggerPing, // Example
}

// MapMethodToTrigger translates a method string to a defined Trigger.
func MapMethodToTrigger(method string) (Trigger, bool) {
	t, ok := methodToTriggerMap[method]
	return t, ok
}

// MapErrorToStateTrigger translates specific errors into state machine triggers.
// TODO: Implement actual error checking logic if needed.
func MapErrorToStateTrigger(err error) Trigger {
	// Example: Check for specific error types if they should trigger state changes
	// if errors.Is(err, SomeSpecificErrorType) {
	//     return TriggerSomeErrorCondition
	// }

	// Default or generic error trigger if no specific mapping found
	// Returning empty might mean no specific trigger, handle accordingly in state machine.
	if err != nil {
		return TriggerErrorOccurred // Or return "", depending on state machine logic
	}
	return "" // Return empty string Trigger (or specific NoError trigger if defined)
}

// isCompatibleProtocolVersion checks if the client's protocol version is compatible.
// Using map-based approach from original types.go for clarity.
func isCompatibleProtocolVersion(version string) bool {
	// Define currently supported versions accurately
	supportedVersions := map[string]bool{
		"2.0":        true,
		"2024-11-05": true, // Example specific version string
		// Add other explicitly supported versions here
	}

	// Simple exact match check
	return supportedVersions[version]

	// Future: Could implement semantic version checking
}
