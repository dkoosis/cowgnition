// file: internal/mcp/types.go
package mcp

import (
	// Import the new definitions package
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
)

// InitializeRequest represents the MCP initialize request structure.
// (Remains here as it's a top-level protocol message)
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
// (Remains here)
type InitializeResponse struct {
	ServerInfo      ServerInfo             `json:"server_info"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ProtocolVersion string                 `json:"protocolVersion"` // Protocol version
}

// ServerInfo represents the server information structure.
// (Remains here)
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// --- ResourceDefinition MOVED to definitions/types.go ---
// --- ResourceArgument MOVED to definitions/types.go ---

// ListResourcesResponse represents the MCP list_resources response structure.
// (Remains here, but references definitions.ResourceDefinition)
type ListResourcesResponse struct {
	Resources []definitions.ResourceDefinition `json:"resources"` // <-- Updated type reference
}

// ResourceResponse represents the MCP read_resource response structure.
// (Remains here as it defines the structure of the response payload)
type ResourceResponse struct {
	Content  string `json:"content"`
	MimeType string `json:"mime_type"`
}

// --- ToolDefinition MOVED to definitions/types.go ---
// --- ToolArgument MOVED to definitions/types.go ---

// ListToolsResponse represents the MCP list_tools response structure.
// (Remains here, but references definitions.ToolDefinition)
type ListToolsResponse struct {
	Tools []definitions.ToolDefinition `json:"tools"` // <-- Updated type reference
}

// CallToolRequest represents the MCP call_tool request structure.
// (Remains here)
type CallToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolResponse represents the MCP call_tool response structure.
// (Remains here)
type ToolResponse struct {
	Result string `json:"result"`
}
