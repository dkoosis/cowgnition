// file: internal/mcp/tool.go
// Package mcp handles the Model Context Protocol (MCP) server functionality.
// This file implements the ToolManager.
package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	// Use the corrected definitions package.
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// ToolManagerImpl manages all registered tool providers.
type ToolManagerImpl struct {
	providers []ToolProvider
}

// NewToolManager creates a new tool manager.
func NewToolManager() ToolManager {
	// This function now correctly returns a ToolManager interface type.
	// The compiler error was in the CallTool implementation below.
	return &ToolManagerImpl{
		providers: []ToolProvider{},
	}
}

// RegisterProvider registers a ToolProvider.
func (tm *ToolManagerImpl) RegisterProvider(provider ToolProvider) {
	tm.providers = append(tm.providers, provider)
}

// GetAllToolDefinitions returns all tool definitions from all providers.
// It now correctly returns a slice of the potentially updated definitions.ToolDefinition.
func (tm *ToolManagerImpl) GetAllToolDefinitions() []definitions.ToolDefinition {
	var allTools []definitions.ToolDefinition
	for _, provider := range tm.providers {
		// Assuming provider.GetToolDefinitions() signature matches the updated ToolProvider interface.
		allTools = append(allTools, provider.GetToolDefinitions()...)
	}
	return allTools
}

// FindToolProvider finds the provider for a specific tool name.
// No changes needed here, but relies on providers returning correct ToolDefinition.
func (tm *ToolManagerImpl) FindToolProvider(name string) (ToolProvider, error) {
	for _, provider := range tm.providers {
		// Assuming provider.GetToolDefinitions() signature matches the updated ToolProvider interface.
		for _, tool := range provider.GetToolDefinitions() {
			if tool.Name == name {
				return provider, nil
			}
		}
	}

	// Get all available tool names for better error context.
	var availableTools []string
	for _, provider := range tm.providers {
		// Assuming provider.GetToolDefinitions() signature matches the updated ToolProvider interface.
		for _, tool := range provider.GetToolDefinitions() {
			availableTools = append(availableTools, tool.Name)
		}
	}

	return nil, cgerr.NewToolError(
		fmt.Sprintf("tool '%s' not found.", name), // Added period.
		nil,
		map[string]interface{}{
			"tool_name":       name,
			"available_tools": availableTools,
		},
	)
}

// CallTool calls a tool across all providers.
// Signature updated to return definitions.CallToolResult and error to match ToolManager interface.
func (tm *ToolManagerImpl) CallTool(ctx context.Context, name string, args map[string]interface{}) (definitions.CallToolResult, error) {
	emptyResult := definitions.CallToolResult{} // Helper for error returns.

	provider, err := tm.FindToolProvider(name)
	if err != nil {
		// Error from FindToolProvider is already detailed. Wrap for context.
		return emptyResult, errors.Wrapf(err, "CallTool: failed to find provider for tool '%s'.", name)
	}

	// Capture the start time for timing information.
	startTime := time.Now()
	providerType := fmt.Sprintf("%T", provider)

	// Check for context cancellation or deadline *before* calling the provider.
	if ctxErr := ctx.Err(); ctxErr != nil {
		timeoutErr := cgerr.NewTimeoutError(
			fmt.Sprintf("CallTool: context ended before executing tool '%s'.", name),
			map[string]interface{}{
				"tool_name":     name,
				"context_error": ctxErr.Error(),
			},
		)
		// Return the Go error directly, handler will convert to JSON-RPC error.
		return emptyResult, timeoutErr
	}

	// Call the provider's CallTool method, which now returns CallToolResult.
	// Assuming provider.CallTool signature matches the updated ToolProvider interface.
	result, err := provider.CallTool(ctx, name, args)
	if err != nil {
		// If the provider returned a Go error (e.g., network issue, internal error).
		// Wrap it as a ToolError. The handler will convert this to a JSON-RPC error.
		toolErr := cgerr.NewToolError(
			fmt.Sprintf("CallTool: failed to execute tool '%s'.", name),
			err, // Wrap the original error.
			map[string]interface{}{
				"tool_name":      name,
				"args":           args,
				"provider_type":  providerType,
				"operation_time": time.Since(startTime).String(),
			},
		)
		return emptyResult, toolErr
	}

	// If the provider returned successfully (nil Go error), the 'result' contains
	// the CallToolResult structure, which might internally indicate a tool-specific
	// error via its 'IsError' field. The handler in handlers.go needs to check this.
	// We simply return the result struct here.
	return result, nil
}
