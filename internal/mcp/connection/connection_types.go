// file: internal/mcp/connection/connection_types.go
// Package connection defines types and interfaces related to managing MCP connections.
// These interfaces MUST align with the corresponding interfaces in the mcp package
// and use the corrected MCP data structures from the definitions package.
// Terminate all comments with a period.
package connection

import (
	"context"

	// Use the corrected definitions package.
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	"github.com/sourcegraph/jsonrpc2" // Needed for RPCConnection interface types.
)

// ResourceManagerContract defines the interface expected by the connection manager
// for resource management operations. This MUST match mcp.ResourceManager.
type ResourceManagerContract interface {
	// GetAllResourceDefinitions returns all available resource definitions.
	// Return type updated to use corrected definitions.Resource.
	GetAllResourceDefinitions() []definitions.Resource

	// ReadResource reads a resource identified by its URI.
	// Returns a definitions.ReadResourceResult containing the resource contents.
	// Signature updated to use URI and return definitions.ReadResourceResult.
	ReadResource(ctx context.Context, uri string) (definitions.ReadResourceResult, error)
}

// ToolManagerContract defines the interface expected by the connection manager
// for tool management operations. This MUST match mcp.ToolManager.
type ToolManagerContract interface {
	// GetAllToolDefinitions returns all available tool definitions.
	// Return type updated to use corrected definitions.ToolDefinition.
	GetAllToolDefinitions() []definitions.ToolDefinition

	// CallTool attempts to execute a tool with the given name and arguments.
	// Returns a definitions.CallToolResult containing the tool's output or error status.
	// Return type updated to definitions.CallToolResult.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (definitions.CallToolResult, error)
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
// type State string // Definition likely exists elsewhere in package (e.g., state.go).

// Trigger represents events that cause state transitions.
// type Trigger string // Definition likely exists elsewhere in package (e.g., state.go).
