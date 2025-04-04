// file: internal/mcp/connection_adapter.go
package mcp

import (
	"context"

	"github.com/dkoosis/cowgnition/internal/mcp/connection"
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
)

// ConnectWithStateManager connects the Server to a state-machine-based connection manager.
func (s *Server) ConnectWithStateManager() error {
	// Create server configuration
	config := connection.ServerConfig{
		Name:            s.config.GetServerName(),
		Version:         s.version,
		RequestTimeout:  s.requestTimeout,
		ShutdownTimeout: s.shutdownTimeout,
		Capabilities: map[string]interface{}{
			"resources": map[string]interface{}{
				"list": true,
				"read": true,
			},
			"tools": map[string]interface{}{
				"list": true,
				"call": true,
			},
		},
	}

	// Create resource manager adapter
	resourceAdapter := &resourceManagerAdapter{
		rm: s.resourceManager,
	}

	// Create tool manager adapter
	toolAdapter := &toolManagerAdapter{
		tm: s.toolManager,
	}

	// Create connection manager
	_, err := connection.NewConnectionServer(config, resourceAdapter, toolAdapter)
	if err != nil {
		return err
	}

	return nil
}

// resourceManagerAdapter adapts the mcp.ResourceManager to the connection.ResourceManagerContract.
type resourceManagerAdapter struct {
	rm ResourceManager
}

// GetAllResourceDefinitions implements the connection.ResourceManagerContract.
func (a *resourceManagerAdapter) GetAllResourceDefinitions() []definitions.ResourceDefinition {
	return a.rm.GetAllResourceDefinitions()
}

// ReadResource implements the connection.ResourceManagerContract.
func (a *resourceManagerAdapter) ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) {
	return a.rm.ReadResource(ctx, name, args)
}

// toolManagerAdapter adapts the mcp.ToolManager to the connection.ToolManagerContract.
type toolManagerAdapter struct {
	tm ToolManager
}

// GetAllToolDefinitions implements the connection.ToolManagerContract.
func (a *toolManagerAdapter) GetAllToolDefinitions() []definitions.ToolDefinition {
	return a.tm.GetAllToolDefinitions()
}

// CallTool implements the connection.ToolManagerContract.
func (a *toolManagerAdapter) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	return a.tm.CallTool(ctx, name, args)
}
