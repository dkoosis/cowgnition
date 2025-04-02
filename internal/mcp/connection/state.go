package connection

// Assuming your custom error package exists:
// cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"

// ConnectionState represents the different states a connection can be in.
// Using the exact lowercase values from your original file.
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
type Trigger string // Or use iota if preferred

const (
	TriggerInitialize       Trigger = "Initialize"
	TriggerInitSuccess      Trigger = "InitSuccess" // Fired internally on successful init
	TriggerInitFailure      Trigger = "InitFailure" // Fired internally on failed init
	TriggerListResources    Trigger = "ListResources"
	TriggerReadResource     Trigger = "ReadResource"
	TriggerListTools        Trigger = "ListTools"
	TriggerCallTool         Trigger = "CallTool"
	TriggerShutdown         Trigger = "Shutdown"         // Fired by shutdown request or internal need
	TriggerShutdownComplete Trigger = "ShutdownComplete" // Fired internally after shutdown actions
	TriggerErrorOccurred    Trigger = "ErrorOccurred"    // Fired on significant errors
	TriggerDisconnect       Trigger = "Disconnect"       // Fired if underlying transport closes unexpectedly
	// Add other triggers as needed (Ping, Cancel, Subscribe...)
)

// String provides string representation for logging/debugging.
func (t Trigger) String() string { return string(t) }

// Helper map to translate JSON-RPC method strings to state machine Triggers.
// Expand this with all methods defined in your MCP protocol schema.
var methodToTriggerMap = map[string]Trigger{
	// Initialization / Lifecycle
	"initialize": TriggerInitialize,
	"shutdown":   TriggerShutdown,
	// Resources (adjust method names based on your actual implementation if different from schema)
	"resources/list":      TriggerListResources, // Assuming mapped from schema
	"resources/read":      TriggerReadResource,  // Assuming mapped from schema
	"list_resources":      TriggerListResources, // Keep if used in old code handlers
	"read_resource":       TriggerReadResource,  // Keep if used in old code handlers
	"resources/subscribe": Trigger("Subscribe"), // Example placeholder
	// Tools (adjust method names)
	"tools/list": TriggerListTools, // Assuming mapped from schema
	"tools/call": TriggerCallTool,  // Assuming mapped from schema
	"list_tools": TriggerListTools, // Keep if used in old code handlers
	"call_tool":  TriggerCallTool,  // Keep if used in old code handlers
	// Add mappings for Ping, Cancel, SetLevel, GetPrompt, etc.
	"ping": Trigger("Ping"), // Example placeholder
}

// MapMethodToTrigger translates a method string to a defined Trigger.
func MapMethodToTrigger(method string) (Trigger, bool) {
	t, ok := methodToTriggerMap[method]
	return t, ok
}

// MapErrorToStateTrigger translates specific errors into state machine triggers.
// Customize this based on how you want errors to affect the connection state.
func MapErrorToStateTrigger(err error) Trigger {
	// Example: Add logic to check error types if needed
	// if errors.Is(err, some_critical_resource_error) {
	//     return TriggerErrorOccurred
	// }
	// By default, most handler errors probably don't change the overall connection state
	return ""
}

// isCompatibleProtocolVersion checks if the client's protocol version is compatible.
// Copied from your original file - keep this business logic.
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

// --- DELETED FUNCTIONS ---
// isValidTransition(...) - No longer needed, handled by stateless configuration.
// validateStateTransition(...) - No longer needed, handled by stateless configuration.

// --- Assumed Types (Ensure these are defined elsewhere) ---
// Define LogLevel or remove its usage from logf if not needed.
type LogLevel int // Example placeholder
const (
	LogLevelDebug LogLevel = iota
	LogLevelInfo
	LogLevelWarning
	LogLevelError
)
