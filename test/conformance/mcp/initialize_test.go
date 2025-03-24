// Package conformance provides tests to verify MCP protocol compliance.
// file: test/conformance/mcp_initialize_endpoint_test.go
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/helpers/common"
	"github.com/cowgnition/cowgnition/test/mocks/common"
)

// TestMCPInitializeEndpointEnhanced tests the /mcp/initialize endpoint with more
// thorough validation of the MCP protocol requirements.
func TestMCPInitializeEndpointEnhanced(t *testing.T) {
	// Create a test configuration.
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name: "Test MCP Server",
			Port: 8080,
		},
		RTM: config.RTMConfig{
			APIKey:       "test_key",
			SharedSecret: "test_secret",
		},
		Auth: config.AuthConfig{
			TokenPath: t.TempDir() + "/token",
		},
	}

	// Create a mock RTM server.
	rtmMock := mocks.NewRTMServer(t)
	defer rtmMock.Close()

	// Override RTM API endpoint in client.
	if err := os.Setenv("RTM_API_ENDPOINT", rtmMock.BaseURL); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer os.Unsetenv("RTM_API_ENDPOINT")

	// Create and start the MCP server.
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	s.SetVersion("test-version")

	// Create MCP test client.
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// Test cases for initialization.
	testCases := []struct {
		name          string
		serverName    string
		serverVersion string
		wantStatus    int
		wantError     bool
	}{
		{
			name:          "Valid initialization",
			serverName:    "Test Client",
			serverVersion: "1.0.0",
			wantStatus:    http.StatusOK,
			wantError:     false,
		},
		{
			name:          "Empty server name",
			serverName:    "",
			serverVersion: "1.0.0",
			wantStatus:    http.StatusOK, // Should still accept this.
			wantError:     false,
		},
		{
			name:          "Empty server version",
			serverName:    "Test Client",
			serverVersion: "",
			wantStatus:    http.StatusOK, // Should still accept this.
			wantError:     false,
		},
		{
			name:          "Very long server name",
			serverName:    strings.Repeat("Long", 100), // 400 character name.
			serverVersion: "1.0.0",
			wantStatus:    http.StatusOK, // Should handle long names.
			wantError:     false,
		},
		{
			name:          "Non-standard version format",
			serverName:    "Test Client",
			serverVersion: "dev.build.123+456",
			wantStatus:    http.StatusOK, // Should accept non-standard versions.
			wantError:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Send initialization request.
			reqBody := map[string]interface{}{
				"server_name":    tc.serverName,
				"server_version": tc.serverVersion,
			}
			body, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/mcp/initialize", strings.NewReader(string(body)))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Client.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			// Verify status code.
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, tc.wantStatus)
			}

			// Parse response.
			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				if !tc.wantError {
					t.Fatalf("Failed to decode response: %v", err)
				}
				return
			}

			// Validate response structure.
			// 1. Verify server_info is present and correct.
			validateServerInfo(t, result, cfg.Server.Name)

			// 2. Verify capabilities structure follows the MCP specification.
			validateCapabilities(t, result)
		})
	}

	// Test with malformed JSON.
	t.Run("Malformed JSON", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		malformedJSON := `{"server_name": "Test", "server_version": "1.0.0", invalid_json}`
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/mcp/initialize", strings.NewReader(malformedJSON))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Should return an error status.
		if resp.StatusCode == http.StatusOK {
			t.Errorf("Expected error status for malformed JSON, got %d", resp.StatusCode)
		}
	})

	// Test with wrong HTTP method.
	t.Run("Wrong HTTP Method", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.BaseURL+"/mcp/initialize", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Should return an error status - initialize should only accept POST.
		if resp.StatusCode == http.StatusOK {
			t.Errorf("Expected error status for wrong HTTP method, got %d", resp.StatusCode)
		}
	})
}

// validateServerInfo verifies that the server_info field in the initialization response
// conforms to the MCP specification.
func validateServerInfo(t *testing.T, result map[string]interface{}, expectedName string) {
	t.Helper()

	if result["server_info"] == nil {
		t.Error("Response missing server_info field")
		return
	}

	serverInfo, ok := result["server_info"].(map[string]interface{})
	if !ok {
		t.Error("server_info is not an object")
		return
	}

	// Check required fields.
	if serverInfo["name"] == nil {
		t.Error("server_info missing required field: name")
	}

	if serverInfo["version"] == nil {
		t.Error("server_info missing required field: version")
	}

	// Validate field types.
	name, ok := serverInfo["name"].(string)
	if !ok {
		t.Error("server_info.name is not a string")
	} else if name != expectedName {
		t.Errorf("server_info.name mismatch: got %v, want %s", name, expectedName)
	}

	version, ok := serverInfo["version"].(string)
	if !ok {
		t.Error("server_info.version is not a string")
	} else if version == "" {
		t.Error("server_info.version cannot be empty")
	}
}

// validateCapabilities verifies that the capabilities field in the initialization response
// conforms to the MCP specification.
func validateCapabilities(t *testing.T, result map[string]interface{}) {
	t.Helper()

	if result["capabilities"] == nil {
		t.Error("Response missing capabilities field")
		return
	}

	capabilities, ok := result["capabilities"].(map[string]interface{})
	if !ok {
		t.Error("capabilities is not an object")
		return
	}

	// Verify required capabilities according to MCP spec.
	// According to the spec, "resources" and "tools" capabilities are required.
	requiredCaps := []string{"resources", "tools"}
	for _, cap := range requiredCaps {
		if capabilities[cap] == nil {
			t.Errorf("Missing required capability: %s", cap)
		}
	}

	// Validate resources capability structure.
	validateResourcesCapability(t, capabilities)

	// Validate tools capability structure.
	validateToolsCapability(t, capabilities)

	// Validate logging capability if present.
	if logging, ok := capabilities["logging"].(map[string]interface{}); ok {
		validateLoggingCapability(t, logging)
	}

	// Validate prompts capability if present.
	if prompts, ok := capabilities["prompts"].(map[string]interface{}); ok {
		validatePromptsCapability(t, prompts)
	}

	// Check for unknown capabilities.
	// The MCP spec defines the allowed capabilities.
	knownCaps := map[string]bool{
		"resources":  true,
		"tools":      true,
		"logging":    true,
		"prompts":    true,
		"completion": true,
	}

	for cap := range capabilities {
		if !knownCaps[cap] {
			t.Logf("Warning: Unknown capability found: %s", cap)
		}
	}
}

// validateResourcesCapability validates the resources capability structure.
func validateResourcesCapability(t *testing.T, capabilities map[string]interface{}) {
	t.Helper()

	resources, ok := capabilities["resources"].(map[string]interface{})
	if !ok {
		t.Error("resources capability is not an object")
		return
	}

	// The resources capability should have at least these fields.
	requiredFields := []string{"list", "read"}
	for _, field := range requiredFields {
		if resources[field] == nil {
			t.Errorf("resources capability missing required field: %s", field)
		}
	}

	// Validate field types - should be boolean values.
	for field, value := range resources {
		if _, ok := value.(bool); !ok {
			t.Errorf("resources.%s is not a boolean", field)
		}
	}

	// list and read should be true for a conformant server.
	if list, ok := resources["list"].(bool); ok && !list {
		t.Error("resources.list capability should be true")
	}

	if read, ok := resources["read"].(bool); ok && !read {
		t.Error("resources.read capability should be true")
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

// validateToolsCapability validates the tools capability structure.
func validateToolsCapability(t *testing.T, capabilities map[string]interface{}) {
	t.Helper()

	tools, ok := capabilities["tools"].(map[string]interface{})
	if !ok {
		t.Error("tools capability is not an object")
		return
	}

	// The tools capability should have at least these fields.
	requiredFields := []string{"list", "call"}
	for _, field := range requiredFields {
		if tools[field] == nil {
			t.Errorf("tools capability missing required field: %s", field)
		}
	}

	// Validate field types - should be boolean values.
	for field, value := range tools {
		if _, ok := value.(bool); !ok {
			t.Errorf("tools.%s is not a boolean", field)
		}
	}

	// list and call should be true for a conformant server.
	if list, ok := tools["list"].(bool); ok && !list {
		t.Error("tools.list capability should be true")
	}

	if call, ok := tools["call"].(bool); ok && !call {
		t.Error("tools.call capability should be true")
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

// validateLoggingCapability validates the logging capability structure.
func validateLoggingCapability(t *testing.T, logging map[string]interface{}) {
	t.Helper()

	// The logging capability should have these fields.
	logFields := []string{"log", "warning", "error"}
	for _, field := range logFields {
		if val, ok := logging[field].(bool); ok {
			// Just check that it's a boolean, we don't enforce true/false.
			_ = val
		} else if logging[field] != nil {
			// If it exists but isn't a boolean, that's an error.
			t.Errorf("logging.%s is not a boolean", field)
		}
	}
}

// validatePromptsCapability validates the prompts capability structure.
func validatePromptsCapability(t *testing.T, prompts map[string]interface{}) {
	t.Helper()

	// The prompts capability should have at least these fields.
	promptFields := []string{"list", "get"}
	for _, field := range promptFields {
		if val, ok := prompts[field].(bool); ok {
			// Just check that it's a boolean, we don't enforce true/false.
			_ = val
		} else if prompts[field] != nil {
			// If it exists but isn't a boolean, that's an error.
			t.Errorf("prompts.%s is not a boolean", field)
		}
	}

	// Optional capabilities.
	if val, ok := prompts["listChanged"].(bool); ok {
		// Just check that it's a boolean.
		_ = val
	} else if prompts["listChanged"] != nil {
		// If it exists but isn't a boolean, that's an error.
		t.Error("prompts.listChanged is not a boolean")
	}
}
