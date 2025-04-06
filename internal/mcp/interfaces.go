// file: internal/mcp/interfaces.go
// Package mcp defines interfaces and structures for managing resources and tools
// according to the Model Context Protocol (MCP).
// These interfaces MUST align with the MCP specification to ensure compliance.
// Terminate all comments with a period.
package mcp

import (
	"context"

	// Import the corrected definitions package.
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
)

// ResourceProvider defines an interface for components that provide MCP resources.
type ResourceProvider interface {
	// GetResourceDefinitions returns the list of resources this provider handles.
	// The returned structs MUST conform to definitions.Resource, derived from the MCP spec.
	GetResourceDefinitions() []definitions.Resource // Changed return type.

	// ReadResource attempts to read the content of a resource identified by its URI.
	// The URI identifies the specific resource instance to read.
	// Returns a definitions.ReadResourceResult containing the resource contents,
	// or an error if reading fails. The result structure MUST conform to the MCP spec.
	ReadResource(ctx context.Context, uri string) (definitions.ReadResourceResult, error) // Changed signature: uri input, complex result output.
}

// ToolProvider defines an interface for components that provide MCP tools.
type ToolProvider interface {
	// GetToolDefinitions returns the list of tools this provider handles.
	// The returned structs MUST conform to definitions.ToolDefinition, derived from the MCP spec.
	GetToolDefinitions() []definitions.ToolDefinition // Ensure this uses the corrected definition struct.

	// CallTool attempts to execute a tool with the given name and arguments.
	// Returns a definitions.CallToolResult containing the tool's output or error status,
	// or a Go error for protocol/internal failures. The result structure MUST conform to the MCP spec.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (definitions.CallToolResult, error) // Changed return type.
}

// ResourceManager defines the interface for managing resource providers.
type ResourceManager interface {
	// RegisterProvider registers a ResourceProvider.
	RegisterProvider(provider ResourceProvider)

	// GetAllResourceDefinitions returns all resource definitions from all registered providers.
	// The returned structs MUST conform to definitions.Resource.
	GetAllResourceDefinitions() []definitions.Resource // Changed return type.

	// ReadResource finds the appropriate provider and reads a resource identified by its URI.
	// Returns a definitions.ReadResourceResult or an error.
	ReadResource(ctx context.Context, uri string) (definitions.ReadResourceResult, error) // Changed signature.
}

// ToolManager defines the interface for managing tool providers.
type ToolManager interface {
	// RegisterProvider registers a ToolProvider.
	RegisterProvider(provider ToolProvider)

	// GetAllToolDefinitions returns all tool definitions from all registered providers.
	// The returned structs MUST conform to definitions.ToolDefinition.
	GetAllToolDefinitions() []definitions.ToolDefinition // Ensure this uses the corrected definition struct.

	// CallTool finds the appropriate provider and calls a tool by name with arguments.
	// Returns a definitions.CallToolResult or an error.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (definitions.CallToolResult, error) // Changed return type.
}
