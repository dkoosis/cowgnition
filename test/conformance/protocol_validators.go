// Package conformance provides tests to verify MCP protocol compliance.
package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/helpers"
)

// MCPProtocolVersion defines the version of the MCP protocol being tested.
const MCPProtocolVersion = "1.0.0"

// MCPProtocolTester provides utilities for testing MCP protocol compliance.
type MCPProtocolTester struct {
	server *server.MCPServer
	client *helpers.MCPClient
	t      *testing.T
}

// NewMCPProtocolTester creates a new MCP protocol tester.
func NewMCPProtocolTester(t *testing.T, server *server.MCPServer) *MCPProtocolTester {
	t.Helper()
	client := helpers.NewMCPClient(t, server)
	return &MCPProtocolTester{
		server: server,
		client: client,
		t:      t,
	}
}

// Close releases resources associated with the tester.
func (tester *MCPProtocolTester) Close() {
	tester.client.Close()
}

// TestInitialization tests the /mcp/initialize endpoint.
// Returns true if initialization succeeded, false otherwise.
func (tester *MCPProtocolTester) TestInitialization() bool {
	tester.t.Helper()
	t := tester.t

	// Perform initialization with standard values.
	reqBody := map[string]interface{}{
		"server_name":    "MCP Conformance Tester",
		"server_version": "1.0.0",
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Errorf("Failed to marshal initialization request: %v", err)
		return false
	}

	// Send initialization request.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		tester.client.BaseURL+"/mcp/initialize", bytes.NewBuffer(body))
	if err != nil {
		t.Errorf("Failed to create initialization request: %v", err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tester.client.Client.Do(req)
	if err != nil {
		t.Errorf("Failed to send initialization request: %v", err)
		return false
	}
	defer resp.Body.Close()

	// Check status code.
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Initialization failed with status: %d", resp.StatusCode)
		return false
	}

	// Check response structure.
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Errorf("Failed to decode initialization response: %v", err)
		return false
	}

	// Validate server_info field.
	if result["server_info"] == nil {
		t.Error("Initialization response missing server_info field")
		return false
	}

	// Validate capabilities field.
	if result["capabilities"] == nil {
		t.Error("Initialization response missing capabilities field")
		return false
	}

	return true
}

// TestListResources tests the /mcp/list_resources endpoint.
// Returns the list of resources if successful, nil otherwise.
func (tester *MCPProtocolTester) TestListResources() []map[string]interface{} {
	tester.t.Helper()
	t := tester.t

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		tester.client.BaseURL+"/mcp/list_resources", nil)
	if err != nil {
		t.Errorf("Failed to create list_resources request: %v", err)
		return nil
	}

	resp, err := tester.client.Client.Do(req)
	if err != nil {
		t.Errorf("Failed to send list_resources request: %v", err)
		return nil
	}
	defer resp.Body.Close()

	// Check status code.
	if resp.StatusCode != http.StatusOK {
		t.Errorf("list_resources failed with status: %d", resp.StatusCode)
		return nil
	}

	// Check response structure.
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Errorf("Failed to decode list_resources response: %v", err)
		return nil
	}

	// Extract resources array.
	resources, ok := result["resources"].([]interface{})
	if !ok {
		t.Errorf("list_resources response does not contain resources array")
		return nil
	}

	// Convert to better type.
	resourcesList := make([]map[string]interface{}, 0, len(resources))
	for _, r := range resources {
		if res, ok := r.(map[string]interface{}); ok {
			resourcesList = append(resourcesList, res)
		}
	}

	return resourcesList
}

// TestReadResource tests the /mcp/read_resource endpoint for a specific resource.
// Returns the resource content and MIME type if successful, empty strings otherwise.
func (tester *MCPProtocolTester) TestReadResource(resourceName string) (string, string) {
	tester.t.Helper()
	t := tester.t

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	urlPath := fmt.Sprintf("%s/mcp/read_resource?name=%s",
		tester.client.BaseURL, url.QueryEscape(resourceName))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath, nil)
	if err != nil {
		t.Errorf("Failed to create read_resource request: %v", err)
		return "", ""
	}

	resp, err := tester.client.Client.Do(req)
	if err != nil {
		t.Errorf("Failed to send read_resource request: %v", err)
		return "", ""
	}
	defer resp.Body.Close()

	// Check status code.
	if resp.StatusCode != http.StatusOK {
		t.Logf("read_resource failed with status: %d", resp.StatusCode)
		return "", ""
	}

	// Check response structure.
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Errorf("Failed to decode read_resource response: %v", err)
		return "", ""
	}

	// Extract content and mimeType.
	content, _ := result["content"].(string)
	mimeType, _ := result["mime_type"].(string)

	return content, mimeType
}

// TestListTools tests the /mcp/list_tools endpoint.
// Returns the list of tools if successful, nil otherwise.
func (tester *MCPProtocolTester) TestListTools() []map[string]interface{} {
	tester.t.Helper()
	t := tester.t

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		tester.client.BaseURL+"/mcp/list_tools", nil)
	if err != nil {
		t.Errorf("Failed to create list_tools request: %v", err)
		return nil
	}

	resp, err := tester.client.Client.Do(req)
	if err != nil {
		t.Errorf("Failed to send list_tools request: %v", err)
		return nil
	}
	defer resp.Body.Close()

	// Check status code.
	if resp.StatusCode != http.StatusOK {
		t.Errorf("list_tools failed with status: %d", resp.StatusCode)
		return nil
	}

	// Check response structure.
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Errorf("Failed to decode list_tools response: %v", err)
		return nil
	}

	// Extract tools array.
	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Errorf("list_tools response does not contain tools array")
		return nil
	}

	// Convert to better type.
	toolsList := make([]map[string]interface{}, 0, len(tools))
	for _, tool := range tools {
		if t, ok := tool.(map[string]interface{}); ok {
			toolsList = append(toolsList, t)
		}
	}

	return toolsList
}

// TestCallTool tests the /mcp/call_tool endpoint for a specific tool.
// Returns the tool result if successful, empty string otherwise.
func (tester *MCPProtocolTester) TestCallTool(toolName string, args map[string]interface{}) string {
	tester.t.Helper()
	t := tester.t

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reqBody := map[string]interface{}{
		"name":      toolName,
		"arguments": args,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Errorf("Failed to marshal call_tool request: %v", err)
		return ""
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		tester.client.BaseURL+"/mcp/call_tool", bytes.NewBuffer(body))
	if err != nil {
		t.Errorf("Failed to create call_tool request: %v", err)
		return ""
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tester.client.Client.Do(req)
	if err != nil {
		t.Errorf("Failed to send call_tool request: %v", err)
		return ""
	}
	defer resp.Body.Close()

	// Check status code - call_tool can return error codes for invalid tool calls.
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Logf("call_tool failed with status: %d, response: %s",
			resp.StatusCode, string(respBody))
		return ""
	}

	// Check response structure.
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Errorf("Failed to decode call_tool response: %v", err)
		return ""
	}

	// Extract result.
	toolResult, _ := result["result"].(string)
	return toolResult
}

// RunComprehensiveTest runs all MCP protocol conformance tests.
// This conducts a full test of the MCP server's protocol compliance.
func (tester *MCPProtocolTester) RunComprehensiveTest() {
	tester.t.Helper()
	t := tester.t

	// Test initialization.
	t.Run("Initialize", func(t *testing.T) {
		if !tester.TestInitialization() {
			t.Fatal("Initialization failed, cannot continue with other tests")
		}
	})

	// Test list_resources.
	var resources []map[string]interface{}
	t.Run("ListResources", func(t *testing.T) {
		resources = tester.TestListResources()
		if resources == nil {
			t.Error("list_resources failed")
		} else {
			// Validate resource structures.
			for i, resource := range resources {
				if !validateMCPResource(t, resource) {
					t.Errorf("Resource %d failed validation", i)
				}
			}
		}
	})

	// Test read_resource for all resources.
	t.Run("ReadResources", func(t *testing.T) {
		if len(resources) == 0 {
			t.Skip("No resources to test")
		}

		for _, resource := range resources {
			name, ok := resource["name"].(string)
			if !ok {
				continue
			}

			t.Run(name, func(t *testing.T) {
				content, mimeType := tester.TestReadResource(name)
				if content == "" && mimeType == "" {
					t.Logf("Resource %s is not readable or requires authentication", name)
					return
				}

				// Validate content and MIME type.
				if !validateMimeType(mimeType) {
					t.Errorf("Invalid MIME type: %s", mimeType)
				}

				if strings.TrimSpace(content) == "" {
					t.Errorf("Resource content is empty")
				}
			})
		}
	})

	// Test list_tools.
	var tools []map[string]interface{}
	t.Run("ListTools", func(t *testing.T) {
		tools = tester.TestListTools()
		if tools == nil {
			t.Error("list_tools failed")
		} else {
			// Validate tool structures.
			for i, tool := range tools {
				if !validateMCPTool(t, tool) {
					t.Errorf("Tool %d failed validation", i)
				}
			}
		}
	})

	// Don't try to call tools that may have side effects.
	// Test only basic tools or tools with safe read-only operations.
	t.Run("CallTools", func(t *testing.T) {
		if len(tools) == 0 {
			t.Skip("No tools to test")
		}

		// Find tools that are safe to call without arguments.
		safeTools := []string{"auth_status"}

		for _, tool := range tools {
			name, ok := tool["name"].(string)
			if !ok {
				continue
			}

			// Only call known safe tools.
			for _, safeTool := range safeTools {
				if name == safeTool {
					result := tester.TestCallTool(name, map[string]interface{}{})
					t.Logf("Tool %s result: %s", name, result)

					// Verify result isn't empty.
					if strings.TrimSpace(result) == "" {
						t.Errorf("Tool %s returned empty result", name)
					}
				}
			}
		}
	})
}
