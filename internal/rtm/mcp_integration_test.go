// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/mcp_integration_test.go

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRTMToolsIntegration tests the integration between MCP tools and RTM service.
func TestRTMToolsIntegration(t *testing.T) {
	// Skip test if integration tests are not requested
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a logger for tests
	logger := logging.GetNoopLogger()

	// Load configuration
	cfg := config.DefaultConfig()
	if cfg.RTM.APIKey == "" || cfg.RTM.SharedSecret == "" {
		t.Skip("Skipping RTM integration tests: API key or shared secret not configured")
	}

	// Create RTM service
	rtmService := NewService(cfg, logger)
	ctx := context.Background()

	// Initialize the service
	err := rtmService.Initialize(ctx)
	require.NoError(t, err, "Failed to initialize RTM service")

	// Check authentication state
	if !rtmService.IsAuthenticated() {
		t.Skip("Skipping authenticated RTM tests: not authenticated")
	}

	// Test GetTools
	t.Run("GetTools", func(t *testing.T) {
		tools := rtmService.GetTools()
		assert.NotEmpty(t, tools, "RTM service should provide tools")

		// Verify required tools exist
		foundGetTasks := false
		foundCreateTask := false

		for _, tool := range tools {
			switch tool.Name {
			case "getTasks":
				foundGetTasks = true
			case "createTask":
				foundCreateTask = true
			}
		}

		assert.True(t, foundGetTasks, "RTM service should provide getTasks tool")
		assert.True(t, foundCreateTask, "RTM service should provide createTask tool")
	})

	// Test CallTool - getTasks
	t.Run("CallTool_GetTasks", func(t *testing.T) {
		// Create getTasks arguments
		args := map[string]interface{}{
			"filter": "status:incomplete",
		}
		argsBytes, err := json.Marshal(args)
		require.NoError(t, err, "Failed to marshal getTasks arguments")

		// Call the tool
		result, err := rtmService.CallTool(ctx, "getTasks", argsBytes)
		require.NoError(t, err, "CallTool getTasks returned error")
		require.NotNil(t, result, "CallTool getTasks returned nil result")

		// Verify result structure
		assert.False(t, result.IsError, "getTasks should not return an error result")
		assert.NotEmpty(t, result.Content, "getTasks should return content")
	})

	// Test GetResources
	t.Run("GetResources", func(t *testing.T) {
		resources := rtmService.GetResources()
		assert.NotEmpty(t, resources, "RTM service should provide resources")

		// Verify required resources exist
		foundAuthResource := false
		foundListsResource := false

		for _, resource := range resources {
			switch resource.URI {
			case "rtm://auth":
				foundAuthResource = true
			case "rtm://lists":
				foundListsResource = true
			}
		}

		assert.True(t, foundAuthResource, "RTM service should provide auth resource")
		assert.True(t, foundListsResource, "RTM service should provide lists resource")
	})

	// Test ReadResource - auth status
	t.Run("ReadResource_Auth", func(t *testing.T) {
		// Read the auth resource
		content, err := rtmService.ReadResource(ctx, "rtm://auth")
		require.NoError(t, err, "ReadResource auth returned error")
		require.NotEmpty(t, content, "ReadResource auth returned empty content")

		// Verify that content contains authentication information
		textContent, ok := content[0].(mcp.TextResourceContents)
		assert.True(t, ok, "Content should be TextResourceContents")
		assert.Contains(t, textContent.Text, "isAuthenticated", "Auth resource should contain authentication status")
	})

	// Test ReadResource - lists
	t.Run("ReadResource_Lists", func(t *testing.T) {
		// Read the lists resource
		content, err := rtmService.ReadResource(ctx, "rtm://lists")
		require.NoError(t, err, "ReadResource lists returned error")
		require.NotEmpty(t, content, "ReadResource lists returned empty content")

		// Verify that content contains list information
		textContent, ok := content[0].(mcp.TextResourceContents)
		assert.True(t, ok, "Content should be TextResourceContents")
		assert.Contains(t, textContent.Text, "id", "Lists resource should contain list IDs")
		assert.Contains(t, textContent.Text, "name", "Lists resource should contain list names")
	})
}
