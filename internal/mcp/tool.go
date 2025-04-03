// Package mcp handles the Model Context Protocol (MCP) server functionality.
// file: internal/mcp/tool.go
package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// ToolManagerImpl manages all registered tool providers.
type ToolManagerImpl struct {
	providers []ToolProvider
}

// NewToolManager creates a new tool manager.
func NewToolManager() ToolManager {
	return &ToolManagerImpl{
		providers: []ToolProvider{},
	}
}

// RegisterProvider registers a ToolProvider.
func (tm *ToolManagerImpl) RegisterProvider(provider ToolProvider) {
	tm.providers = append(tm.providers, provider)
}

// GetAllToolDefinitions returns all tool definitions from all providers.
func (tm *ToolManagerImpl) GetAllToolDefinitions() []definitions.ToolDefinition {
	var allTools []definitions.ToolDefinition
	for _, provider := range tm.providers {
		allTools = append(allTools, provider.GetToolDefinitions()...)
	}
	return allTools
}

// FindToolProvider finds the provider for a specific tool name.
func (tm *ToolManagerImpl) FindToolProvider(name string) (ToolProvider, error) {
	for _, provider := range tm.providers {
		for _, tool := range provider.GetToolDefinitions() {
			if tool.Name == name {
				return provider, nil
			}
		}
	}

	// Get all available tool names for better error context
	var availableTools []string
	for _, provider := range tm.providers {
		for _, tool := range provider.GetToolDefinitions() {
			availableTools = append(availableTools, tool.Name)
		}
	}

	return nil, cgerr.NewToolError(
		fmt.Sprintf("tool '%s' not found", name),
		nil,
		map[string]interface{}{
			"tool_name":       name,
			"available_tools": availableTools,
		},
	)
}

// CallTool calls a tool across all providers.
func (tm *ToolManagerImpl) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	provider, err := tm.FindToolProvider(name)
	if err != nil {
		return "", errors.Wrap(err, "failed to find tool provider")
	}

	// Capture the start time for timing information
	startTime := time.Now()

	// Check for context cancellation or deadline
	if ctx.Err() != nil {
		return "", cgerr.NewTimeoutError(
			fmt.Sprintf("context ended before executing tool '%s'", name),
			map[string]interface{}{
				"tool_name":     name,
				"context_error": ctx.Err().Error(),
			},
		)
	}

	result, err := provider.CallTool(ctx, name, args)
	if err != nil {
		return "", cgerr.NewToolError(
			fmt.Sprintf("failed to execute tool '%s'", name),
			err,
			map[string]interface{}{
				"tool_name":      name,
				"args":           args,
				"operation_time": time.Since(startTime).String(),
			},
		)
	}

	return result, nil
}
