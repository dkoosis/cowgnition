// test/mcp/resources_test.go
// CONSOLIDATION-2024-03-24: Updated to use centralized validators from test/helpers/mcp

package mcp

import (
	"strings"
	"testing"

	"github.com/cowgnition/cowgnition/test/helpers"
	mcpvalidators "github.com/cowgnition/cowgnition/test/helpers/mcp"
)

// Resources verifies the MCP resource listing and reading capabilities.
func Resources(t *testing.T, client *helpers.MCPClient) {
	t.Helper()

	// Test resource listing
	t.Run("ListResources", func(t *testing.T) {
		resp, err := client.ListResources(t)
		if err != nil {
			t.Fatalf("Failed to list resources: %v", err)
		}

		// Validate response structure
		resources, ok := resp["resources"].([]interface{})
		if !ok {
			t.Fatalf("Response missing resources array: %v", resp)
		}

		// A conformant server should have at least one resource
		if len(resources) == 0 {
			t.Error("No resources returned from list_resources")
		}

		// Validate resource structures
		for i, res := range resources {
			resource, ok := res.(map[string]interface{})
			if !ok {
				t.Errorf("Resource %d is not an object", i)
				continue
			}

			// Use centralized validator
			if !mcpvalidators.ValidateMCPResource(t, resource) {
				t.Errorf("Resource %d failed validation", i)
			}
		}

		// Test for specific expected resources
		authResourceFound := false
		tasksAllResourceFound := false

		for _, res := range resources {
			resource, ok := res.(map[string]interface{})
			if !ok {
				continue
			}

			name, ok := resource["name"].(string)
			if !ok {
				continue
			}

			if name == "auth://rtm" {
				authResourceFound = true
			} else if name == "tasks://all" {
				tasksAllResourceFound = true
			}
		}

		// Auth resource should always be available
		if !authResourceFound {
			t.Error("auth://rtm resource not found in list_resources")
		}

		// tasks://all should be available when authenticated
		if helpers.IsAuthenticated(client) && !tasksAllResourceFound {
			t.Error("tasks://all resource not found when authenticated")
		}
	})

	// Test resource reading
	t.Run("ReadResource", func(t *testing.T) {
		// Test the auth resource which should always be available
		resp, err := client.ReadResource(t, "auth://rtm")
		if err != nil {
			t.Fatalf("Failed to read auth resource: %v", err)
		}

		// Validate response structure using centralized validator
		if !mcpvalidators.ValidateResourceResponse(t, resp) {
			t.Error("Auth resource response failed validation")
		}

		// If authenticated, test task resources
		if helpers.IsAuthenticated(client) {
			// Test reading tasks resource
			resp, err := client.ReadResource(t, "tasks://all")
			if err != nil {
				t.Fatalf("Failed to read tasks resource: %v", err)
			}

			// Validate response structure using centralized validator
			if !mcpvalidators.ValidateResourceResponse(t, resp) {
				t.Error("Tasks resource response failed validation")
			}

			// Validate task-specific content
			content, ok := resp["content"].(string)
			if !ok || content == "" {
				t.Error("Tasks resource returned empty content")
			} else {
				// Tasks content should mention tasks
				if !strings.Contains(strings.ToLower(content), "task") {
					t.Error("Tasks resource content doesn't mention tasks")
				}
			}
		}
	})

	// Test resource validation
	t.Run("ResourceValidation", func(t *testing.T) {
		// Ensure nonexistent resources are properly handled
		resp, err := client.ReadResource(t, "nonexistent://resource")

		// Should return an error for nonexistent resource
		if err == nil {
			t.Error("Reading nonexistent resource should fail")
		}

		// Or return an appropriate error response if err is nil
		if err == nil && resp != nil {
			if _, ok := resp["error"]; !ok {
				t.Error("Error response for nonexistent resource missing error field")
			}
		}
	})
}
