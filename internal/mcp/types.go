// file: internal/mcp/types.go
package mcp

import (
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
)

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
