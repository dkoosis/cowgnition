// Package conformance provides tests to verify MCP protocol compliance.
package conformance

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/helpers"
	"github.com/cowgnition/cowgnition/test/mocks"
)

// TestMCPComprehensiveConformance provides a comprehensive test suite for
// validating conformance with the MCP protocol specification.
func TestMCPComprehensiveConformance(t *testing.T) {
	// Create a test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name: "Conformance Test Server",
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

	// Setup mock responses for all required RTM API endpoints
	setupMockRTMResponses(rtmMock)

	// Create and start the MCP server
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	s.SetVersion("conformance-test-version")

	// Simulate authentication for testing
	if err := helpers.SimulateAuthentication(s); err != nil {
		t.Logf("Warning: Could not simulate authentication: %v", err)
	}

	// Create MCP test client
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// Tests for Protocol Initialization
	t.Run("Initialization", func(t *testing.T) {
		testInitialization(t, client)
	})

	// Tests for Resource Management
	t.Run("Resources", func(t *testing.T) {
		testResources(t, client)
	})

	// Tests for Tool Management
	t.Run("Tools", func(t *testing.T) {
		testTools(t, client)
	})

	// Tests for Error Handling
	t.Run("ErrorHandling", func(t *testing.T) {
		testErrorHandling(t, client)
	})

	// Tests for Special Scenarios
	t.Run("SpecialScenarios", func(t *testing.T) {
		testSpecialScenarios(t, client)
	})
}

// testInitialization verifies the MCP initialization protocol flow.
func testInitialization(t *testing.T, client *helpers.MCPClient) {
	t.Helper()

	// Test initialization with valid parameters
	t.Run("ValidInitialization", func(t *testing.T) {
		_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Create initialization request
		reqBody := map[string]interface{}{
			"server_name":    "Test Client",
			"server_version": "1.0.0",
		}

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
}

// testResources verifies the MCP resource listing and reading capabilities.
func testResources(t *testing.T, client *helpers.MCPClient) {
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
// Renamed from validateResourceResponse to avoid conflict
func validateResourceResponseStructure(t *testing.T, response map[string]interface{}) {
	t.Helper()

	// Check required fields
	content, ok := response["content"].(string)
	if !ok {
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

// testTools verifies the MCP tool listing and calling capabilities.
func testTools(t *testing.T, client *helpers.MCPClient) {
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

// testErrorHandling verifies proper error handling according to MCP protocol.
func testErrorHandling(t *testing.T, client *helpers.MCPClient) {
	t.Helper()

	// Test missing resource
	t.Run("MissingResource", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			client.BaseURL+"/mcp/read_resource", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Should return an error for missing name parameter
		if resp.StatusCode == http.StatusOK {
			t.Error("Missing name parameter should not return 200 OK")
		}

		// Validate error response structure
		var errorResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
			t.Fatalf("Failed to decode error response: %v", err)
		}

		validateErrorResponse(t, errorResp)
	})

	// Test method not allowed
	t.Run("MethodNotAllowed", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			client.BaseURL+"/mcp/list_resources", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Should return an error for wrong method
		if resp.StatusCode == http.StatusOK {
			t.Error("Wrong method should not return 200 OK")
		}

		// Validate error response structure
		var errorResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
			t.Fatalf("Failed to decode error response: %v", err)
		}

		validateErrorResponse(t, errorResp)
	})

	// Test malformed JSON
	t.Run("MalformedJSON", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			client.BaseURL+"/mcp/initialize", strings.NewReader("{malformed json}"))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Should return an error for malformed JSON
		if resp.StatusCode == http.StatusOK {
			t.Error("Malformed JSON should not return 200 OK")
		}

		// Validate error response structure
		var errorResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err != nil {
			t.Fatalf("Failed to decode error response: %v", err)
		}

		validateErrorResponse(t, errorResp)
	})
}

// validateErrorResponse validates an error response structure.
func validateErrorResponse(t *testing.T, response map[string]interface{}) {
	t.Helper()

	// Check for error field or status field (required in MCP error responses)
	if response["error"] == nil && response["status"] == nil {
		t.Error("Error response missing both error and status fields")
	}

	// MCP spec requires status field in standardized error responses
	if status, ok := response["status"].(float64); ok {
		if status < 400 {
			t.Errorf("Error status code should be >= 400, got %v", status)
		}
	}

	// If error field is an object, validate its structure
	if errObj, ok := response["error"].(map[string]interface{}); ok {
		// Check for required error fields
		if code, ok := errObj["code"].(float64); !ok {
			t.Error("Error object missing code field or code is not a number")
		} else if code == 0 {
			t.Error("Error code should not be 0")
		}

		if msg, ok := errObj["message"].(string); !ok || msg == "" {
			t.Error("Error object missing message field or message is empty")
		}
	}

	// Check for timestamp field (recommended in errors)
	if ts, ok := response["timestamp"].(string); ok {
		// Attempt to parse timestamp to validate format
		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("Invalid timestamp format: %s", ts)
		}
	}
}

// testSpecialScenarios tests special edge cases for MCP protocol compliance.
func testSpecialScenarios(t *testing.T, client *helpers.MCPClient) {
	t.Helper()

	// Test case-sensitivity in resource names
	t.Run("ResourceCaseSensitivity", func(t *testing.T) {
		// MCP URIs should be case-sensitive
		resp, err := client.ReadResource(t, "AUTH://RTM")

		// Should fail for uppercase resource name
		if err == nil {
			t.Error("Uppercase resource name should not be accepted")
		}

		// Or return an error response if err is nil
		if err == nil && resp != nil {
			validateErrorResponse(t, resp)
		}
	})

	// Test for expected headers in responses
	t.Run("ResponseHeaders", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			client.BaseURL+"/mcp/list_resources", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Check Content-Type header
		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			t.Errorf("Response Content-Type should be application/json, got %s", contentType)
		}
	})
}

// setupMockRTMResponses configures the mock RTM server with required responses.
func setupMockRTMResponses(rtmMock *mocks.RTMServer) {
	// Authentication-related responses
	rtmMock.AddResponse("rtm.auth.getFrob", `<rsp stat="ok"><frob>test_frob_12345</frob></rsp>`)
	rtmMock.AddResponse("rtm.auth.getToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)
	rtmMock.AddResponse("rtm.auth.checkToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)

	// Timeline-related responses
	rtmMock.AddResponse("rtm.timelines.create", `<rsp stat="ok"><timeline>timeline_12345</timeline></rsp>`)

	// Task and list related responses
	rtmMock.AddResponse("rtm.lists.getList", `<rsp stat="ok"><lists><list id="1" name="Inbox" deleted="0" locked="1" archived="0" position="-1" smart="0" /></lists></rsp>`)
	rtmMock.AddResponse("rtm.tasks.getList", `<rsp stat="ok"><tasks><list id="1"><taskseries id="1" created="2025-03-15T12:00:00Z" modified="2025-03-15T12:00:00Z" name="Test Task" source="api"><tags /><participants /><notes /><task id="1" due="" has_due_time="0" added="2025-03-15T12:00:00Z" completed="" deleted="" priority="N" postponed="0" estimate="" /></taskseries></list></tasks></rsp>`)

	// Tool-related responses
	rtmMock.AddResponse("rtm.tasks.add", `<rsp stat="ok"><transaction id="1" undoable="1" /><list id="1"><taskseries id="1" created="2025-03-15T12:00:00Z" modified="2025-03-15T12:00:00Z" name="New Task" source="api"><tags /><participants /><notes /><task id="1" due="" has_due_time="0" added="2025-03-15T12:00:00Z" completed="" deleted="" priority="N" postponed="0" estimate="" /></taskseries></list></rsp>`)

	// Error responses for testing
	rtmMock.AddResponse("rtm.error.test", `<rsp stat="fail"><err code="101" msg="Test error message" /></rsp>`)
}
