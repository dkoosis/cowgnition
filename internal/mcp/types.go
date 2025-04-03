// file: internal/mcp/types.go
package mcp

import (
	"context"
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
)

// String provides string representation for logging/debugging.
func (t Trigger) String() string { return string(t) }

// ResourceManagerContract defines the interface expected by the connection manager
// for resource management operations.
type ResourceManagerContract interface {
	// GetAllResourceDefinitions returns all available resource definitions.
	GetAllResourceDefinitions() []interface{}

	// ReadResource reads a resource with the given name and arguments.
	// Returns the resource content, MIME type, and any error encountered.
	ReadResource(ctx context.Context, name string, args map[string]string) (interface{}, string, error)
}

// ToolManagerContract defines the interface expected by the connection manager
// for tool management operations.
type ToolManagerContract interface {
	// GetAllToolDefinitions returns all available tool definitions.
	GetAllToolDefinitions() []interface{}

	// CallTool attempts to execute a tool with the given name and arguments.
	// Returns the result of the tool execution and any error encountered.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (interface{}, error)
}

// Helper map to translate JSON-RPC method strings to state machine Triggers.
var methodToTriggerMap = map[string]Trigger{
	// Initialization / Lifecycle
	"initialize": TriggerInitialize,
	"shutdown":   TriggerShutdown,
	// Resources
	"resources/list":      TriggerListResources,
	"resources/read":      TriggerReadResource,
	"list_resources":      TriggerListResources,
	"read_resource":       TriggerReadResource,
	"resources/subscribe": Trigger("Subscribe"),
	// Tools
	"tools/list": TriggerListTools,
	"tools/call": TriggerCallTool,
	"list_tools": TriggerListTools,
	"call_tool":  TriggerCallTool,
	"ping":       Trigger("Ping"),
}

// MapMethodToTrigger translates a method string to a defined Trigger.
func MapMethodToTrigger(method string) (Trigger, bool) {
	t, ok := methodToTriggerMap[method]
	return t, ok
}

// MapErrorToStateTrigger translates specific errors into state machine triggers.
func MapErrorToStateTrigger(err error) Trigger {
	// Add logic to check error types if needed
	return ""
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
