// Package mcp provides validation utilities for MCP protocol testing.
// file: test/conformance/mcp/validators.go
package mcp

import (
	"fmt"
	"regexp"
	"testing"
	"time"
)

// Constants for validation
const (
	// Resource field names
	FieldResourceName        = "name"
	FieldResourceDescription = "description"
	FieldResourceMimeType    = "mime_type"
	FieldResourceContent     = "content"

	// Argument field names
	FieldArgName        = "name"
	FieldArgDescription = "description"
	FieldArgRequired    = "required"

	// Tool field names
	FieldToolName        = "name"
	FieldToolDescription = "description"
	FieldToolResult      = "result"
)

// ValidateMCPResource validates a resource definition conforms to MCP protocol.
func ValidateMCPResource(t *testing.T, resource interface{}) bool {
	t.Helper()

	resourceObj, ok := resource.(map[string]interface{})
	if !ok {
		t.Errorf("Resource is not an object: %T", resource)
		return false
	}

	// Check required fields
	requiredFields := []string{FieldResourceName, FieldResourceDescription}
	for _, field := range requiredFields {
		if resourceObj[field] == nil {
			t.Errorf("Resource missing required field: %s", field)
			return false
		}
	}

	// Validate field types
	name, ok := resourceObj[FieldResourceName].(string)
	if !ok {
		t.Errorf("Resource name is not a string: %v", resourceObj[FieldResourceName])
		return false
	}

	if name == "" {
		t.Errorf("Resource name cannot be empty")
		return false
	}

	_, ok = resourceObj[FieldResourceDescription].(string)
	if !ok {
		t.Errorf("Resource description is not a string: %v", resourceObj[FieldResourceDescription])
		return false
	}

	// Validate resource name format
	if !validateResourceNameFormat(name) {
		t.Errorf("Invalid resource name format: %s", name)
		return false
	}

	// Check arguments if present
	if args, ok := resourceObj["arguments"].([]interface{}); ok {
		for i, arg := range args {
			if !validateArgument(t, i, arg) {
				return false
			}
		}
	}

	return true
}

// ValidateMCPTool validates a tool definition conforms to MCP protocol.
func ValidateMCPTool(t *testing.T, tool interface{}) bool {
	t.Helper()

	toolObj, ok := tool.(map[string]interface{})
	if !ok {
		t.Errorf("Tool is not an object: %T", tool)
		return false
	}

	// Check required fields
	requiredFields := []string{FieldToolName, FieldToolDescription}
	for _, field := range requiredFields {
		if toolObj[field] == nil {
			t.Errorf("Tool missing required field: %s", field)
			return false
		}
	}

	// Validate field types
	name, ok := toolObj[FieldToolName].(string)
	if !ok {
		t.Errorf("Tool name is not a string: %v", toolObj[FieldToolName])
		return false
	}

	if name == "" {
		t.Errorf("Tool name cannot be empty")
		return false
	}

	_, ok = toolObj[FieldToolDescription].(string)
	if !ok {
		t.Errorf("Tool description is not a string: %v", toolObj[FieldToolDescription])
		return false
	}

	// Check arguments if present
	if args, ok := toolObj["arguments"].([]interface{}); ok {
		for i, arg := range args {
			if !validateArgument(t, i, arg) {
				return false
			}
		}
	}

	return true
}

// ValidateResourceResponse validates resource read response.
func ValidateResourceResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check required fields
	requiredFields := []string{FieldResourceContent, FieldResourceMimeType}
	for _, field := range requiredFields {
		if response[field] == nil {
			t.Errorf("Resource response missing required field: %s", field)
			return false
		}
	}

	// Validate field types
	content, ok := response[FieldResourceContent].(string)
	if !ok {
		t.Errorf("Resource content is not a string: %v", response[FieldResourceContent])
		return false
	}

	mimeType, ok := response[FieldResourceMimeType].(string)
	if !ok {
		t.Errorf("Resource mime_type is not a string: %v", response[FieldResourceMimeType])
		return false
	}

	// Validate MIME type
	if !validateMimeType(mimeType) {
		t.Errorf("Invalid MIME type: %s", mimeType)
		return false
	}

	// Content shouldn't be empty for most resources
	if content == "" {
		t.Logf("Warning: Resource content is empty")
	}

	return true
}

// ValidateToolResponse validates tool call response.
func ValidateToolResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check result field
	result, ok := response[FieldToolResult]
	if !ok {
		t.Errorf("Tool response missing required field: %s", FieldToolResult)
		return false
	}

	if result == nil {
		t.Errorf("Tool response '%s' field is nil", FieldToolResult)
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

// ValidateErrorResponse validates error response structure.
func ValidateErrorResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check for error field or status field
	if response["error"] == nil && response["status"] == nil {
		t.Error("Error response missing both error and status fields")
		return false
	}

	// Check status code if present
	if status, ok := response["status"].(float64); ok {
		if status < 400 {
			t.Errorf("Error status code should be >= 400, got %v", status)
			return false
		}
	}

	// Check error object structure if present
	if errObj, ok := response["error"].(map[string]interface{}); ok {
		// Check error code
		if code, ok := errObj["code"].(float64); !ok {
			t.Error("Error object missing code field or code is not a number")
			return false
		} else if code == 0 {
			t.Error("Error code should not be 0")
			return false
		}

		// Check error message
		if msg, ok := errObj["message"].(string); !ok || msg == "" {
			t.Error("Error object missing message field or message is empty")
			return false
		}
	}

	// Check timestamp if present
	if ts, ok := response["timestamp"].(string); ok {
		// Validate timestamp format
		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("Invalid timestamp format: %s", ts)
			return false
		}
	}

	return true
}

// Helper functions

// validateArgument validates an argument structure.
func validateArgument(t *testing.T, index int, arg interface{}) bool {
	t.Helper()

	argObj, ok := arg.(map[string]interface{})
	if !ok {
		t.Errorf("Argument %d is not an object: %T", index, arg)
		return false
	}

	// Check required fields
	requiredFields := []string{FieldArgName, FieldArgDescription, FieldArgRequired}
	for _, field := range requiredFields {
		if argObj[field] == nil {
			t.Errorf("Argument %d missing required field: %s", index, field)
			return false
		}
	}

	// Validate field types
	_, ok = argObj[FieldArgName].(string)
	if !ok {
		t.Errorf("Argument %d name is not a string", index)
		return false
	}

	_, ok = argObj[FieldArgDescription].(string)
	if !ok {
		t.Errorf("Argument %d description is not a string", index)
		return false
	}

	_, ok = argObj[FieldArgRequired].(bool)
	if !ok {
		t.Errorf("Argument %d required is not a boolean", index)
		return false
	}

	return true
}

// validateResourceNameFormat validates resource name follows 'scheme://path' format.
func validateResourceNameFormat(name string) bool {
	// Check for scheme://path format
	pattern := regexp.MustCompile(`^[a-z]+://[a-zA-Z0-9\-_\./]+(?:/\{[a-zA-Z0-9\-_]+\})?$`)
	return pattern.MatchString(name)
}

// validateMimeType validates MIME type format.
func validateMimeType(mimeType string) bool {
	// Basic MIME type pattern
	pattern := regexp.MustCompile(`^[a-z]+/[a-z0-9\-\.\+]+`)
	return pattern.MatchString(mimeType)
}

// FormatValidationError returns a formatted validation error message.
func FormatValidationError(objType, field string, expected, got interface{}) string {
	return fmt.Sprintf("%s %s validation error: expected %v, got %v",
		objType, field, expected, got)
}
