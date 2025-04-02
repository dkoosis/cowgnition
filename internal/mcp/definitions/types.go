package definitions

// ResourceDefinition represents an MCP resource structure.
// Moved from internal/mcp/types.go
type ResourceDefinition struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Arguments   []ResourceArgument `json:"arguments,omitempty"`
}

// ResourceArgument represents an argument for a resource.
// Moved from internal/mcp/types.go
type ResourceArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// ToolDefinition represents an MCP tool structure.
// Moved from internal/mcp/types.go
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Arguments   []ToolArgument `json:"arguments,omitempty"`
}

// ToolArgument represents an argument for a tool.
// Moved from internal/mcp/types.go
type ToolArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// Add any other core type definitions here if they are needed by both
// mcp and mcp/connection and were previously causing cycles.
