// file: internal/mcp/types.go
package mcp

import (
	"encoding/json"
)

// Implementation defines the name and version of an MCP implementation.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientCapabilities defines the capabilities that a client may support.
type ClientCapabilities struct {
	Sampling     *struct{}                  `json:"sampling,omitempty"`
	Roots        *struct{}                  `json:"roots,omitempty"`
	Experimental map[string]json.RawMessage `json:"experimental,omitempty"`
}

// ServerCapabilities defines the capabilities that the server supports.
type ServerCapabilities struct {
	Prompts      *PromptsCapability         `json:"prompts,omitempty"`
	Resources    *ResourcesCapability       `json:"resources,omitempty"`
	Tools        *ToolsCapability           `json:"tools,omitempty"`
	Logging      map[string]interface{}     `json:"logging,omitempty"`
	Experimental map[string]json.RawMessage `json:"experimental,omitempty"`
}

// PromptsCapability indicates that the server offers prompt templates.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged"`
}

// ResourcesCapability indicates that the server offers resources.
type ResourcesCapability struct {
	ListChanged bool `json:"listChanged"`
	Subscribe   bool `json:"subscribe"`
}

// ToolsCapability indicates that the server offers tools.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

// InitializeRequest represents the request sent by a client to initialize the connection.
type InitializeRequest struct {
	ClientInfo      Implementation     `json:"clientInfo"`
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
}

// InitializeResult represents the server's response to an initialize request.
type InitializeResult struct {
	ServerInfo      Implementation     `json:"serverInfo"`
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	Instructions    string             `json:"instructions,omitempty"`
}

// Tool represents a tool that the server offers to the client.
type Tool struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	InputSchema json.RawMessage  `json:"inputSchema"`
	Annotations *ToolAnnotations `json:"annotations,omitempty"`
}

// ToolAnnotations contains additional information about a tool.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint,omitempty"`
	IdempotentHint  bool   `json:"idempotentHint,omitempty"`
	OpenWorldHint   bool   `json:"openWorldHint,omitempty"`
	DestructiveHint bool   `json:"destructiveHint,omitempty"`
}

// ListToolsResult represents the result of a tools/list request.
type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// CallToolRequest represents a request to call a tool.
type CallToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// CallToolResult represents the result of a tool call.
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content represents a content item in a message.
type Content interface {
	GetType() string
}

// TextContent represents a text content item.
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// GetType returns the type of content.
func (t TextContent) GetType() string {
	return "text"
}

// Resource represents a resource that the server offers to the client.
type Resource struct {
	Name        string `json:"name"`
	URI         string `json:"uri"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ListResourcesResult represents the result of a resources/list request.
type ListResourcesResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

// ReadResourceRequest represents a request to read a resource.
type ReadResourceRequest struct {
	URI string `json:"uri"`
}

// ResourceContents represents the contents of a resource.
type ResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
}

// TextResourceContents represents the contents of a text resource.
type TextResourceContents struct {
	ResourceContents
	Text string `json:"text"`
}

// ReadResourceResult represents the result of a resources/read request.
type ReadResourceResult struct {
	Contents []interface{} `json:"contents"`
}
