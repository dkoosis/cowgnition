// Package mcp provides test utilities for MCP protocol testing.
package mcp

import (
	"strings"
	"testing"

	"github.com/cowgnition/cowgnition/test/helpers"
)

// Tools verifies the MCP tool listing and calling capabilities.
func Tools(t *testing.T, client *helpers.MCPClient) {
	t.Helper()

	// Test tool listing
	t.Run("ListTools", func(t *testing.T) {
		resp, err := client.ListTools(t)
		if err != nil {
			t.Fatalf("Failed to list tools: %v", err)
		}

		// Validate response structure
		tools, ok := resp["tools"].([]interface{})
		if !ok {
			t.Fatalf("Response missing tools array: %v", resp)
		}

		// A conformant server should have at least one tool
		if len(tools) == 0 {
			t.Error("No tools returned from list_tools")
		}

		// Validate tool structures
		for i, toolItem := range tools {
			tool, ok := toolItem.(map[string]interface{})
			if !ok {
				t.Errorf("Tool %d is not an object", i)
				continue
			}

			validateToolObject(t, tool)
		}

		// Check for expected authentication tools
		authenticateToolFound := false
		authStatusToolFound := false

		for _, toolItem := range tools {
			tool, ok := toolItem.(map[string]interface{})
			if !ok {
				continue
			}

			name, ok := tool["name"].(string)
			if !ok {
				continue
			}

			if name == "authenticate" {
				authenticateToolFound = true
			} else if name == "auth_status" {
				authStatusToolFound = true
			}
		}

		// Authentication tool should be available
		if !authenticateToolFound && !helpers.IsAuthenticated(client) {
			t.Error("authenticate tool not found when not authenticated")
		}

		// Auth status tool should be available when authenticated
		if helpers.IsAuthenticated(client) && !authStatusToolFound {
			t.Error("auth_status tool not found when authenticated")
		}
	})

	// Test tool calling (only for safe tools)
	t.Run("CallTool", func(t *testing.T) {
		// If authenticated, test auth_status tool
		if helpers.IsAuthenticated(client) {
			resp, err := client.CallTool(t, "auth_status", map[string]interface{}{})
			if err != nil {
				t.Fatalf("Failed to call auth_status tool: %v", err)
			}

			// Validate response structure
			result, ok := resp["result"].(string)
			if !ok {
				t.Error("Tool response missing result field")
			} else if result == "" {
				t.Error("Tool response has empty result")
			} else {
				// Result should mention authentication status
				if !strings.Contains(strings.ToLower(result), "status") {
					t.Error("auth_status result doesn't mention status")
				}
			}
		}
	})

	// Test tool validation
	t.Run("ToolValidation", func(t *testing.T) {
		// Ensure nonexistent tools are properly handled
		resp, err := client.CallTool(t, "nonexistent_tool", map[string]interface{}{})

		// Should return an error for nonexistent tool
		if err == nil {
			t.Error("Calling nonexistent tool should fail")
		}

		// Or return an appropriate error response if err is nil
		if err == nil && resp != nil {
			if _, ok := resp["error"]; !ok {
				t.Error("Error response for nonexistent tool missing error field")
			}
		}
	})
}

// validateToolObject validates an individual tool object structure.
func validateToolObject(t *testing.T, tool map[string]interface{}) {
	t.Helper()

	// Check required fields
	required := []string{"name", "description"}
	for _, field := range required {
		val, ok := tool[field].(string)
		if !ok {
			t.Errorf("Tool missing required field: %s", field)
		} else if val == "" {
			t.Errorf("Tool has empty %s", field)
		}
	}

	// Check arguments if present
	if args, ok := tool["arguments"].([]interface{}); ok {
		for i, arg := range args {
			argObj, ok := arg.(map[string]interface{})
			if !ok {
				t.Errorf("Tool argument %d is not an object", i)
				continue
			}

			// Check required argument fields
			argRequired := []string{"name", "description"}
			for _, field := range argRequired {
				if _, ok := argObj[field].(string); !ok {
					t.Errorf("Tool argument %d missing required field: %s", i, field)
				}
			}

			// Check required flag is a boolean
			if _, ok := argObj["required"].(bool); !ok {
				t.Errorf("Tool argument %d required field is not a boolean", i)
			}
		}
	}
}
