// file: internal/mcp/connection/types.go

// Package connection handles the state management and communication logic
// for a single client connection using the MCP protocol.
package connection

import (
	"context"
	// Import the definitions package to use specific MCP types
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	// "errors" // Uncomment if errors.Is or similar is used in MapErrorToStateTrigger
)

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
	// Add any other necessary triggers like Ping, Subscribe, etc.
	TriggerPing      Trigger = "Ping"
	TriggerSubscribe Trigger = "Subscribe" // Example if needed
)

// String provides string representation for logging/debugging.
func (t Trigger) String() string { return string(t) }

// ResourceManagerContract defines the interface expected by the connection manager
// for resource management operations, using specific definition types.
type ResourceManagerContract interface {
	// GetAllResourceDefinitions returns all available resource definitions.
	GetAllResourceDefinitions() []definitions.ResourceDefinition // Use specific type

	// ReadResource reads a resource with the given name and arguments.
	// Returns the resource content (string), MIME type (string), and any error encountered.
	ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) // Return specific types (content, mime)
}

// ToolManagerContract defines the interface expected by the connection manager
// for tool management operations, using specific definition types.
type ToolManagerContract interface {
	// GetAllToolDefinitions returns all available tool definitions.
	GetAllToolDefinitions() []definitions.ToolDefinition // Use specific type

	// CallTool attempts to execute a tool with the given name and arguments.
	// Returns the result of the tool execution (string) and any error encountered.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) // Return specific type (result string)
}

// Helper map to translate JSON-RPC method strings to state machine Triggers.
// Note: Ensure this aligns with your actual JSON-RPC method names.
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
	return ""
}

// isCompatibleProtocolVersion checks if the client's protocol version is compatible.
func isCompatibleProtocolVersion(version string) bool {
	// Define currently supported versions accurately
	supportedVersions := map[string]bool{
		"2.0":        true,
		"2024-11-05": true, // Example specific version string
		// Add other explicitly supported versions here
	}

	// Simple exact match check
	return supportedVersions[version]

	// Future: Could implement semantic version checking (e.g., using libraries
	// like "golang.org/x/mod/semver") if versions follow semver patterns
	// and compatibility rules are defined (e.g., compatible with >= 2.0, < 3.0)
}
