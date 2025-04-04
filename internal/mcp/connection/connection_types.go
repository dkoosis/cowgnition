// file: internal/mcp/connection/connection_types.go
package connection

import (
	"context"

	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
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
