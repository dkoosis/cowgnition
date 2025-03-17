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
	"regexp"
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
	client := helpers.NewMCPClient(t, server) // Pass the *server.MCPServer
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

// testListResourcesAndValidate runs list_resources and validates the returned resources.
func (tester *MCPProtocolTester) testListResourcesAndValidate(t *testing.T) []map[string]interface{} {
	t.Helper()
	resources := tester.TestListResources()
	if resources == nil {
		t.Error("list_resources failed")
		return nil
	}
	for i, resource := range resources {
		t.Run(fmt.Sprintf("Resource %d", i), func(t *testing.T) {
			if !validateMCPResource(t, resource) { // Use existing validator
				t.Errorf("Resource %d failed validation", i)
			}
		})
	}
	return resources
}

// testReadResources runs read_resource for all provided resources.
func (tester *MCPProtocolTester) testReadResources(t *testing.T, resources []map[string]interface{}) {
	t.Helper()
	if len(resources) == 0 {
		t.Skip("No resources to test")
		return
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
}

// testListToolsAndValidate runs list_tools and validates the returned tools.
func (tester *MCPProtocolTester) testListToolsAndValidate(t *testing.T) []map[string]interface{} {
	t.Helper()
	tools := tester.TestListTools()
	if tools == nil {
		t.Error("list_tools failed")
		return nil
	}
	for i, tool := range tools {
		t.Run(fmt.Sprintf("Tool %d", i), func(t *testing.T) {
			if !validateMCPTool(t, tool) { // Use existing validator
				t.Errorf("Tool %d failed validation", i)
			}
		})
	}
	return tools
}

// testCallSafeTools calls a predefined set of safe tools.
func (tester *MCPProtocolTester) testCallSafeTools(t *testing.T, tools []map[string]interface{}) {
	t.Helper()

	if len(tools) == 0 {
		t.Skip("No tools to test")
		return
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
			t.Run(name, func(t *testing.T) {
				if name == safeTool {
					result := tester.TestCallTool(name, map[string]interface{}{})
					t.Logf("Tool %s result: %s", name, result)

					// Verify result isn't empty.
					if strings.TrimSpace(result) == "" {
						t.Errorf("Tool %s returned empty result", name)
					}
				}
			})
		}
	}
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

	// Test list_resources and validate.
	var resources []map[string]interface{}
	t.Run("ListResources", func(t *testing.T) {
		resources = tester.testListResourcesAndValidate(t)
	})

	// Test Read Resources
	t.Run("ReadResources", func(t *testing.T) {
		tester.testReadResources(t, resources)
	})

	// Test List Tools and validate.
	var tools []map[string]interface{}
	t.Run("ListTools", func(t *testing.T) {
		tools = tester.testListToolsAndValidate(t)
	})

	// Test Call Safe Tools.
	t.Run("CallTools", func(t *testing.T) {
		tester.testCallSafeTools(t, tools)
	})
}

// Constants for required field names.
const (
	resourceFieldName        = "name"
	resourceFieldDescription = "description"
	resourceFieldMimeType    = "mime_type"
	resourceFieldContent     = "content"

	argFieldName        = "name"
	argFieldDescription = "description"
	argFieldRequired    = "required"

	toolFieldName        = "name"
	toolFieldDescription = "description"

	toolResponseFieldResult = "result"
)

// MCPResourceDefinition represents the expected structure of a resource
// definition from the MCP protocol.
type MCPResourceDefinition struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Arguments   []MCPResourceArgument `json:"arguments,omitempty"`
}

// MCPResourceArgument represents an argument for an MCP resource.
type MCPResourceArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// MCPResourceResponse represents the expected response structure for a resource.
type MCPResourceResponse struct {
	Content  string `json:"content"`
	MimeType string `json:"mime_type"`
}

// validateMCPResource validates a resource definition from list_resources conforms
// to the MCP protocol specification.
func validateMCPResource(t *testing.T, resource interface{}) bool {
	t.Helper()

	// Cast the resource to a map.
	resourceObj, ok := resource.(map[string]interface{})
	if !ok {
		t.Errorf("Resource is not an object; expected map[string]interface{}, got %T", resource)
		return false
	}

	// Check required fields.
	requiredFields := []string{resourceFieldName, resourceFieldDescription}
	for _, field := range requiredFields {
		if resourceObj[field] == nil {
			t.Errorf("Resource missing required field: %s", field)
			return false
		}
	}

	// Validate field types.
	name, ok := resourceObj[resourceFieldName].(string)
	if !ok {
		t.Errorf("Resource name is not a string: %v", resourceObj[resourceFieldName])
		return false
	}

	if name == "" {
		t.Errorf("Resource name cannot be empty")
		return false
	}

	_, ok = resourceObj[resourceFieldDescription].(string)
	if !ok {
		t.Errorf("Resource description is not a string: %v", resourceObj[resourceFieldDescription])
		return false
	}

	// Validate resource name format.
	if !validateResourceNameFormat(name) {
		t.Errorf("Invalid resource name format: %s (should be scheme://path or scheme://path/{param})", name)
		return false
	}

	// Check arguments if present.
	if args, ok := resourceObj["arguments"].([]interface{}); ok {
		for i, arg := range args {
			if !validateResourceArgument(t, i, arg) {
				return false
			}
		}
	}

	return true
}

// validateResourceArgument validates a single resource argument.
func validateResourceArgument(t *testing.T, index int, arg interface{}) bool {
	t.Helper()

	argObj, ok := arg.(map[string]interface{})
	if !ok {
		t.Errorf("Argument %d is not an object; expected map[string]interface{}, got %T", index, arg)
		return false
	}

	// Check required argument fields.
	argFields := []string{argFieldName, argFieldDescription, argFieldRequired}
	for _, field := range argFields {
		if argObj[field] == nil {
			t.Errorf("Argument %d missing required field: %s", index, field)
			return false
		}
	}

	// Validate field types.
	_, ok = argObj[argFieldName].(string)
	if !ok {
		t.Errorf("Argument %d name is not a string", index)
		return false
	}

	_, ok = argObj[argFieldDescription].(string)
	if !ok {
		t.Errorf("Argument %d description is not a string", index)
		return false
	}

	_, ok = argObj[argFieldRequired].(bool)
	if !ok {
		t.Errorf("Argument %d required is not a boolean", index)
		return false
	}

	return true
}

// validateResourceNameFormat checks if a resource name follows the MCP specification.
func validateResourceNameFormat(name string) bool {
	// Basic regex for scheme://path[/optional/path/segments][/{param}]
	nameRegex := regexp.MustCompile(`^[a-z]+://[a-zA-Z0-9\-_\./]+(?:/\{[a-zA-Z0-9\-_]+\})?$`)
	return nameRegex.MatchString(name)
}

// validateResourceResponse validates a response from read_resource.
func validateResourceResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check for required fields.
	requiredFields := []string{resourceFieldContent, resourceFieldMimeType}
	for _, field := range requiredFields {
		if response[field] == nil {
			t.Errorf("Resource response missing required field: %s", field)
			return false
		}
	}

	// Validate field types.
	content, ok := response[resourceFieldContent].(string)
	if !ok {
		t.Errorf("Resource content is not a string: %v", response[resourceFieldContent])
		return false
	}

	mimeType, ok := response[resourceFieldMimeType].(string)
	if !ok {
		t.Errorf("Resource mime_type is not a string: %v", response[resourceFieldMimeType])
		return false
	}

	// Validate mime type format.
	if !validateMimeType(mimeType) {
		t.Errorf("Invalid mime type: %s", mimeType)
		return false
	}

	// Additional validation - content shouldn't be empty for most resources.
	if content == "" {
		t.Logf("Warning: Resource content is empty")
	}

	return true
}

// validateMimeType checks if a MIME type is in a valid format.
func validateMimeType(mimeType string) bool {
	// More robust MIME type validation using a regular expression.
	mimeRegex := regexp.MustCompile(`^[a-z]+/[a-z0-9\-\.\+]*(;\s?[a-z0-9\-\.]+\s*=\s*[a-z0-9\-\.]+)*$`)
	return mimeRegex.MatchString(mimeType)
}

// MCPToolDefinition represents the expected structure of a tool
// definition from the MCP protocol.
type MCPToolDefinition struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Arguments   []MCPToolArgument `json:"arguments,omitempty"`
}

// MCPToolArgument represents an argument for an MCP tool.
type MCPToolArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// MCPToolResponse represents the expected response structure for a tool.
type MCPToolResponse struct {
	Result string `json:"result"`
}

// validateMCPTool validates a tool definition from list_tools conforms
// to the MCP protocol specification.
func validateMCPTool(t *testing.T, tool interface{}) bool {
	t.Helper()

	// Cast the tool to a map.
	toolObj, ok := tool.(map[string]interface{})
	if !ok {
		t.Errorf("Tool is not an object; expected map[string]interface{} got, %T", tool)
		return false
	}

	// Check required fields.
	requiredFields := []string{toolFieldName, toolFieldDescription}
	for _, field := range requiredFields {
		if toolObj[field] == nil {
			t.Errorf("Tool missing required field: %s", field)
			return false
		}
	}

	// Validate field types.
	name, ok := toolObj[toolFieldName].(string)
	if !ok {
		t.Errorf("Tool name is not a string: %v", toolObj[toolFieldName])
		return false
	}

	if name == "" {
		t.Errorf("Tool name cannot be empty")
		return false
	}

	_, ok = toolObj[toolFieldDescription].(string)
	if !ok {
		t.Errorf("Tool description is not a string: %v", toolObj[toolFieldDescription])
		return false
	}

	// Check arguments if present.
	if args, ok := toolObj["arguments"].([]interface{}); ok {
		for i, arg := range args {
			if !validateToolArgument(t, i, arg) {
				return false
			}
		}
	}

	return true
}

// validateToolArgument validates a single tool argument.
func validateToolArgument(t *testing.T, index int, arg interface{}) bool {
	t.Helper()

	argObj, ok := arg.(map[string]interface{})
	if !ok {
		t.Errorf("Tool argument %d is not an object; expected map[string]interface{}, got %T", index, arg)
		return false
	}

	// Check required argument fields.
	argFields := []string{argFieldName, argFieldDescription, argFieldRequired}
	for _, field := range argFields {
		if argObj[field] == nil {
			t.Errorf("Tool argument %d missing required field: %s", index, field)
			return false
		}
	}

	// Validate field types.
	_, ok = argObj[argFieldName].(string)
	if !ok {
		t.Errorf("Tool argument %d name is not a string", index)
		return false
	}

	_, ok = argObj[argFieldDescription].(string)
	if !ok {
		t.Errorf("Tool argument %d description is not a string", index)
		return false
	}

	_, ok = argObj[argFieldRequired].(bool)
	if !ok {
		t.Errorf("Tool argument %d required is not a boolean", index)
		return false
	}

	return true
}

// validateToolResponse validates a response from call_tool.
func validateToolResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check for required fields and handle nil result
	result, ok := response[toolResponseFieldResult]
	if !ok {
		t.Errorf("Tool response missing required field: result")
		return false
	}
	if result == nil {
		t.Errorf("Tool response 'result' field is nil")
		return false
	}

	// Validate field type
	_, ok = result.(string)
	if !ok {
		t.Errorf("Tool result is not a string: %v", result)
		return false
	}

	return true
}
