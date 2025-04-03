// file: internal/mcp/connection/types.go
package connection

import (
	"context"
	// Import the new definitions package for shared types
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	// --- REMOVE the import for the parent mcp package ---
	// "github.com/dkoosis/cowgnition/internal/mcp"
)

// --- NOTE ---
// Request/Response struct definitions (like InitializeRequest, InitializeResponse,
// ListResourcesResponse, ResourceResponse, ListToolsResponse, CallToolRequest,
// ToolResponse, ServerInfo) should be defined in the parent package's
// `internal/mcp/types.go` file, not duplicated here.

// ResourceManager interface represents the contract for resource management
// required by the ConnectionManager. It uses types from the definitions package.
type ResourceManager interface {
	// GetAllResourceDefinitions returns all available resource definitions.
	GetAllResourceDefinitions() []definitions.ResourceDefinition // <-- Use definitions.

	// ReadResource reads the content and determines the mime type of a resource.
	// Return types match the implementation expected (string content, string mimeType).
	ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error)
}

// ToolManager interface represents the contract for tool management
// required by the ConnectionManager. It uses types from the definitions package.
type ToolManager interface {
	// GetAllToolDefinitions returns all available tool definitions.
	GetAllToolDefinitions() []definitions.ToolDefinition // <-- Use definitions.

	// CallTool executes a tool and returns its result as a string.
	// Return type matches the implementation expected (string result).
	CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error)
}

// Add any other types or interfaces here that are TRULY SPECIFIC
// only to the internal workings of the 'connection' package.
// Based on the files shown previously, only the interfaces seem necessary here.
