// Package mcp provides type definitions and utilities for the Model Context Protocol.
package mcp

// ResourceDefinition represents an MCP resource definition exposed by the server.
type ResourceDefinition struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Arguments   []ResourceArgument `json:"arguments"`
}

// ResourceArgument represents a parameter for an MCP resource.
type ResourceArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ToolDefinition represents an MCP tool definition exposed by the server.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Arguments   []ToolArgument `json:"arguments"`
}

// ToolArgument represents a parameter for an MCP tool.
type ToolArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ResourceResponse represents the response to a read_resource request.
type ResourceResponse struct {
	Content  string `json:"content"`
	MimeType string `json:"mime_type"`
}

// ToolResponse represents the response to a call_tool request.
type ToolResponse struct {
	Result string `json:"result"`
}

// InitializeRequest represents the request for server initialization.
type InitializeRequest struct {
	ServerName    string `json:"server_name"`
	ServerVersion string `json:"server_version"`
}

// InitializeResponse represents the response to server initialization.
type InitializeResponse struct {
	ServerInfo   ServerInfo             `json:"server_info"`
	Capabilities map[string]interface{} `json:"capabilities"`
}

// ServerInfo provides information about the MCP server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ListResourcesResponse represents the response to a list_resources request.
type ListResourcesResponse struct {
	Resources []ResourceDefinition `json:"resources"`
}

// ListToolsResponse represents the response to a list_tools request.
type ListToolsResponse struct {
	Tools []ToolDefinition `json:"tools"`
}

// CallToolRequest represents a request to call a tool.
type CallToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}
