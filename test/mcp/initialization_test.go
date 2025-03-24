// Package mcp provides test utilities for MCP protocol testing.
package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/test/helpers"
)

// Initialization verifies the MCP initialization protocol flow.
func Initialization(t *testing.T, client *helpers.MCPClient) {
	t.Helper()

	// Test initialization with valid parameters
	t.Run("ValidInitialization", func(t *testing.T) {
		_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Send initialization request
		resp, err := client.Initialize(t, "Test Client", "1.0.0")
		if err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}

		// Check server_info field
		serverInfo, ok := resp["server_info"].(map[string]interface{})
		if !ok {
			t.Error("Response missing server_info field")
		} else {
			// Validate server_info structure
			validateServerInfoStructure(t, serverInfo)
		}

		// Check capabilities field
		capabilities, ok := resp["capabilities"].(map[string]interface{})
		if !ok {
			t.Error("Response missing capabilities field")
		} else {
			// Validate capabilities structure
			validateCapabilitiesStructure(t, capabilities)
		}
	})

	// Test initialization with minimal parameters
	t.Run("MinimalInitialization", func(t *testing.T) {
		resp, err := client.Initialize(t, "", "")
		if err != nil {
			t.Fatalf("Failed to initialize with minimal params: %v", err)
		}

		// Even with minimal params, response should have required fields
		if _, ok := resp["server_info"].(map[string]interface{}); !ok {
			t.Error("Response missing server_info field with minimal params")
		}
		if _, ok := resp["capabilities"].(map[string]interface{}); !ok {
			t.Error("Response missing capabilities field with minimal params")
		}
	})
}

// validateServerInfoStructure validates the server_info object structure.
func validateServerInfoStructure(t *testing.T, serverInfo map[string]interface{}) {
	t.Helper()

	// Check name field
	name, ok := serverInfo["name"].(string)
	if !ok || name == "" {
		t.Error("server_info missing or empty name field")
	}

	// Check version field
	version, ok := serverInfo["version"].(string)
	if !ok || version == "" {
		t.Error("server_info missing or empty version field")
	}
}

// validateCapabilitiesStructure validates the capabilities object structure.
func validateCapabilitiesStructure(t *testing.T, capabilities map[string]interface{}) {
	t.Helper()

	// Required capabilities
	requiredCapabilities := []string{
		"resources",
		"tools",
	}

	for _, capName := range requiredCapabilities {
		cap, ok := capabilities[capName].(map[string]interface{})
		if !ok {
			t.Errorf("capabilities missing required capability: %s", capName)
			continue
		}

		// Validate resources capability
		if capName == "resources" {
			validateResourcesCapabilityStructure(t, cap)
		}

		// Validate tools capability
		if capName == "tools" {
			validateToolsCapabilityStructure(t, cap)
		}
	}

	// Optional capabilities (validate if present)
	optionalCapabilities := []string{
		"logging",
		"prompts",
	}

	for _, capName := range optionalCapabilities {
		if cap, ok := capabilities[capName].(map[string]interface{}); ok {
			// If present, validate structure
			for key, val := range cap {
				if _, ok := val.(bool); !ok {
					t.Errorf("capabilities.%s.%s is not a boolean", capName, key)
				}
			}
		}
	}
}

// validateResourcesCapabilityStructure validates the resources capability structure.
func validateResourcesCapabilityStructure(t *testing.T, resources map[string]interface{}) {
	t.Helper()

	// Required operations
	requiredOps := []string{
		"list",
		"read",
	}

	for _, op := range requiredOps {
		val, ok := resources[op].(bool)
		if !ok {
			t.Errorf("resources.%s is not a boolean", op)
		} else if !val {
			t.Errorf("resources.%s should be true for a conformant server", op)
		}
	}

	// Optional capabilities can be true or false.
	optionalFields := []string{"subscribe", "listChanged"}
	for _, field := range optionalFields {
		if val, ok := resources[field].(bool); ok {
			// Just check that it's a boolean, we don't enforce true/false.
			_ = val
		} else if resources[field] != nil {
			// If it exists but isn't a boolean, that's an error.
			t.Errorf("resources.%s is not a boolean", field)
		}
	}
}

// validateToolsCapabilityStructure validates the tools capability structure.
func validateToolsCapabilityStructure(t *testing.T, tools map[string]interface{}) {
	t.Helper()

	// Required operations
	requiredOps := []string{
		"list",
		"call",
	}

	for _, op := range requiredOps {
		val, ok := tools[op].(bool)
		if !ok {
			t.Errorf("tools.%s is not a boolean", op)
		} else if !val {
			t.Errorf("tools.%s should be true for a conformant server", op)
		}
	}

	// Optional capabilities can be true or false.
	optionalFields := []string{"listChanged"}
	for _, field := range optionalFields {
		if val, ok := tools[field].(bool); ok {
			// Just check that it's a boolean, we don't enforce true/false.
			_ = val
		} else if tools[field] != nil {
			// If it exists but isn't a boolean, that's an error.
			t.Errorf("tools.%s is not a boolean", field)
		}
	}
}
