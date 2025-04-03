// file: internal/mcp/connection/types.go
package connection

import (
	"context"
	// Import the definitions package for shared types
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
)

// ResourceManager interface represents the contract for resource management
// required by the ConnectionManager.
type ResourceManager interface {
	// GetAllResourceDefinitions returns all available resource definitions.
	GetAllResourceDefinitions() []definitions.ResourceDefinition

	// ReadResource reads the content and determines the mime type of a resource.
	ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error)
}

// ToolManager interface represents the contract for tool management
// required by the ConnectionManager.
type ToolManager interface {
	// GetAllToolDefinitions returns all available tool definitions.
	GetAllToolDefinitions() []definitions.ToolDefinition

	// CallTool executes a tool and returns its result as a string.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error)
}
