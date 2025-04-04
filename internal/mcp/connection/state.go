// file: internal/mcp/connection/state.go
package connection

// ConnectionState represents the different states a connection can be in.
type ConnectionState string

const (
	StateUnconnected  ConnectionState = "unconnected"
	StateInitializing ConnectionState = "initializing"
	StateConnected    ConnectionState = "connected"
	StateTerminating  ConnectionState = "terminating"
	StateError        ConnectionState = "error"
)

func (s ConnectionState) String() string { return string(s) }

// Trigger represents events that can cause state transitions within the state machine.
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
	TriggerPing             Trigger = "Ping"
	TriggerSubscribe        Trigger = "Subscribe"
)

func (t Trigger) String() string { return string(t) }

// methodToTriggerMap translates known JSON-RPC method strings to their
// corresponding state machine Triggers.
var methodToTriggerMap = map[string]Trigger{
	// Initialization / Lifecycle
	"initialize": TriggerInitialize,
	"shutdown":   TriggerShutdown,

	// Resources
	"resources/list":      TriggerListResources,
	"resources/read":      TriggerReadResource,
	"resources/subscribe": TriggerSubscribe,

	// Tools
	"tools/list": TriggerListTools,
	"tools/call": TriggerCallTool,

	// Other
	"ping": TriggerPing,
}

// MapMethodToTrigger translates a method string to a defined Trigger.
// It returns the corresponding trigger and true if found, otherwise an empty trigger and false.
func MapMethodToTrigger(method string) (Trigger, bool) {
	t, ok := methodToTriggerMap[method]
	return t, ok
}

// MapErrorToStateTrigger translates specific errors into state machine triggers.
// This version triggers a generic error state change if any error is passed.
func MapErrorToStateTrigger(err error) Trigger {
	if err != nil {
		return TriggerErrorOccurred
	}
	return ""
}

// isCompatibleProtocolVersion checks if the client's protocol version is compatible.
func isCompatibleProtocolVersion(clientVersion string) bool {
	supportedVersions := map[string]bool{"2.0": true, "2024-11-05": true}
	return supportedVersions[clientVersion]
}
