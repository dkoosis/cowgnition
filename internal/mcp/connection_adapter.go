// file: internal/mcp/connection_adapter.go
// Package mcp contains core MCP server logic, including adapters for connection management.
// Terminate all comments with a period.
package mcp

import (
	"context"
	// Needed for fmt.Sprintf in adapter methods.
	// Import connection package which defines the contracts these adapters implement.
	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/mcp/connection"

	// Import corrected definitions.
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
)

// Helper function to safely create a pointer to a boolean true value.
//
//nolint:unused
func ptrBoolTrue() *bool { b := true; return &b }

// ConnectWithStateManager connects the Server to a state-machine-based connection manager.
// It configures the connection manager and sets up adapters to bridge the main MCP
// interfaces (ResourceManager, ToolManager) with the contracts expected by the connection manager.
func (s *Server) ConnectWithStateManager() error {
	// Create server configuration using corrected capabilities structure.
	// This configuration is passed to the connection manager.
	config := connection.ServerConfig{
		Name:            s.config.GetServerName(),
		Version:         s.version,
		RequestTimeout:  s.requestTimeout,
		ShutdownTimeout: s.shutdownTimeout,
		// Create a server capabilities struct that properly matches the MCP spec
		Capabilities: map[string]interface{}{
			"resources": map[string]interface{}{
				"listChanged": true,
				"subscribe":   true,
			},
			"tools": map[string]interface{}{
				"listChanged": true,
			},
		},
	}

	// Create resource manager adapter.
	// This adapts the main ResourceManager to the connection.ResourceManagerContract.
	resourceAdapter := &resourceManagerAdapter{
		rm: s.resourceManager,
	}

	// Create tool manager adapter.
	// This adapts the main ToolManager to the connection.ToolManagerContract.
	toolAdapter := &toolManagerAdapter{
		tm: s.toolManager,
	}

	// Create connection manager using the factory function from the connection package.
	// Pass the config and the adapters.
	_, err := connection.NewConnectionServerFactory(config, resourceAdapter, toolAdapter)
	if err != nil {
		// Wrap error for context.
		return errors.Wrapf(err, "ConnectWithStateManager: failed to create connection manager.")
	}

	return nil
}

// resourceManagerAdapter adapts the mcp.ResourceManager interface to the
// connection.ResourceManagerContract interface expected by the connection manager.
type resourceManagerAdapter struct {
	// Holds a reference to the actual ResourceManager implementation.
	rm ResourceManager
}

// GetAllResourceDefinitions implements connection.ResourceManagerContract.
// It calls the underlying ResourceManager and returns the result.
// Signature updated to return []definitions.Resource.
func (a *resourceManagerAdapter) GetAllResourceDefinitions() []definitions.Resource {
	// Assuming a.rm.GetAllResourceDefinitions signature matches updated ResourceManager interface.
	return a.rm.GetAllResourceDefinitions()
}

// ReadResource implements connection.ResourceManagerContract.
// It calls the underlying ResourceManager's ReadResource method.
// Signature updated to accept uri string and return definitions.ReadResourceResult.
func (a *resourceManagerAdapter) ReadResource(ctx context.Context, uri string) (definitions.ReadResourceResult, error) {
	// Call the underlying manager with the corrected signature.
	// Assuming a.rm.ReadResource signature matches updated ResourceManager interface.
	return a.rm.ReadResource(ctx, uri)
}

// toolManagerAdapter adapts the mcp.ToolManager interface to the
// connection.ToolManagerContract interface expected by the connection manager.
type toolManagerAdapter struct {
	// Holds a reference to the actual ToolManager implementation.
	tm ToolManager
}

// GetAllToolDefinitions implements connection.ToolManagerContract.
// It calls the underlying ToolManager and returns the result.
// Signature updated to return []definitions.ToolDefinition.
func (a *toolManagerAdapter) GetAllToolDefinitions() []definitions.ToolDefinition {
	// Assuming a.tm.GetAllToolDefinitions signature matches updated ToolManager interface.
	return a.tm.GetAllToolDefinitions()
}

// CallTool implements connection.ToolManagerContract.
// It calls the underlying ToolManager's CallTool method.
// Signature updated to return definitions.CallToolResult.
func (a *toolManagerAdapter) CallTool(ctx context.Context, name string, args map[string]interface{}) (definitions.CallToolResult, error) {
	// Call the underlying manager with the corrected signature.
	// Assuming a.tm.CallTool signature matches updated ToolManager interface.
	return a.tm.CallTool(ctx, name, args)
}
