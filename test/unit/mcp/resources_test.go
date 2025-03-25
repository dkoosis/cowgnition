// Package mcp provides unit tests for MCP protocol implementation.
// file: test/unit/mcp/resources_test.go
package mcp

import (
	"strings"
	"testing"

	"github.com/cowgnition/cowgnition/test/helpers/common"
	validators "github.com/cowgnition/cowgnition/test/validators/mcp"
)

// TestResources verifies the MCP resource listing and reading capabilities.
// This test ensures that resource listing and reading functionalities of the MCP server are working as expected.
func TestResources(t *testing.T) {
	// Create client for testing
	// A new MCP client is created for each test to ensure isolation.
	client := common.NewMCPClient(t, nil)
	defer client.Close()

	// Run the implementation
	testResourcesImpl(t, client)
}

// testResourcesImpl contains the actual test implementation.
// It encapsulates the core logic for testing MCP resource operations.
func testResourcesImpl(t *testing.T, client *common.MCPClient) {
	t.Helper()

	// Test resource listing
	t.Run("ListResources", func(t *testing.T) {
		resp, err := client.ListResources(t)
		if err != nil {
			t.Fatalf("Failed to list resources: %v", err)
		}

		// Validate response structure
		// Checks if the response contains the 'resources' key with an array of interfaces.
		resources, ok := resp["resources"].(interface{})
		if !ok {
			t.Fatalf("Response missing resources array: %v", resp)
		}

		// A conformant server should have at least one resource
		// Verifies that the server returns at least one resource in the list.
		if len(resources) == 0 {
			t.Error("No resources returned from list_resources")
		}

		// Validate resource structures
		// Iterates through each resource in the list to validate its structure.
		for i, res := range resources {
			resource, ok := res.(map[string]interface{})
			if !ok {
				t.Errorf("Resource %d is not an object", i)
				continue
			}

			// Use centralized validator
			// Calls a validator function to check the resource structure against expected schema.
			if !validators.ValidateMCPResource(t, resource) {
				t.Errorf("Resource %d failed validation", i)
			}
		}

		// Test for specific expected resources
		// These flags track the presence of specific resources ("auth://rtm" and "tasks://all").
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
		// Checks if the "auth://rtm" resource is present in the listed resources.
		if !authResourceFound {
			t.Error("auth://rtm resource not found in list_resources")
		}

		// tasks://all should be available when authenticated
		// Checks if the "tasks://all" resource is present when the client is authenticated.
		if common.IsAuthenticated(client) && !tasksAllResourceFound {
			t.Error("tasks://all resource not found when authenticated")
		}
	})

	// Test resource reading
	t.Run("ReadResource", func(t *testing.T) {
		// Test the auth resource which should always be available
		// Reads the "auth://rtm" resource to verify resource reading functionality.
		resp, err := client.ReadResource(t, "auth://rtm")
		if err != nil {
			t.Fatalf("Failed to read auth resource: %v", err)
		}

		// Validate response structure using centralized validator
		// Validates the structure of the "auth://rtm" resource response.
		if !validators.ValidateResourceResponse(t, resp) {
			t.Error("Auth resource response failed validation")
		}

		// If authenticated, test task resources
		// Only executes task resource tests if the client is authenticated.
		if common.IsAuthenticated(client) {
			// Test reading tasks resource
			// Reads the "tasks://all" resource.
			resp, err := client.ReadResource(t, "tasks://all")
			if err != nil {
				t.Fatalf("Failed to read tasks resource: %v", err)
			}

			// Validate response structure using centralized validator
			// Validates the structure of the "tasks://all" resource response.
			if !validators.ValidateResourceResponse(t, resp) {
				t.Error("Tasks resource response failed validation")
			}

			// Validate task-specific content
			// Checks if the content of the "tasks://all" resource is not empty and contains the word "task".
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
		// Attempts to read a nonexistent resource to verify error handling.
		resp, err := client.ReadResource(t, "nonexistent://resource")

		// Should return an error for nonexistent resource
		// Checks if reading a nonexistent resource returns an error.
		if err == nil {
			t.Error("Reading nonexistent resource should fail")
		}

		// Or return an appropriate error response if err is nil
		// If no error is returned, checks for an error field in the response.
		if err == nil && resp != nil {
			if _, ok := resp["error"]; !ok {
				t.Error("Error response for nonexistent resource missing error field")
			}
		}
	})
}

// DocEnhanced: 2024-12-20
