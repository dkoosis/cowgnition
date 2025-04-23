package mcptypes

import (
	"context"
	"encoding/json"
)

// --- Core Interfaces ---.

// MessageHandler defines the function signature for processing a single MCP message.
// Implementations receive the message bytes and should return response bytes or an error.
// This type is used as the core processing unit in the server and middleware chain.
type MessageHandler func(ctx context.Context, message []byte) ([]byte, error)

// MiddlewareFunc defines the signature for middleware functions.
// A middleware function takes the next MessageHandler in the chain and returns
// a new MessageHandler that typically performs some action before or after
// calling the next handler. This allows for composing layers of functionality.
type MiddlewareFunc func(handler MessageHandler) MessageHandler

// Chain defines an interface for building and managing a sequence of middleware functions
// that culminate in a final MessageHandler.
type Chain interface {
	// Use adds a MiddlewareFunc to the chain. Middlewares are typically executed
	// in the reverse order they are added.
	Use(middleware MiddlewareFunc) Chain

	// Handler finalizes the chain and returns the composed MessageHandler.
	// Once called, the chain should generally not be modified further.
	Handler() MessageHandler
}

// ValidatorInterface defines the core methods required for validating messages
// against a loaded schema. This allows different schema validation implementations
// to be used interchangeably by the middleware.
type ValidatorInterface interface {
	// Validate checks if the provided data conforms to the schema definition
	// associated with the given messageType (e.g., MCP method name).
	Validate(ctx context.Context, messageType string, data []byte) error
	// HasSchema checks if a compiled schema definition exists for the given name.
	HasSchema(name string) bool
	// IsInitialized returns true if the validator has successfully loaded and
	// compiled the necessary schema definitions.
	IsInitialized() bool
}

// --- Configuration Structs ---.

// ValidationOptions holds configuration settings for the validation middleware.
// These options control whether validation is enabled, how strict it is,
// and whether performance should be measured.
type ValidationOptions struct {
	// Enabled controls whether validation is performed at all. Defaults to true.
	Enabled bool
	// StrictMode, if true, causes validation failures to immediately return a
	// JSON-RPC error response. If false, errors are logged, but processing may continue.
	StrictMode bool
	// ValidateOutgoing determines whether responses sent by the server should be validated.
	ValidateOutgoing bool
	// StrictOutgoing, if true, causes invalid outgoing messages to be replaced
	// with an internal server error response. If false, errors are logged,
	// but the potentially invalid message is still sent.
	StrictOutgoing bool
	// MeasurePerformance enables logging of validation duration for performance analysis.
	MeasurePerformance bool
	// SkipTypes maps message method names (e.g., "ping") to true if incoming
	// validation should be skipped for that specific method.
	SkipTypes map[string]bool
}

// --- MCP Protocol Types ---.

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

// ImageContent represents an image content item.
type ImageContent struct {
	Type     string `json:"type"`
	Data     string `json:"data"`     // Base64 encoded image data.
	MimeType string `json:"mimeType"` // Mime type of the image.
}

// GetType returns the type of content.
func (i ImageContent) GetType() string {
	return "image"
}

// EmbeddedResource represents embedded resource content.
type EmbeddedResource struct {
	Type     string           `json:"type"`
	Resource ResourceContents `json:"resource"` // Contains URI, MimeType, Text/Blob.
}

// GetType returns the type of content.
func (e EmbeddedResource) GetType() string {
	return "resource"
}

// Resource represents a resource that the server offers to the client.
type Resource struct {
	Name        string `json:"name"`
	URI         string `json:"uri"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	// Annotations can be added here if needed based on schema evolution.
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

// ResourceContents represents the base contents of a resource (URI, MimeType).
type ResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
}

// TextResourceContents represents the contents of a text resource.
type TextResourceContents struct {
	ResourceContents
	Text string `json:"text"`
}

// BlobResourceContents represents the contents of a binary resource.
type BlobResourceContents struct {
	ResourceContents
	Blob string `json:"blob"` // Base64 encoded string.
}

// ReadResourceResult represents the result of a resources/read request.
// It contains a slice of interface{} to accommodate different content types
// (TextResourceContents, BlobResourceContents).
type ReadResourceResult struct {
	Contents []interface{} `json:"contents"`
}

// Prompt represents a prompt or prompt template that the server offers.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument describes an argument that a prompt can accept.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ListPromptsResult represents the result of a prompts/list request.
type ListPromptsResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

// GetPromptResult represents the result of a prompts/get request.
type GetPromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// PromptMessage describes a message returned as part of a prompt.
type PromptMessage struct {
	Role    string  `json:"role"`
	Content Content `json:"content"` // Uses the Content interface.
}

// CompleteResult represents the result of a completion/complete request.
type CompleteResult struct {
	Completion struct {
		Values  []string `json:"values"`
		Total   int      `json:"total,omitempty"`
		HasMore bool     `json:"hasMore,omitempty"`
	} `json:"completion"`
}
