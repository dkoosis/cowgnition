// file: internal/mcp/connection/connection_types.go
package connection

import (
	"context"

	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	"github.com/sourcegraph/jsonrpc2" // Needed for RPCConnection interface types.
)

// ResourceManagerContract defines the interface expected by the connection manager
// for resource management operations.
type ResourceManagerContract interface {
	// GetAllResourceDefinitions returns all available resource definitions.
	GetAllResourceDefinitions() []definitions.ResourceDefinition

	// ReadResource reads a resource with the given name and arguments.
	// Returns the resource content, MIME type, and any error encountered.
	ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error)
}

// ToolManagerContract defines the interface expected by the connection manager
// for tool management operations.
type ToolManagerContract interface {
	// GetAllToolDefinitions returns all available tool definitions.
	GetAllToolDefinitions() []definitions.ToolDefinition

	// CallTool attempts to execute a tool with the given name and arguments.
	// Returns the result of the tool execution and any error encountered.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error)
}

// RPCConnection defines the subset of *jsonrpc2.Conn methods used by Manager.
// This allows for mocking the connection in tests by depending on an interface
// rather than the concrete *jsonrpc2.Conn type.
type RPCConnection interface {
	// Reply sends a successful JSON-RPC response.
	Reply(ctx context.Context, id jsonrpc2.ID, result interface{}) error
	// ReplyWithError sends a JSON-RPC error response.
	ReplyWithError(ctx context.Context, id jsonrpc2.ID, respErr *jsonrpc2.Error) error
	// Close terminates the underlying connection.
	Close() error
	// Other methods from *jsonrpc2.Conn could be added here if the Manager requires them.
}

// State represents the connection lifecycle states.
// type State string

// Trigger represents events that cause state transitions.
// type Trigger string
