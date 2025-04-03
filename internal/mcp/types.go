// internal/mcp/connection/types.go
package connection

import (
	"context"

	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
)

// Define log levels once in this file to avoid redeclaration
type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
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

// InitializeRequest represents the MCP initialize request structure.
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

// ServerInfo represents the server information structure.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResponse represents the MCP initialize response structure.
type InitializeResponse struct {
	ServerInfo      ServerInfo             `json:"server_info"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ProtocolVersion string                 `json:"protocolVersion"` // Protocol version
}

// ListResourcesResponse represents the MCP list_resources response structure.
type ListResourcesResponse struct {
	Resources []definitions.ResourceDefinition `json:"resources"`
}

// ResourceResponse represents the MCP read_resource response structure.
type ResourceResponse struct {
	Content  string `json:"content"`
	MimeType string `json:"mime_type"`
}

// ListToolsResponse represents the MCP list_tools response structure.
type ListToolsResponse struct {
	Tools []definitions.ToolDefinition `json:"tools"`
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
