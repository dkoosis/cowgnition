// internal/mcp/connection/types.go
package connection

import (
	"context"

	"github.com/dkoosis/cowgnition/internal/mcp"
)

// InitializeRequest represents the MCP initialize request structure.
// Importing from parent package or redefining here for internal use
type InitializeRequest struct {
	ProtocolVersion string `json:"protocolVersion"` // Protocol version
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"` // Client information
	Capabilities map[string]interface{} `json:"capabilities"` // Client capabilities
	// Legacy fields
	ServerName    string `json:"server_name,omitempty"`    // Optional in newer MCP spec versions
	ServerVersion string `json:"server_version,omitempty"` // Optional in newer MCP spec versions
}

// InitializeResponse represents the MCP initialize response structure.
type InitializeResponse struct {
	ServerInfo      ServerInfo             `json:"server_info"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ProtocolVersion string                 `json:"protocolVersion"` // Protocol version
}

// ServerInfo represents the server information structure.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ListResourcesResponse represents the MCP list_resources response structure.
type ListResourcesResponse struct {
	Resources []mcp.ResourceDefinition `json:"resources"`
}

// ResourceResponse represents the MCP read_resource response structure.
type ResourceResponse struct {
	Content  string `json:"content"`
	MimeType string `json:"mime_type"`
}

// ListToolsResponse represents the MCP list_tools response structure.
type ListToolsResponse struct {
	Tools []mcp.ToolDefinition `json:"tools"`
}

// CallToolRequest represents the MCP call_tool request structure.
type CallToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolResponse represents the MCP call_tool response structure.
type ToolResponse struct {
	Result string `json:"result"`
}

// ResourceManager interface represents the contract for resource management.
type ResourceManager interface {
	// GetAllResourceDefinitions returns all resource definitions.
	GetAllResourceDefinitions() []mcp.ResourceDefinition

	// ReadResource reads a resource with the given name and arguments.
	ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error)
}

// ToolManager interface represents the contract for tool management.
type ToolManager interface {
	// GetAllToolDefinitions returns all tool definitions.
	GetAllToolDefinitions() []mcp.ToolDefinition

	// CallTool calls a tool with the given name and arguments.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error)
}
