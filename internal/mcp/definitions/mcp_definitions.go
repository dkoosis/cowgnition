// Package definitions contains Go structs mirroring the Model Context Protocol (MCP) specification.
// It is CRITICAL that these structures accurately reflect the types defined in the official MCP schema
// (e.g., https://github.com/modelcontextprotocol/specification/blob/main/schema/2024-11-05/schema.ts)
// to ensure correct parsing of requests and formatting of responses according to the protocol.
// Mismatches will lead to interoperability failures with compliant clients or servers.
// Terminate all comments with a period.
package definitions

const LATEST_PROTOCOL_VERSION = "2024-11-05"

// LogLevel represents the severity of a log message according to RFC-5424.
// It MUST be serialized/parsed as lowercase strings per the MCP spec.
type LogLevel string

// Log level constants MUST match the lowercase strings in the MCP specification.
const (
	LogLevelDebug     LogLevel = "debug"
	LogLevelInfo      LogLevel = "info"
	LogLevelNotice    LogLevel = "notice"
	LogLevelWarning   LogLevel = "warning"
	LogLevelError     LogLevel = "error"
	LogLevelCritical  LogLevel = "critical"
	LogLevelAlert     LogLevel = "alert"
	LogLevelEmergency LogLevel = "emergency"
)

// Role represents the sender or recipient of messages.
// It MUST be serialized/parsed as lowercase strings per the MCP spec.
type Role string

// Role constants MUST match the lowercase strings in the MCP specification.
const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Annotations provides optional metadata for certain MCP objects.
// Conforms to the Annotations type in the MCP spec.
type Annotations struct {
	// Describes who the intended customer of this object or data is.
	Audience []Role `json:"audience,omitempty"`
	// Describes how important this data is (0=least, 1=most).
	Priority *float64 `json:"priority,omitempty"` // Use pointer for optional number.
}

// Resource represents a known resource the server can read.
// Conforms to the Resource type in the MCP spec.
type Resource struct {
	// The URI of this resource. MUST be present.
	URI string `json:"uri"`
	// A human-readable name for this resource. MUST be present.
	Name string `json:"name"`
	// An optional description of what this resource represents.
	Description *string `json:"description,omitempty"`
	// The optional MIME type of this resource.
	MimeType *string `json:"mimeType,omitempty"`
	// The optional size of the raw resource content in bytes.
	Size *int64 `json:"size,omitempty"` // Use int64 for size, pointer for optionality.
	// Optional annotations.
	Annotations *Annotations `json:"annotations,omitempty"`
}

// ResourceTemplate represents a template for dynamic resources.
// Conforms to the ResourceTemplate type in the MCP spec.
type ResourceTemplate struct {
	// A URI template (RFC 6570) for resource URIs. MUST be present.
	URITemplate string `json:"uriTemplate"`
	// A human-readable name for the type of resource. MUST be present.
	Name string `json:"name"`
	// An optional description of what this template is for.
	Description *string `json:"description,omitempty"`
	// An optional MIME type if all matching resources have the same type.
	MimeType *string `json:"mimeType,omitempty"`
	// Optional annotations.
	Annotations *Annotations `json:"annotations,omitempty"`
}

// BaseResourceContents contains fields common to text and blob contents.
// This is a helper struct, not directly in the spec, but represents common fields.
type BaseResourceContents struct {
	// The URI of this resource content. MUST be present.
	URI string `json:"uri"`
	// The optional MIME type of this resource content.
	MimeType *string `json:"mimeType,omitempty"`
}

// TextResourceContents represents text content of a resource.
// Conforms to the TextResourceContents type in the MCP spec.
type TextResourceContents struct {
	BaseResourceContents
	// The text of the item. MUST be present for text resources.
	Text string `json:"text"`
	// Type field to potentially help with unions, though MCP doesn't require it here.
	// Type string `json:"type"` // Spec doesn't mandate type field here, unlike Content blocks.
}

// BlobResourceContents represents binary content of a resource.
// Conforms to the BlobResourceContents type in the MCP spec.
type BlobResourceContents struct {
	BaseResourceContents
	// A base64-encoded string representing the binary data. MUST be present for blob resources.
	Blob string `json:"blob"`
	// Type field to potentially help with unions.
	// Type string `json:"type"` // Spec doesn't mandate type field here.
}

// ResourceContents is used for the ReadResourceResult.Contents array.
// Since Go doesn't have native union types for JSON, we use a struct
// containing pointers to optional fields for text and blob. Only one should be non-nil.
// Alternatively, use interface{} and type assertions, or separate structs with a 'type' field if needed downstream.
// This approach tries to map closely for JSON marshalling.
type ResourceContents struct {
	// The URI of this resource content. MUST be present.
	URI string `json:"uri"`
	// The optional MIME type of this resource content.
	MimeType *string `json:"mimeType,omitempty"`
	// The text content, if this is a text resource. Omit if blob is present.
	Text *string `json:"text,omitempty"`
	// The blob content (base64), if this is a binary resource. Omit if text is present.
	Blob *string `json:"blob,omitempty"`
}

// ToolInputSchema represents the JSON schema for a tool's input parameters.
// Conforms to the inputSchema field structure within the Tool type in the MCP spec.
type ToolInputSchema struct {
	// Type must be "object".
	Type string `json:"type"` // Should always be "object".
	// Optional map defining parameter properties (each value is a JSON schema object).
	Properties map[string]interface{} `json:"properties,omitempty"`
	// Optional list of required parameter names.
	Required []string `json:"required,omitempty"`
}

// ToolDefinition represents an MCP tool structure.
// Conforms to the Tool type in the MCP spec.
type ToolDefinition struct {
	// The name of the tool. MUST be present.
	Name string `json:"name"`
	// An optional human-readable description of the tool.
	Description *string `json:"description,omitempty"`
	// The JSON Schema object defining the expected parameters. MUST be present.
	InputSchema ToolInputSchema `json:"inputSchema"`
}

// Implementation represents the name and version of a client or server.
// Conforms to the Implementation type in the MCP spec.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ClientCapabilities represents capabilities a client may support.
// Conforms to the ClientCapabilities type in the MCP spec.
type ClientCapabilities struct {
	Experimental map[string]interface{} `json:"experimental,omitempty"`
	Roots        *struct {
		ListChanged *bool `json:"listChanged,omitempty"`
	} `json:"roots,omitempty"`
	Sampling map[string]interface{} `json:"sampling,omitempty"` // Spec uses 'object', map is flexible.
}

// ServerCapabilities represents capabilities a server may support.
// Conforms to the ServerCapabilities type in the MCP spec.
type ServerCapabilities struct {
	Experimental map[string]interface{} `json:"experimental,omitempty"`
	Logging      map[string]interface{} `json:"logging,omitempty"` // Spec uses 'object', map is flexible.
	Prompts      *struct {
		ListChanged *bool `json:"listChanged,omitempty"`
	} `json:"prompts,omitempty"`
	Resources *struct {
		Subscribe   *bool `json:"subscribe,omitempty"`
		ListChanged *bool `json:"listChanged,omitempty"`
	} `json:"resources,omitempty"`
	Tools *struct {
		ListChanged *bool `json:"listChanged,omitempty"`
	} `json:"tools,omitempty"`
}

// InitializeRequest represents the structure expected within the 'params' of an initialize request.
// Conforms to the InitializeRequest.params structure in the MCP spec.
type InitializeRequestParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Implementation     `json:"clientInfo"`
}

// InitializeResult represents the data structure for the 'result' field of a successful initialize response.
// Conforms to the InitializeResult type in the MCP spec.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Implementation     `json:"serverInfo"`
	Instructions    *string            `json:"instructions,omitempty"`
}

// ListResourcesResult represents the data structure for the 'result' field of a successful resources/list response.
// Conforms to the ListResourcesResult type in the MCP spec.
type ListResourcesResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor *string    `json:"nextCursor,omitempty"`
}

// ReadResourceResult represents the data structure for the 'result' field of a successful resources/read response.
// Conforms to the ReadResourceResult type in the MCP spec.
type ReadResourceResult struct {
	Contents []ResourceContents     `json:"contents"` // Using the combined struct approach.
	Meta     map[string]interface{} `json:"_meta,omitempty"`
}

// ListToolsResult represents the data structure for the 'result' field of a successful tools/list response.
// Conforms to the ListToolsResult type in the MCP spec.
type ListToolsResult struct {
	Tools      []ToolDefinition `json:"tools"`
	NextCursor *string          `json:"nextCursor,omitempty"`
}

// CallToolRequestParams represents the structure expected within the 'params' of a tools/call request.
// Conforms to the CallToolRequest.params structure in the MCP spec.
type CallToolRequestParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"` // Use interface{} for unknown value types.
}

// --- Tool Call Result Content Types ---

// TextContent represents text content within a tool call result or prompt message.
// Conforms to the TextContent type in the MCP spec.
type TextContent struct {
	Type        string       `json:"type"` // Should be "text".
	Text        string       `json:"text"`
	Annotations *Annotations `json:"annotations,omitempty"`
}

// ImageContent represents image content within a tool call result or prompt message.
// Conforms to the ImageContent type in the MCP spec.
type ImageContent struct {
	Type        string       `json:"type"` // Should be "image".
	Data        string       `json:"data"` // Base64 encoded image data.
	MimeType    string       `json:"mimeType"`
	Annotations *Annotations `json:"annotations,omitempty"`
}

// EmbeddedResource represents an embedded resource within a tool call result or prompt message.
// Conforms to the EmbeddedResource type in the MCP spec.
type EmbeddedResource struct {
	Type        string           `json:"type"`     // Should be "resource".
	Resource    ResourceContents `json:"resource"` // Using the combined struct approach.
	Annotations *Annotations     `json:"annotations,omitempty"`
}

// ToolResultContent is used for the CallToolResult.Content array.
// Similar to ResourceContents, this uses optional fields to simulate a union type for JSON.
// Only one of TextContent, ImageContent, or EmbeddedResource fields should be non-nil,
// identified by the mandatory Type field.
type ToolResultContent struct {
	// Type MUST be "text", "image", or "resource" to indicate which content is present.
	Type string `json:"type"`

	// Text content fields (only if Type == "text").
	Text *string `json:"text,omitempty"`

	// Image content fields (only if Type == "image").
	Data     *string `json:"data,omitempty"`     // Base64 encoded.
	MimeType *string `json:"mimeType,omitempty"` // Mandatory if Type == "image".

	// Resource content fields (only if Type == "resource").
	// We embed the fields of ResourceContents directly here for simplicity,
	// matching the structure { type: "resource", resource: { uri: ..., ... } }.
	// An alternative is a nested struct: Resource *ResourceContents `json:"resource,omitempty"`.
	Resource *ResourceContents `json:"resource,omitempty"` // Pointer to allow omitempty.

	// Annotations are common to all content types.
	Annotations *Annotations `json:"annotations,omitempty"`
}

// CallToolResult represents the data structure for the 'result' field of a successful tools/call response.
// Conforms to the CallToolResult type in the MCP spec.
type CallToolResult struct {
	Content []ToolResultContent    `json:"content"`
	IsError *bool                  `json:"isError,omitempty"`
	Meta    map[string]interface{} `json:"_meta,omitempty"`
}

// --- Prompt Related Definitions ---

// PromptArgument describes an argument a prompt template accepts.
// Conforms to the PromptArgument type in the MCP spec.
type PromptArgument struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Required    *bool   `json:"required,omitempty"`
}

// Prompt represents a prompt template offered by the server.
// Conforms to the Prompt type in the MCP spec.
type Prompt struct {
	Name        string           `json:"name"`
	Description *string          `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptMessage represents a single message within a prompt result.
// Conforms to the PromptMessage type in the MCP spec.
// Note the similarity to ToolResultContent; reusing parts might be possible.
// For clarity, defining separately based on spec structure.
type PromptMessage struct {
	Role    Role              `json:"role"`    // "user" or "assistant".
	Content ToolResultContent `json:"content"` // Reusing ToolResultContent as it covers Text, Image, EmbeddedResource.
}

// ListPromptsResult represents the 'result' field for a prompts/list response.
// Conforms to the ListPromptsResult type in the MCP spec.
type ListPromptsResult struct {
	Prompts    []Prompt `json:"prompts"`
	NextCursor *string  `json:"nextCursor,omitempty"`
}

// GetPromptResult represents the 'result' field for a prompts/get response.
// Conforms to the GetPromptResult type in the MCP spec.
type GetPromptResult struct {
	Description *string                `json:"description,omitempty"`
	Messages    []PromptMessage        `json:"messages"`
	Meta        map[string]interface{} `json:"_meta,omitempty"`
}

// NOTE: This file does not include definitions for all MCP requests/notifications/errors,
// focusing on the response/result types and related definitions discussed.
// A complete implementation would require structs for all message types defined in the spec.
