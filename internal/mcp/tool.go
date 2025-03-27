// internal/mcp/tool.go
package mcp

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// ToolProvider defines an interface for components that provide MCP tools.
type ToolProvider interface {
	// GetToolDefinitions returns the list of tools this provider handles.
	GetToolDefinitions() []ToolDefinition

	// CallTool attempts to execute a tool with the given name and arguments.
	// Returns the result of the tool execution and any error encountered.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error)
}

// ToolManager manages all registered tool providers.
type ToolManager struct {
	providers []ToolProvider
}

// NewToolManager creates a new tool manager.
func NewToolManager() *ToolManager {
	return &ToolManager{
		providers: []ToolProvider{},
	}
}

// RegisterProvider registers a ToolProvider.
func (tm *ToolManager) RegisterProvider(provider ToolProvider) {
	tm.providers = append(tm.providers, provider)
}

// GetAllToolDefinitions returns all tool definitions from all providers.
func (tm *ToolManager) GetAllToolDefinitions() []ToolDefinition {
	var allTools []ToolDefinition
	for _, provider := range tm.providers {
		allTools = append(allTools, provider.GetToolDefinitions()...)
	}
	return allTools
}

// FindToolProvider finds the provider for a specific tool name.
func (tm *ToolManager) FindToolProvider(name string) (ToolProvider, error) {
	for _, provider := range tm.providers {
		for _, tool := range provider.GetToolDefinitions() {
			if tool.Name == name {
				return provider, nil
			}
		}
	}

	return nil, cgerr.NewToolError(
		fmt.Sprintf("tool '%s' not found", name),
		nil,
		map[string]interface{}{"tool_name": name},
	)
}

// CallTool calls a tool across all providers.
func (tm *ToolManager) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	provider, err := tm.FindToolProvider(name)
	if err != nil {
		return "", errors.Wrap(err, "failed to find tool provider")
	}

	result, err := provider.CallTool(ctx, name, args)
	if err != nil {
		return "", cgerr.NewToolError(
			fmt.Sprintf("failed to execute tool '%s'", name),
			err,
			map[string]interface{}{
				"tool_name": name,
				"args":      args,
			},
		)
	}

	return result, nil
}
