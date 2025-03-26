// internal/mcp/types.go
package mcp

// InitializeRequest represents the MCP initialize request structure.
type InitializeRequest struct {
	ServerName    string `json:"server_name"`
	ServerVersion string `json:"server_version"`
}

// ServerInfo represents the server information structure.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResponse represents the MCP initialize response structure.
type InitializeResponse struct {
	ServerInfo   ServerInfo             `json:"server_info"`
	Capabilities map[string]interface{} `json:"capabilities"`
}

// ResourceDefinition represents an MCP resource structure.
type ResourceDefinition struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Arguments   []ResourceArgument `json:"arguments,omitempty"`
}

// ResourceArgument represents an argument for a resource.
type ResourceArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ListResourcesResponse represents the MCP list_resources response structure.
type ListResourcesResponse struct {
	Resources []ResourceDefinition `json:"resources"`
}

// ResourceResponse represents the MCP read_resource response structure.
type ResourceResponse struct {
	Content  string `json:"content"`
	MimeType string `json:"mime_type"`
}

// ToolDefinition represents an MCP tool structure.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Arguments   []ToolArgument `json:"arguments,omitempty"`
}

// ToolArgument represents an argument for a tool.
type ToolArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ListToolsResponse represents the MCP list_tools response structure.
type ListToolsResponse struct {
	Tools []ToolDefinition `json:"tools"`
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
