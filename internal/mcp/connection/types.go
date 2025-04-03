// file: internal/mcp/connection/types.go

// Package connection handles the state management and communication logic
// for a single client connection using the MCP protocol.
// This file primarily contains interface definitions required by the connection package.
package connection

import (
	"context"
	// Import the definitions package to use specific MCP types
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
)

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
