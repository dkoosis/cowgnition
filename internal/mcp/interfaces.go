// file: internal/mcp/interfaces.go
package mcp

import (
	"context"

	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
)

// ResourceProvider defines an interface for components that provide MCP resources.
type ResourceProvider interface {
	// GetResourceDefinitions returns the list of resources this provider handles.
	GetResourceDefinitions() []definitions.ResourceDefinition

	// ReadResource attempts to read the content of a resource with the given name and arguments.
	// Returns the resource content, MIME type, and any error encountered.
	ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error)
}

// ToolProvider defines an interface for components that provide MCP tools.
type ToolProvider interface {
	// GetToolDefinitions returns the list of tools this provider handles.
	GetToolDefinitions() []definitions.ToolDefinition

	// CallTool attempts to execute a tool with the given name and arguments.
	// Returns the result of the tool execution and any error encountered.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error)
}

// ResourceManager defines the interface for managing resource providers
type ResourceManager interface {
	// RegisterProvider registers a ResourceProvider.
	RegisterProvider(provider ResourceProvider)

	// GetAllResourceDefinitions returns all resource definitions from all providers.
	GetAllResourceDefinitions() []definitions.ResourceDefinition

	// ReadResource reads a resource across all providers.
	ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error)
}

// ToolManager defines the interface for managing tool providers
type ToolManager interface {
	// RegisterProvider registers a ToolProvider.
	RegisterProvider(provider ToolProvider)

	// GetAllToolDefinitions returns all tool definitions from all providers.
	GetAllToolDefinitions() []definitions.ToolDefinition

	// CallTool calls a tool across all providers.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error)
}
