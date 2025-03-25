// file: test/validators/mcp/validators.go
// Package validators provides validation utilities for MCP protocol testing.
package validators

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Constants for validation.
const (
	// Resource field names.
	FieldResourceName        = "name"
	FieldResourceDescription = "description"
	FieldResourceMimeType    = "mime_type"
	FieldResourceContent     = "content"

	// Argument field names.
	FieldArgName        = "name"
	FieldArgDescription = "description"
	FieldArgRequired    = "required"

	// Tool field names.
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

	// Check required fields.
	requiredFields := []string{FieldResourceName, FieldResourceDescription}
	for _, field := range requiredFields {
		if resourceObj[field] == nil {
			t.Errorf("Resource missing required field: %s.", field)
			return false
		}
	}

	// Validate field types.
	name, ok := resourceObj[FieldResourceName].(string)
	if !ok {
		t.Errorf("Resource name is not a string: %v.", resourceObj[FieldResourceName])
		return false
	}

	if name == "" {
		t.Errorf("Resource name cannot be empty.")
		return false
	}

	_, ok = resourceObj[FieldResourceDescription].(string)
	if !ok {
		t.Errorf("Resource description is not a string: %v.", resourceObj[FieldResourceDescription])
		return false
	}

	// Validate resource name format.
	if !validateResourceNameFormat(name) {
		t.Errorf("Invalid resource name format: %s.", name)
		return false
	}

	// Check arguments if present.
	if args, ok := resourceObj["arguments"].([]interface{}); ok {
		for i, arg := range args {
			if !ValidateArgument(t, i, arg) {
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
		t.Errorf("Tool is not an object: %T.", tool)
		return false
	}

	// Check required fields.
	requiredFields := []string{FieldToolName, FieldToolDescription}
	for _, field := range requiredFields {
		if toolObj[field] == nil {
			t.Errorf("Tool missing required field: %s.", field)
			return false
		}
	}

	// Validate field types.
	name, ok := toolObj[FieldToolName].(string)
	if !ok {
		t.Errorf("Tool name is not a string: %v.", toolObj[FieldToolName])
		return false
	}

	if name == "" {
		t.Errorf("Tool name cannot be empty.")
		return false
	}

	_, ok = toolObj[FieldToolDescription].(string)
	if !ok {
		t.Errorf("Tool description is not a string: %v.", toolObj[FieldToolDescription])
		return false
	}

	// Check arguments if present.
	if args, ok := toolObj["arguments"].([]interface{}); ok {
		for i, arg := range args {
			if !ValidateArgument(t, i, arg) {
				return false
			}
		}
	}

	return true
}

// ValidateResourceResponse validates resource read response.
func ValidateResourceResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check for required fields
	requiredFields := []string{FieldResourceContent, FieldResourceMimeType}
	for _, field := range requiredFields {
		if response[field] == nil {
			t.Errorf("Resource response missing required field: %s.", field)
			return false
		}
	}

	// Validate field types
	content, ok := response[FieldResourceContent].(string)
	if !ok {
		t.Errorf("Resource content is not a string: %v.", response[FieldResourceContent])
		return false
	}

	mimeType, ok := response[FieldResourceMimeType].(string)
	if !ok {
		t.Errorf("Resource mime_type is not a string: %v.", response[FieldResourceMimeType])
		return false
	}

	// Validate MIME type.
	if !validateMimeType(mimeType) {
		t.Errorf("Invalid MIME type: %s.", mimeType)
		return false
	}

	// Content shouldn't be empty for most resources.
	if content == "" {
		t.Logf("Warning: Resource content is empty.")
	}

	return true
}

// ValidateToolResponse validates tool call response.
func ValidateToolResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check result field.
	result, ok := response[FieldToolResult]
	if !ok {
		t.Errorf("Tool response missing required field: %s.", FieldToolResult)
		return false
	}

	if result == nil {
		t.Errorf("Tool response '%s' field is nil.", FieldToolResult)
		return false
	}

	// Validate field type.
	_, ok = result.(string)
	if !ok {
		t.Errorf("Tool result is not a string: %v.", result)
		return false
	}

	return true
}

// ValidateErrorResponse validates error response structure.
func ValidateErrorResponse(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check for error field or status field.
	if response["error"] == nil && response["status"] == nil {
		t.Error("Error response missing both error and status fields.")
		return false
	}

	// Check status code if present.
	if status, ok := response["status"].(float64); ok {
		if status < 400 {
			t.Errorf("Error status code should be >= 400, got %v.", status)
			return false
		}
	}

	// Check error object structure if present.
	if errObj, ok := response["error"].(map[string]interface{}); ok {
		// Check error code.
		if code, ok := errObj["code"].(float64); !ok {
			t.Error("Error object missing code field or code is not a number.")
			return false
		} else if code == 0 {
			t.Error("Error code should not be 0.")
			return false
		}

		// Check error message.
		if msg, ok := errObj["message"].(string); !ok || msg == "" {
			t.Error("Error object missing message field or message is empty.")
			return false
		}
	}

	// Check timestamp if present.
	if ts, ok := response["timestamp"].(string); ok {
		// Validate timestamp format.
		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("Invalid timestamp format: %s.", ts)
			return false
		}
	}

	return true
}

// ValidateServerInfo validates the server_info object structure.
func ValidateServerInfo(t *testing.T, serverInfo map[string]interface{}, expectedName string) bool {
	t.Helper()

	// Check name field
	name, ok := serverInfo["name"].(string)
	if !ok || name == "" {
		t.Error("server_info missing or empty name field.")
		return false
	}

	// Check version field
	version, ok := serverInfo["version"].(string)
	if !ok || version == "" {
		t.Error("server_info missing or empty version field.")
		return false
	}

	// If expected name is provided, validate it
	if expectedName != "" && name != expectedName {
		t.Errorf("server_info.name mismatch: got %v, want %s.", name, expectedName)
		return false
	}

	return true
}

// ValidateCapabilities validates the capabilities object structure.
func ValidateCapabilities(t *testing.T, capabilities map[string]interface{}) bool {
	t.Helper()

	// Required capabilities
	requiredCapabilities := []string{
		"resources",
		"tools",
	}

	for _, capName := range requiredCapabilities {
		cap, ok := capabilities[capName].(map[string]interface{})
		if !ok {
			t.Errorf("capabilities missing required capability: %s.", capName)
			return false
		}

		// Validate resources capability
		if capName == "resources" {
			if !ValidateResourcesCapability(t, cap) {
				return false
			}
		}

		// Validate tools capability
		if capName == "tools" {
			if !ValidateToolsCapability(t, cap) {
				return false
			}
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
			if capName == "logging" {
				if !ValidateLoggingCapability(t, cap) {
					return false
				}
			}
			if capName == "prompts" {
				if !ValidatePromptsCapability(t, cap) {
					return false
				}
			}
		}
	}

	// Check for unknown capabilities
	knownCaps := map[string]bool{
		"resources":  true,
		"tools":      true,
		"logging":    true,
		"prompts":    true,
		"completion": true,
	}

	for cap := range capabilities {
		if !knownCaps[cap] {
			t.Logf("Warning: Unknown capability found: %s.", cap)
		}
	}

	return true
}

// ValidateResourcesCapability validates the resources capability structure.
func ValidateResourcesCapability(t *testing.T, resources map[string]interface{}) bool {
	t.Helper()

	// Required operations
	requiredOps := []string{
		"list",
		"read",
	}

	for _, op := range requiredOps {
		val, ok := resources[op].(bool)
		if !ok {
			t.Errorf("resources.%s is not a boolean.", op)
			return false
		} else if !val {
			t.Errorf("resources.%s should be true for a conformant server.", op)
			return false
		}
	}

	// Optional operations
	optionalOps := []string{
		"subscribe",
		"listChanged",
	}

	for _, op := range optionalOps {
		if val, ok := resources[op]; ok {
			if _, ok := val.(bool); !ok {
				t.Errorf("resources.%s is not a boolean.", op)
				return false
			}
		}
	}

	return true
}

// ValidateToolsCapability validates the tools capability structure.
func ValidateToolsCapability(t *testing.T, tools map[string]interface{}) bool {
	t.Helper()

	// Required operations
	requiredOps := []string{
		"list",
		"call",
	}

	for _, op := range requiredOps {
		val, ok := tools[op].(bool)
		if !ok {
			t.Errorf("tools.%s is not a boolean.", op)
			return false
		} else if !val {
			t.Errorf("tools.%s should be true for a conformant server.", op)
			return false
		}
	}

	// Optional operations
	optionalOps := []string{
		"listChanged",
	}

	for _, op := range optionalOps {
		if val, ok := tools[op]; ok {
			if _, ok := val.(bool); !ok {
				t.Errorf("tools.%s is not a boolean.", op)
				return false
			}
		}
	}

	return true
}

// ValidateLoggingCapability validates the logging capability structure.
func ValidateLoggingCapability(t *testing.T, logging map[string]interface{}) bool {
	t.Helper()

	// Expected logging fields
	logFields := []string{
		"log",
		"warning",
		"error",
	}

	for _, field := range logFields {
		if val, ok := logging[field]; ok {
			if _, ok := val.(bool); !ok {
				t.Errorf("logging.%s is not a boolean.", field)
				return false
			}
		}
	}

	return true
}

// ValidatePromptsCapability validates the prompts capability structure.
func ValidatePromptsCapability(t *testing.T, prompts map[string]interface{}) bool {
	t.Helper()

	// Expected prompt fields
	promptFields := []string{
		"list",
		"get",
		"listChanged",
	}

	for _, field := range promptFields {
		if val, ok := prompts[field]; ok {
			if _, ok := val.(bool); !ok {
				t.Errorf("prompts.%s is not a boolean.", field)
				return false
			}
		}
	}

	return true
}

// ValidateStandardErrorResponse checks if an error response has the required fields.
func ValidateStandardErrorResponse(t *testing.T, response map[string]interface{}, expectedStatus int) bool {
	t.Helper()

	// Check for required error response fields according to MCP spec
	requiredFields := []string{"error", "status", "timestamp"}
	for _, field := range requiredFields {
		if response[field] == nil {
			t.Errorf("Error response missing required field: %s.", field)
			return false
		}
	}

	// Verify correct status code
	if status, ok := response["status"].(float64); !ok || int(status) != expectedStatus {
		t.Errorf("Incorrect status code in error response: got %v, want %d.", response["status"], expectedStatus)
		return false
	}

	// Verify timestamp is present and in a reasonable format
	if timestamp, ok := response["timestamp"].(string); !ok || timestamp == "" {
		t.Error("Missing or invalid timestamp in error response.")
		return false
	}

	return true
}

// ValidateErrorFieldExists checks if the error field exists and is non-empty.
func ValidateErrorFieldExists(t *testing.T, response map[string]interface{}, field string) bool {
	t.Helper()

	if response[field] == nil {
		t.Errorf("Response missing field: %s.", field)
		return false
	}

	if errStr, ok := response[field].(string); !ok || errStr == "" {
		t.Errorf("Field %s is not a non-empty string: %v.", field, response[field])
		return false
	}

	return true
}

// ValidateErrorMessage checks if the error message contains an expected string.
func ValidateErrorMessage(t *testing.T, response map[string]interface{}, expectedContent string) bool {
	t.Helper()

	errMsg, ok := response["error"].(string)
	if !ok {
		t.Error("Error field is not a string.")
		return false
	}

	if !strings.Contains(strings.ToLower(errMsg), strings.ToLower(expectedContent)) {
		t.Errorf("Error message does not contain expected content: got %q, want to contain %q.", errMsg, expectedContent)
		return false
	}

	return true
}

// ValidateJSONRPCErrorSchema checks if a response follows the JSON-RPC 2.0 error format.
func ValidateJSONRPCErrorSchema(t *testing.T, response map[string]interface{}) bool {
	t.Helper()

	// Check error field exists
	errObj, ok := response["error"]
	if !ok {
		t.Error("Error response missing 'error' field.")
		return false
	}

	// Check error is an object
	errMap, ok := errObj.(map[string]interface{})
	if !ok {
		t.Errorf("Error field is not an object: %T.", errObj)
		return false
	}

	// Check required error object fields
	requiredFields := []string{"code", "message"}
	for _, field := range requiredFields {
		if _, ok := errMap[field]; !ok {
			t.Errorf("Error object missing required field: %s.", field)
			return false
		}
	}

	// Validate field types
	if code, ok := errMap["code"].(float64); !ok {
		t.Errorf("Error code is not a number: %T.", errMap["code"])
		return false
	} else if code == 0 {
		t.Error("Error code should not be 0.")
		return false
	}

	if msg, ok := errMap["message"].(string); !ok {
		t.Errorf("Error message is not a string: %T.", errMap["message"])
		return false
	} else if msg == "" {
		t.Error("Error message should not be empty.")
		return false
	}

	// Check timestamp exists
	if _, ok := response["timestamp"]; !ok {
		t.Error("Error response missing 'timestamp' field.")
		return false
	}

	return true
}

// ValidateListToolsResponse validates the response from list_tools.
func ValidateListToolsResponse(t *testing.T, result map[string]interface{}) bool {
	t.Helper()

	// Check for tools field
	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Errorf("tools is not an array: %v.", result["tools"])
		return false
	}

	// At minimum, we should have at least one tool (authenticate)
	if len(tools) < 1 {
		t.Error("Expected at least one tool.")
		return false
	}

	// Validate each tool
	for i, tool := range tools {
		if !ValidateMCPTool(t, tool) {
			t.Errorf("Tool %d failed validation.", i)
			return false
		}
	}

	// Check for authenticate tool specifically
	authenticateToolFound := false
	for _, tool := range tools {
		toolObj, ok := tool.(map[string]interface{})
		if !ok {
			continue
		}

		if name, ok := toolObj["name"].(string); ok && name == "authenticate" {
			authenticateToolFound = true

			// Verify authenticate tool has a frob argument
			if args, ok := toolObj["arguments"].([]interface{}); ok {
				frobArgFound := false
				for _, arg := range args {
					argObj, ok := arg.(map[string]interface{})
					if !ok {
						continue
					}

					if name, ok := argObj["name"].(string); ok && name == "frob" {
						frobArgFound = true

						// Verify frob argument is required
						if required, ok := argObj["required"].(bool); ok && !required {
							t.Error("frob argument for authenticate tool should be required.")
							return false
						}

						break
					}
				}

				if !frobArgFound {
					t.Error("authenticate tool is missing frob argument.")
					return false
				}
			}

			break
		}
	}

	if !authenticateToolFound {
		t.Error("authenticate tool not found in list_tools response.")
		return false
	}

	return true
}

// Helper functions

// ValidateArgument validates an argument structure.
func ValidateArgument(t *testing.T, index int, arg interface{}) bool {
	t.Helper()

	argObj, ok := arg.(map[string]interface{})
	if !ok {
		t.Errorf("Argument %d is not an object: %T.", index, arg)
		return false
	}

	// Check required fields.
	requiredFields := []string{FieldArgName, FieldArgDescription, FieldArgRequired}
	for _, field := range requiredFields {
		if argObj[field] == nil {
			t.Errorf("Argument %d missing required field: %s.", index, field)
			return false
		}
	}

	// Validate field types.
	_, ok = argObj[FieldArgName].(string)
	if !ok {
		t.Errorf("Argument %d name is not a string.", index)
		return false
	}

	_, ok = argObj[FieldArgDescription].(string)
	if !ok {
		t.Errorf("Argument %d description is not a string.", index)
		return false
	}

	_, ok = argObj[FieldArgRequired].(bool)
	if !ok {
		t.Errorf("Argument %d required is not a boolean.", index)
		return false
	}

	return true
}

// validateResourceNameFormat validates resource name follows 'scheme://path' format.
func validateResourceNameFormat(name string) bool {
	// Check for scheme://path format.
	pattern := regexp.MustCompile(`^[a-z]+://[a-zA-Z0-9\-_\./]+(?:/\{[a-zA-Z0-9\-_]+\})?$`)
	return pattern.MatchString(name)
}

// validateMimeType validates MIME type format.
func validateMimeType(mimeType string) bool {
	// Basic MIME type pattern.
	pattern := regexp.MustCompile(`^[a-z]+/[a-z0-9\-\.\+]+`)
	return pattern.MatchString(mimeType)
}

// FormatValidationError returns a formatted validation error message.
func FormatValidationError(objType, field string, expected, got interface{}) string {
	return fmt.Sprintf("%s %s validation error: expected %v, got %v.",
		objType, field, expected, got)
}
