// test/helpers/mcp/validators.go
// CONSOLIDATION-2024-03-24: This file centralizes all MCP validation functions

package mcp

import (
	"strings"
	"testing"
	"time"
)

// ValidateMCPResource validates a resource definition from list_resources
func ValidateMCPResource(t *testing.T, resource interface{}) bool {
	t.Helper()

	resourceObj, ok := resource.(map[string]interface{})
	if !ok {
		t.Errorf("Resource is not an object; got %T", resource)
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

	// Validate field types and content
	name, ok := resourceObj["name"].(string)
	if !ok || name == "" {
		t.Errorf("Resource name invalid: %v", resourceObj["name"])
		return false
	}

	_, ok = resourceObj["description"].(string)
	if !ok {
		t.Errorf("Resource description not a string: %v", resourceObj["description"])
		return false
	}

	// Validate resource name format
	if !strings.Contains(name, "://") {
		t.Errorf("Resource name doesn't follow scheme://path format: %s", name)
		return false
	}

	// Check arguments if present
	if args, ok := resourceObj["arguments"].([]interface{}); ok {
		for i, arg := range args {
			if !ValidateResourceArgument(t, i, arg) {
				return false
			}
		}
	}

	return true
}

// ValidateResourceArgument validates a resource argument
func ValidateResourceArgument(t *testing.T, index int, arg interface{}) bool {
	t.Helper()

	argObj, ok := arg.(map[string]interface{})
	if !ok {
		t.Errorf("Argument %d is not an object", index)
		return false
	}

	// Check required fields
	requiredFields := []string{"name", "description", "required"}
	for _, field := range requiredFields {
		if argObj[field] == nil {
			t.Errorf("Argument %d missing field: %s", index, field)
			return false
		}
	}

	// Validate types
	if _, ok := argObj["name"].(string); !ok {
		t.Errorf("Argument %d name not a string", index)
		return false
	}

	if _, ok := argObj["description"].(string); !ok {
		t.Errorf("Argument %d description not a string", index)
		return false
	}

	if _, ok := argObj["required"].(bool); !ok {
		t.Errorf("Argument %d required not a boolean", index)
		return false
	}

	return true
}

// ValidateResourceResponse validates a response from read_resource
func ValidateResourceResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check required fields
	requiredFields := []string{"content", "mime_type"}
	for _, field := range requiredFields {
		if response[field] == nil {
			t.Errorf("Resource response missing field: %s", field)
			return false
		}
	}

	// Validate types
	content, ok := response["content"].(string)
	if !ok {
		t.Errorf("Resource content not a string: %v", response["content"])
		return false
	}

	mimeType, ok := response["mime_type"].(string)
	if !ok {
		t.Errorf("Resource mime_type not a string: %v", response["mime_type"])
		return false
	}

	// Validate mime type format
	if !strings.Contains(mimeType, "/") {
		t.Errorf("Invalid mime type: %s", mimeType)
		return false
	}

	// Content shouldn't be empty for most resources
	if content == "" {
		t.Logf("Warning: Resource content is empty")
	}

	return true
}

// ValidateMCPTool validates a tool definition
func ValidateMCPTool(t *testing.T, tool interface{}) bool {
	t.Helper()

	toolObj, ok := tool.(map[string]interface{})
	if !ok {
		t.Errorf("Tool is not an object; got %T", tool)
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
	if !ok || name == "" {
		t.Errorf("Tool name invalid: %v", toolObj["name"])
		return false
	}

	_, ok = toolObj["description"].(string)
	if !ok {
		t.Errorf("Tool description not a string: %v", toolObj["description"])
		return false
	}

	// Check arguments if present
	if args, ok := toolObj["arguments"].([]interface{}); ok {
		for i, arg := range args {
			if !ValidateToolArgument(t, i, arg) {
				return false
			}
		}
	}

	return true
}

// ValidateToolArgument validates a tool argument
func ValidateToolArgument(t *testing.T, index int, arg interface{}) bool {
	t.Helper()

	argObj, ok := arg.(map[string]interface{})
	if !ok {
		t.Errorf("Tool argument %d is not an object", index)
		return false
	}

	// Check required fields
	requiredFields := []string{"name", "description", "required"}
	for _, field := range requiredFields {
		if argObj[field] == nil {
			t.Errorf("Tool argument %d missing field: %s", index, field)
			return false
		}
	}

	// Validate types
	if _, ok := argObj["name"].(string); !ok {
		t.Errorf("Tool argument %d name not a string", index)
		return false
	}

	if _, ok := argObj["description"].(string); !ok {
		t.Errorf("Tool argument %d description not a string", index)
		return false
	}

	if _, ok := argObj["required"].(bool); !ok {
		t.Errorf("Tool argument %d required not a boolean", index)
		return false
	}

	return true
}

// ValidateToolResponse validates a response from call_tool
func ValidateToolResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check for result field
	result, ok := response["result"]
	if !ok {
		t.Errorf("Tool response missing result field")
		return false
	}

	if result == nil {
		t.Errorf("Tool response 'result' field is nil")
		return false
	}

	// Validate result is a string
	if _, ok := result.(string); !ok {
		t.Errorf("Tool result not a string: %v", result)
		return false
	}

	return true
}

// ValidateErrorResponse validates JSON-RPC error responses
func ValidateErrorResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check for error or status field
	if response["error"] == nil && response["status"] == nil {
		t.Error("Error response missing both error and status fields")
		return false
	}

	// Check status code
	if status, ok := response["status"].(float64); ok {
		if status < 400 {
			t.Errorf("Error status code should be >= 400, got %v", status)
			return false
		}
	}

	// Validate error object
	if errObj, ok := response["error"].(map[string]interface{}); ok {
		// Check code
		if code, ok := errObj["code"].(float64); !ok {
			t.Error("Error object missing code field or not a number")
			return false
		} else if code == 0 {
			t.Error("Error code should not be 0")
			return false
		}

		// Check message
		if msg, ok := errObj["message"].(string); !ok || msg == "" {
			t.Error("Error object missing message field or empty")
			return false
		}
	}

	// Check timestamp
	if ts, ok := response["timestamp"].(string); ok {
		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("Invalid timestamp format: %s", ts)
			return false
		}
	}

	return true
}
