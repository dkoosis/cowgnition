// Package conformance provides tests to verify MCP protocol compliance.
package conformance

import (
	"strings"
	"testing"
)

// MCPResourceDefinition represents the expected structure of a resource
// definition from the MCP protocol.
type MCPResourceDefinition struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Arguments   []MCPResourceArgument    `json:"arguments,omitempty"`
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

	// Cast the resource to a map
	resourceObj, ok := resource.(map[string]interface{})
	if !ok {
		t.Errorf("Resource is not an object: %v", resource)
		return false
	}

	// Check required fields
	requiredFields := []string{"name", "description"}
	for _, field := range requiredFields {
		if resourceObj[field] == nil {
			t.Errorf("Resource missing required field: %s", field)
			return false
		}
	}

	// Validate field types
	name, ok := resourceObj["name"].(string)
	if !ok {
		t.Errorf("Resource name is not a string: %v", resourceObj["name"])
		return false
	}
	
	if name == "" {
		t.Errorf("Resource name cannot be empty")
		return false
	}

	description, ok := resourceObj["description"].(string)
	if !ok {
		t.Errorf("Resource description is not a string: %v", resourceObj["description"])
		return false
	}

	// Validate resource name format
	// According to MCP spec, resource names should follow a URL-like format
	if !validateResourceNameFormat(name) {
		t.Errorf("Invalid resource name format: %s (should be scheme://path or scheme://path/{param})", name)
		return false
	}

	// Check arguments if present
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
		t.Errorf("Argument %d is not an object: %v", index, arg)
		return false
	}

	// Check required argument fields
	argFields := []string{"name", "description", "required"}
	for _, field := range argFields {
		if argObj[field] == nil {
			t.Errorf("Argument %d missing required field: %s", index, field)
			return false
		}
	}

	// Validate field types
	_, ok = argObj["name"].(string)
	if !ok {
		t.Errorf("Argument %d name is not a string", index)
		return false
	}

	_, ok = argObj["description"].(string)
	if !ok {
		t.Errorf("Argument %d description is not a string", index)
		return false
	}

	_, ok = argObj["required"].(bool)
	if !ok {
		t.Errorf("Argument %d required is not a boolean", index)
		return false
	}

	return true
}

// validateResourceNameFormat checks if a resource name follows the MCP specification.
// According to the spec, resource names should be in the format scheme://path
// or scheme://path/{param} where {param} indicates a parameter.
func validateResourceNameFormat(name string) bool {
	// A complete validation would use regex, but for simplicity,
	// we'll just check for presence of "://" to indicate scheme
	// Acceptable formats include:
	// - files://all
	// - tasks://today
	// - tasks://list/{list_id}
	
	// For our current implementation, we'll accept anything with "://"
	// This could be enhanced with more specific validation in the future
	return len(name) > 3 && strings.Contains(name[1:], "://")
}

// validateResourceResponse validates a response from read_resource.
func validateResourceResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check for required fields
	requiredFields := []string{"content", "mime_type"}
	for _, field := range requiredFields {
		if response[field] == nil {
			t.Errorf("Resource response missing required field: %s", field)
			return false
		}
	}

	// Validate field types
	content, ok := response["content"].(string)
	if !ok {
		t.Errorf("Resource content is not a string: %v", response["content"])
		return false
	}

	mimeType, ok := response["mime_type"].(string)
	if !ok {
		t.Errorf("Resource mime_type is not a string: %v", response["mime_type"])
		return false
	}

	// Validate mime type format
	if !validateMimeType(mimeType) {
		t.Errorf("Invalid mime type: %s", mimeType)
		return false
	}

	// Additional validation - content shouldn't be empty for most resources
	// This might be resource-specific, so we don't fail on this
	if content == "" {
		t.Logf("Warning: Resource content is empty")
	}

	return true
}

// validateMimeType checks if a MIME type is in a valid format.
func validateMimeType(mimeType string) bool {
	// Common MIME types used in MCP resources
	validMimeTypes := map[string]bool{
		"text/plain":     true,
		"text/markdown":  true,
		"text/html":      true,
		"application/json": true,
		"image/png":      true,
		"image/jpeg":     true,
		"image/svg+xml":  true,
	}

	// For more complex validation, we could use regex to check format
	// But for now, we'll accept common types or anything with a "/"
	return validMimeTypes[mimeType] || strings.Contains(mimeType, "/")
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

	// Cast the tool to a map
	toolObj, ok := tool.(map[string]interface{})
	if !ok {
		t.Errorf("Tool is not an object: %v", tool)
		return false
	}

	// Check required fields
	requiredFields := []string{"name", "description"}
	for _, field := range requiredFields {
		if toolObj[field] == nil {
			t.Errorf("Tool missing required field: %s", field)
			return false
		}
	}

	// Validate field types
	name, ok := toolObj["name"].(string)
	if !ok {
		t.Errorf("Tool name is not a string: %v", toolObj["name"])
		return false
	}
	
	if name == "" {
		t.Errorf("Tool name cannot be empty")
		return false
	}

	description, ok := toolObj["description"].(string)
	if !ok {
		t.Errorf("Tool description is not a string: %v", toolObj["description"])
		return false
	}

	// Check arguments if present
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
		t.Errorf("Tool argument %d is not an object: %v", index, arg)
		return false
	}

	// Check required argument fields
	argFields := []string{"name", "description", "required"}
	for _, field := range argFields {
		if argObj[field] == nil {
			t.Errorf("Tool argument %d missing required field: %s", index, field)
			return false
		}
	}

	// Validate field types
	_, ok = argObj["name"].(string)
	if !ok {
		t.Errorf("Tool argument %d name is not a string", index)
		return false
	}

	_, ok = argObj["description"].(string)
	if !ok {
		t.Errorf("Tool argument %d description is not a string", index)
		return false
	}

	_, ok = argObj["required"].(bool)
	if !ok {
		t.Errorf("Tool argument %d required is not a boolean", index)
		return false
	}

	return true
}

// validateToolResponse validates a response from call_tool.
func validateToolResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check for required fields
	result, ok := response["result"]
	if !ok {
		t.Errorf("Tool response missing required field: result")
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
