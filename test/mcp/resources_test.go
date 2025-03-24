// Package mcp provides test utilities for MCP protocol testing.
package mcp

import (
	"strings"
	"testing"

	"github.com/cowgnition/cowgnition/test/helpers"
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

			validateResourceObject(t, resource)
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

		// Validate response structure
		validateResourceResponseStructure(t, resp)

		// If authenticated, test task resources
		if helpers.IsAuthenticated(client) {
			// Test reading tasks resource
			resp, err := client.ReadResource(t, "tasks://all")
			if err != nil {
				t.Fatalf("Failed to read tasks resource: %v", err)
			}

			// Validate response structure
			validateResourceResponseStructure(t, resp)

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

// validateResourceObject validates an individual resource object structure.
func validateResourceObject(t *testing.T, resource map[string]interface{}) {
	t.Helper()

	// Check required fields
	required := []string{"name", "description"}
	for _, field := range required {
		val, ok := resource[field].(string)
		if !ok {
			t.Errorf("Resource missing required field: %s", field)
		} else if val == "" {
			t.Errorf("Resource has empty %s", field)
		}
	}

	// Check name format (scheme://path or scheme://path/{param})
	name, _ := resource["name"].(string)
	if !strings.Contains(name, "://") {
		t.Errorf("Resource name does not follow scheme://path format: %s", name)
	}

	// Check arguments if present
	if args, ok := resource["arguments"].([]interface{}); ok {
		for i, arg := range args {
			argObj, ok := arg.(map[string]interface{})
			if !ok {
				t.Errorf("Resource argument %d is not an object", i)
				continue
			}

			// Check required argument fields
			argRequired := []string{"name", "description"}
			for _, field := range argRequired {
				if _, ok := argObj[field].(string); !ok {
					t.Errorf("Resource argument %d missing required field: %s", i, field)
				}
			}

			// Check required flag is a boolean
			if _, ok := argObj["required"].(bool); !ok {
				t.Errorf("Resource argument %d required field is not a boolean", i)
			}
		}
	}
}

// validateResourceResponseStructure validates a read_resource response.
func validateResourceResponseStructure(t *testing.T, response map[string]interface{}) {
	t.Helper()

	// Check required fields
	if _, ok := response["content"].(string); !ok {
		t.Error("Resource response missing content field")
	}

	mimeType, ok := response["mime_type"].(string)
	if !ok {
		t.Error("Resource response missing mime_type field")
	} else if mimeType == "" {
		t.Error("Resource response has empty mime_type")
	} else {
		// Common MIME types for MCP resources
		validMimeTypes := map[string]bool{
			"text/plain":       true,
			"text/markdown":    true,
			"text/html":        true,
			"application/json": true,
		}

		if !validMimeTypes[mimeType] && !strings.Contains(mimeType, "/") {
			t.Errorf("Resource response has invalid mime_type: %s", mimeType)
		}
	}
}
