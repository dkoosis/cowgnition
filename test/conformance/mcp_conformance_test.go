// Package conformance provides tests to verify MCP protocol compliance.
package conformance

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/helpers"
	"github.com/cowgnition/cowgnition/test/mocks"
)

// TestMCPInitializeEndpoint tests the /mcp/initialize endpoint for compliance.
func TestMCPInitializeEndpoint(t *testing.T) {
	// Create a test configuration
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

	// Create a mock RTM server
	rtmMock := mocks.NewRTMServer(t)
	defer rtmMock.Close()

	// Override RTM API endpoint in client
	if err := os.Setenv("RTM_API_ENDPOINT", rtmMock.URL); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer os.Unsetenv("RTM_API_ENDPOINT")

	// Create and start the MCP server
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	s.SetVersion("test-version")

	// Create MCP test client
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// Test cases for initialization
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
			wantStatus:    http.StatusOK, // Should still accept this
			wantError:     false,
		},
		{
			name:          "Empty server version",
			serverName:    "Test Client",
			serverVersion: "",
			wantStatus:    http.StatusOK, // Should still accept this
			wantError:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Send initialization request
			reqBody := map[string]interface{}{
				"server_name":    tc.serverName,
				"server_version": tc.serverVersion,
			}
			body, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			req, err := http.NewRequest(http.MethodPost, client.BaseURL+"/mcp/initialize", strings.NewReader(string(body)))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Client.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			// Verify status code
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, tc.wantStatus)
			}

			// Parse response
			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				if !tc.wantError {
					t.Fatalf("Failed to decode response: %v", err)
				}
				return
			}

			// Verify response structure
			if result["server_info"] == nil {
				t.Error("Response missing server_info field")
			} else {
				serverInfo, ok := result["server_info"].(map[string]interface{})
				if !ok {
					t.Error("server_info is not an object")
				} else {
					if serverInfo["name"] != cfg.Server.Name {
						t.Errorf("server_info.name mismatch: got %v, want %s", serverInfo["name"], cfg.Server.Name)
					}
					if serverInfo["version"] != "test-version" {
						t.Errorf("server_info.version mismatch: got %v, want %s", serverInfo["version"], "test-version")
					}
				}
			}

			// Verify capabilities structure
			if result["capabilities"] == nil {
				t.Error("Response missing capabilities field")
			} else {
				capabilities, ok := result["capabilities"].(map[string]interface{})
				if !ok {
					t.Error("capabilities is not an object")
				} else {
					// Verify required capabilities according to MCP spec
					requiredCaps := []string{"resources", "tools"}
					for _, cap := range requiredCaps {
						if capabilities[cap] == nil {
							t.Errorf("Missing required capability: %s", cap)
						}
					}

					// Verify resources capability structure
					if resCap, ok := capabilities["resources"].(map[string]interface{}); ok {
						if resCap["list"] != true {
							t.Error("resources.list capability should be true")
						}
						if resCap["read"] != true {
							t.Error("resources.read capability should be true")
						}
					} else {
						t.Error("resources capability is not an object")
					}

					// Verify tools capability structure
					if toolsCap, ok := capabilities["tools"].(map[string]interface{}); ok {
						if toolsCap["list"] != true {
							t.Error("tools.list capability should be true")
						}
						if toolsCap["call"] != true {
							t.Error("tools.call capability should be true")
						}
					} else {
						t.Error("tools capability is not an object")
					}
				}
			}
		})
	}
}

// TestMCPResourceEndpoints tests the resource-related endpoints for compliance.
func TestMCPResourceEndpoints(t *testing.T) {
	// Create a test configuration
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

	// Create a mock RTM server with proper responses
	rtmMock := mocks.NewRTMServer(t)

	// Configure the mock to properly handle auth
	rtmMock.AddResponse("rtm.auth.getFrob", `<rsp stat="ok"><frob>test_frob_12345</frob></rsp>`)
	rtmMock.AddResponse("rtm.auth.getToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)
	rtmMock.AddResponse("rtm.auth.checkToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)

	defer rtmMock.Close()

	// Override RTM API endpoint in client
	if err := os.Setenv("RTM_API_ENDPOINT", rtmMock.URL); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer os.Unsetenv("RTM_API_ENDPOINT")

	// Create and start the MCP server
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create MCP test client
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// Test list_resources endpoint
	t.Run("list_resources", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, client.BaseURL+"/mcp/list_resources", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Verify status code
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, http.StatusOK)
		}

		// Parse response
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify response structure
		resources, ok := result["resources"].([]interface{})
		if !ok {
			t.Fatalf("resources is not an array: %v", result["resources"])
		}

		// Without authentication, we should only see auth resource
		// With authentication, we would see more resources
		if len(resources) < 1 {
			t.Error("Expected at least one resource (auth://rtm)")
		}

		// Check if auth resource is present
		authResourceFound := false
		for _, res := range resources {
			resource, ok := res.(map[string]interface{})
			if !ok {
				t.Errorf("Resource is not an object: %v", res)
				continue
			}

			if name, ok := resource["name"].(string); ok && name == "auth://rtm" {
				authResourceFound = true
				break
			}
		}

		if !authResourceFound {
			t.Error("auth://rtm resource not found in list_resources response")
		}
	})

	// Test read_resource endpoint with auth resource
	t.Run("read_resource_auth", func(t *testing.T) {
		// Configure mock for getFrob which is used in the auth resource
		rtmMock.AddResponse("rtm.auth.getFrob", `<rsp stat="ok"><frob>test_frob_12345</frob></rsp>`)

		req, err := http.NewRequest(http.MethodGet, client.BaseURL+"/mcp/read_resource?name=auth://rtm", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Verify status code
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, http.StatusOK)
			// Log body for debugging
			body, _ := io.ReadAll(resp.Body)
			t.Logf("Response body: %s", string(body))
			return
		}

		// Parse response
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify response structure
		content, ok := result["content"].(string)
		if !ok {
			t.Fatalf("content is not a string: %v", result["content"])
		}

		if !strings.Contains(content, "Authentication") {
			t.Error("Authentication content not found in auth://rtm resource")
		}

		mimeType, ok := result["mime_type"].(string)
		if !ok {
			t.Fatalf("mime_type is not a string: %v", result["mime_type"])
		}

		if mimeType != "text/markdown" && mimeType != "text/plain" {
			t.Errorf("Unexpected mime_type: got %s, want text/markdown or text/plain", mimeType)
		}
	})

	// Test read_resource with non-existent resource
	t.Run("read_resource_nonexistent", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, client.BaseURL+"/mcp/read_resource?name=nonexistent://resource", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Verify status code - should be error
		if resp.StatusCode != http.StatusNotFound {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Unexpected status code: got %d, want %d. Body: %s", resp.StatusCode, http.StatusNotFound, string(body))
			return
		}

		// Parse error response
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify error response structure
		if result["error"] == nil {
			t.Error("Error response missing error field")
		}
	})
}

// TestMCPToolEndpoints tests the tool-related endpoints for compliance.
func TestMCPToolEndpoints(t *testing.T) {
	// Create a test configuration
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

	// Create a mock RTM server
	rtmMock := mocks.NewRTMServer(t)
	defer rtmMock.Close()

	// Override RTM API endpoint in client
	if err := os.Setenv("RTM_API_ENDPOINT", rtmMock.URL); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer os.Unsetenv("RTM_API_ENDPOINT")

	// Create and start the MCP server
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create MCP test client
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// Test list_tools endpoint
	t.Run("list_tools", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, client.BaseURL+"/mcp/list_tools", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Verify status code
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, http.StatusOK)
		}

		// Parse response
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify response structure
		tools, ok := result["tools"].([]interface{})
		if !ok {
			t.Fatalf("tools is not an array: %v", result["tools"])
		}

		// Without authentication, we should only see authenticate tool
		// With authentication, we would see more tools
		if len(tools) < 1 {
			t.Error("Expected at least one tool (authenticate)")
		}

		// Check if authenticate tool is present
		authToolFound := false
		for _, tool := range tools {
			toolObj, ok := tool.(map[string]interface{})
			if !ok {
				t.Errorf("Tool is not an object: %v", tool)
				continue
			}

			if name, ok := toolObj["name"].(string); ok && name == "authenticate" {
				authToolFound = true

				// Check arguments structure
				args, ok := toolObj["arguments"].([]interface{})
				if !ok {
					t.Error("authenticate tool arguments is not an array")
				} else if len(args) == 0 {
					t.Error("authenticate tool should have at least one argument (frob)")
				}

				break
			}
		}

		if !authToolFound {
			t.Error("authenticate tool not found in list_tools response")
		}
	})

	// Test call_tool endpoint with authenticate tool (should fail without proper frob)
	t.Run("call_tool_authenticate", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"name": "authenticate",
			"arguments": map[string]interface{}{
				"frob": "invalid_frob",
			},
		}
		body, err := json.Marshal(reqBody)
		if err != nil {
			t.Fatalf("Failed to marshal request: %v", err)
		}

		req, err := http.NewRequest(http.MethodPost, client.BaseURL+"/mcp/call_tool", strings.NewReader(string(body)))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// This should fail because we're using an invalid frob, but the API should
		// accept the request and return a structured error
		if resp.StatusCode != http.StatusInternalServerError &&
			resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Unexpected status code: got %d, want 500 or 400", resp.StatusCode)
		}

		// Parse error response
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify error response structure
		if result["error"] == nil {
			t.Error("Error response missing error field")
		}
	})
}

// TestReadResourceAuthenticated tests the resource endpoints when authenticated
func TestReadResourceAuthenticated(t *testing.T) {
	// Skip for now - needs more setup for authentication flow
	t.Skip("Requires authentication flow setup")

	// Create a test configuration
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

	// Create mock RTM server
	rtmMock := mocks.NewRTMServer(t)
	defer rtmMock.Close()

	// Add necessary responses for authenticated resources
	rtmMock.AddResponse("rtm.auth.getFrob", `<rsp stat="ok"><frob>test_frob_12345</frob></rsp>`)
	rtmMock.AddResponse("rtm.auth.getToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)
	rtmMock.AddResponse("rtm.auth.checkToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)
	rtmMock.AddResponse("rtm.lists.getList", `<rsp stat="ok"><lists><list id="1" name="Inbox" deleted="0" locked="1" archived="0" position="-1" smart="0" /></lists></rsp>`)
	rtmMock.AddResponse("rtm.tasks.getList", `<rsp stat="ok"><tasks><list id="1"><taskseries id="1" created="2025-03-15T12:00:00Z" modified="2025-03-15T12:00:00Z" name="Test Task" source="api"><tags /><participants /><notes /><task id="1" due="" has_due_time="0" added="2025-03-15T12:00:00Z" completed="" deleted="" priority="N" postponed="0" estimate="" /></taskseries></list></tasks></rsp>`)

	// Override RTM API endpoint
	if err := os.Setenv("RTM_API_ENDPOINT", rtmMock.URL); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer os.Unsetenv("RTM_API_ENDPOINT")

	// Create server
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create test client
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// TODO: Implement test for authenticated resources
	// This would require:
	// 1. Running the authentication flow
	// 2. Setting up a mock token in the token manager
	// 3. Testing resource endpoints like lists://all and tasks://today
}

// validateMCPResource validates the structure of a resource definition from list_resources
func validateMCPResource(t *testing.T, resource interface{}) {
	t.Helper()

	resourceObj, ok := resource.(map[string]interface{})
	if !ok {
		t.Errorf("Resource is not an object: %v", resource)
		return
	}

	// Check required fields
	requiredFields := []string{"name", "description"}
	for _, field := range requiredFields {
		if resourceObj[field] == nil {
			t.Errorf("Resource missing required field: %s", field)
		}
	}

	// Check name is a string
	if _, ok := resourceObj["name"].(string); !ok {
		t.Errorf("Resource name is not a string: %v", resourceObj["name"])
	}

	// Check description is a string
	if _, ok := resourceObj["description"].(string); !ok {
		t.Errorf("Resource description is not a string: %v", resourceObj["description"])
	}

	// Check arguments if present
	if args, ok := resourceObj["arguments"].([]interface{}); ok {
		for i, arg := range args {
			argObj, ok := arg.(map[string]interface{})
			if !ok {
				t.Errorf("Argument %d is not an object: %v", i, arg)
				continue
			}

			// Check required argument fields
			argFields := []string{"name", "description", "required"}
			for _, field := range argFields {
				if argObj[field] == nil {
					t.Errorf("Argument %d missing required field: %s", i, field)
				}
			}
		}
	}
}

// validateMCPTool validates the structure of a tool definition from list_tools
func validateMCPTool(t *testing.T, tool interface{}) {
	t.Helper()

	toolObj, ok := tool.(map[string]interface{})
	if !ok {
		t.Errorf("Tool is not an object: %v", tool)
		return
	}

	// Check required fields
	requiredFields := []string{"name", "description"}
	for _, field := range requiredFields {
		if toolObj[field] == nil {
			t.Errorf("Tool missing required field: %s", field)
		}
	}

	// Check name is a string
	if _, ok := toolObj["name"].(string); !ok {
		t.Errorf("Tool name is not a string: %v", toolObj["name"])
	}

	// Check description is a string
	if _, ok := toolObj["description"].(string); !ok {
		t.Errorf("Tool description is not a string: %v", toolObj["description"])
	}

	// Check arguments if present
	if args, ok := toolObj["arguments"].([]interface{}); ok {
		for i, arg := range args {
			argObj, ok := arg.(map[string]interface{})
			if !ok {
				t.Errorf("Argument %d is not an object: %v", i, arg)
				continue
			}

			// Check required argument fields
			argFields := []string{"name", "description", "required"}
			for _, field := range argFields {
				if argObj[field] == nil {
					t.Errorf("Argument %d missing required field: %s", i, field)
				}
			}
		}
	}
}
