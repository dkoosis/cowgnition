// file: internal/mcp/mcp_definitions/types.go
package definitions

// LogLevel represents the severity of a log message.
type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
)

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

// ToolDefinition represents an MCP tool structure.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// ServerInfo represents server information.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeRequest represents the MCP initialize request structure.
type InitializeRequest struct {
	ProtocolVersion string `json:"protocolVersion"`
	ClientInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo"`
	Capabilities map[string]interface{} `json:"capabilities"`
	// Legacy fields
	ServerName    string `json:"server_name,omitempty"`
	ServerVersion string `json:"server_version,omitempty"`
}

// InitializeResponse represents the MCP initialize response structure.
type InitializeResponse struct {
	ServerInfo      ServerInfo             `json:"server_info"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ProtocolVersion string                 `json:"protocolVersion"`
}

// ListResourcesResponse represents the response for listing resources.
type ListResourcesResponse struct {
	Resources []ResourceDefinition `json:"resources"`
}

// ResourceResponse represents the response for a resource read operation.
type ResourceResponse struct {
	Content  string `json:"content"`
	MimeType string `json:"mime_type"`
}

// ListToolsResponse represents the response for listing tools.
type ListToolsResponse struct {
	Tools []ToolDefinition `json:"tools"`
}

// CallToolRequest represents a tool call request.
type CallToolRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolResponse represents a tool call response.
type ToolResponse struct {
	Result string `json:"result"`
}
