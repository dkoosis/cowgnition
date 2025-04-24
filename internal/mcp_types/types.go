// Package mcptypes defines shared types and interfaces for the MCP
// server and middleware components. This file contains core data structures
// used across different packages to prevent import cycles.
// file: internal/mcp_types/types.go
package mcptypes

import (
	"encoding/json"
)

// --- Core MCP Data Structures ---.

// Implementation describes the name and version of an MCP client or server.
// Matches definition in schema.json: Implementation.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientCapabilities describes features supported by the client.
// Matches definition in schema.json: ClientCapabilities.
type ClientCapabilities struct {
	Roots    *RootsCapability    `json:"roots,omitempty"`
	Sampling *SamplingCapability `json:"sampling,omitempty"`
	// Add experimental or other capabilities if needed.
}

// RootsCapability indicates client support for filesystem roots.
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// SamplingCapability indicates client support for LLM sampling requests.
type SamplingCapability struct {
	// Add specific sampling capability fields if defined by the schema/protocol.
}

// ServerCapabilities describes features supported by the server.
// Matches definition in schema.json: ServerCapabilities.
type ServerCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Prompts   *PromptsCapability   `json:"prompts,omitempty"`
	Logging   *LoggingCapability   `json:"logging,omitempty"`
	// Add experimental or other capabilities if needed.
}

// ToolsCapability indicates server support for tools.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability indicates server support for resources.
type ResourcesCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
	Subscribe   bool `json:"subscribe,omitempty"`
}

// PromptsCapability indicates server support for prompts.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// LoggingCapability indicates server support for logging.
type LoggingCapability struct {
	// Add specific logging capability fields if defined.
}

// InitializeRequest represents the parameters for the 'initialize' request.
// Matches definition in schema.json: InitializeRequest.
type InitializeRequest struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ClientInfo      Implementation     `json:"clientInfo"`
	Capabilities    ClientCapabilities `json:"capabilities"`
}

// InitializeResult represents the successful result of an 'initialize' request.
// Matches definition in schema.json: InitializeResult.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ServerInfo      *Implementation    `json:"serverInfo"` // Pointer based on handler_core usage.
	Capabilities    ServerCapabilities `json:"capabilities"`
	Instructions    string             `json:"instructions,omitempty"`
}

// Tool represents a tool that the server offers to the client.
// Used by services.Service interface and mcp package.
type Tool struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	InputSchema json.RawMessage  `json:"inputSchema"`
	Annotations *ToolAnnotations `json:"annotations,omitempty"`
}

// ToolAnnotations contains additional information about a tool.
// Used by Tool struct.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint,omitempty"`
	IdempotentHint  bool   `json:"idempotentHint,omitempty"`
	OpenWorldHint   bool   `json:"openWorldHint,omitempty"`
	DestructiveHint bool   `json:"destructiveHint,omitempty"`
}

// ListToolsResult represents the successful result of a 'tools/list' request.
// Matches definition in schema.json: ListToolsResult.
type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// CallToolRequest represents the parameters for the 'tools/call' request.
// Matches definition in schema.json: CallToolRequest.
type CallToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"` // Keep as RawMessage, specific args parsed by tool handler.
}

// CallToolResult represents the result of a tool call.
// Used by services.Service interface and mcp package.
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Resource represents a resource that the server offers to the client.
// Used by services.Service interface and mcp package.
type Resource struct {
	Name        string `json:"name"`
	URI         string `json:"uri"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	// Annotations could be added here if needed based on schema evolution.
}

// ListResourcesResult represents the successful result of a 'resources/list' request.
// Matches definition in schema.json: ListResourcesResult.
type ListResourcesResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

// ReadResourceRequest represents the parameters for the 'resources/read' request.
// Matches definition in schema.json: ReadResourceRequest.
type ReadResourceRequest struct {
	URI string `json:"uri"`
}

// ReadResourceResult represents the successful result of a 'resources/read' request.
// Matches definition in schema.json: ReadResourceResult.
type ReadResourceResult struct {
	Contents []interface{} `json:"contents"` // Can contain TextResourceContents, BlobResourceContents, etc.
}

// ResourceContents represents the base contents of a resource.
// Used by ReadResource results.
type ResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
}

// TextResourceContents represents the contents of a text resource.
// Used by ReadResource results.
type TextResourceContents struct {
	ResourceContents
	Text string `json:"text"`
}

// BlobResourceContents represents the contents of a binary resource.
// Used by ReadResource results.
type BlobResourceContents struct {
	ResourceContents
	Blob string `json:"blob"` // Base64 encoded string.
}

// PromptArgument describes an argument for a prompt template.
// Matches definition in schema.json: PromptArgument.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Prompt represents a prompt template offered by the server.
// Matches definition in schema.json: Prompt.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// ListPromptsResult represents the successful result of a 'prompts/list' request.
// Matches definition in schema.json: ListPromptsResult.
type ListPromptsResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

// --- Content Types ---.

// Content represents a content item in a message.
// This is an interface fulfilled by specific content types like TextContent.
// Used by CallToolResult.
type Content interface {
	GetType() string
}

// --- JSON-RPC Error Structures ---

// JSONRPCErrorPayload represents the 'error' object in a JSON-RPC error response.
// Matches the structure defined in the JSON-RPC 2.0 specification.
type JSONRPCErrorPayload struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"` // Data is optional
}

// JSONRPCErrorContainer represents the full JSON-RPC error response object.
// Matches the structure defined in the JSON-RPC 2.0 specification.
type JSONRPCErrorContainer struct {
	JSONRPC string              `json:"jsonrpc"` // Should always be "2.0"
	Error   JSONRPCErrorPayload `json:"error"`
	ID      json.RawMessage     `json:"id"` // Can be string, number, or null (but MCP spec requires non-null)
}

// TextContent represents a text content item.
// Implements the Content interface.
type TextContent struct {
	Type string `json:"type"` // Should always be "text".
	Text string `json:"text"`
}

// GetPromptRequest defines the parameters for the prompts/get request.
type GetPromptRequest struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

// GetType returns the type of content ("text").
func (t TextContent) GetType() string {
	return "text"
}

// --- Add this struct definition to internal/mcp_types/types.go ---

// PromptMessage represents a single message within a prompt response.
// Matches definition in schema.json: PromptMessage.
type PromptMessage struct {
	Role    string    `json:"role"`    // "user" or "assistant"
	Content []Content `json:"content"` // Can contain TextContent, ImageContent, EmbeddedResource
}

// GetPromptResult represents the successful result of a 'prompts/get' request.
// Matches definition in schema.json: GetPromptResult.
type GetPromptResult struct {
	Messages    []PromptMessage `json:"messages"`
	Description string          `json:"description,omitempty"`
	// _meta field could be added if needed, but often omitted in Go structs unless used.
}

// --- Ensure Content types (TextContent, etc.) are also defined or imported if needed by PromptMessage ---
// TextContent is already defined, add others if used in prompts.

// NOTE: Other content types like ImageContent, EmbeddedResource would also go here.
// if they were directly used by CallToolResult or other shared types. Add them.
// as needed based on schema requirements and usage. Example:

/*
// ImageContent represents an image content item.
// Implements the Content interface.
type ImageContent struct {
	Type     string `json:"type"` // Should always be "image".
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // Base64 encoded image data.
}

// GetType returns the type of content ("image").
func (i ImageContent) GetType() string {
	return "image"
}

// EmbeddedResource represents embedded resource content.
// Implements the Content interface.
type EmbeddedResource struct {
    Type     string      `json:"type"` // Should always be "resource".
    Resource interface{} `json:"resource"` // Contains TextResourceContents or BlobResourceContents.
    // Annotations can be added here if needed.
}

// GetType returns the type of content ("resource").
func (e EmbeddedResource) GetType() string {
    return "resource"
}
*/
