// Package validation provides validation utilities used throughout the CowGnition project.
package validation

import (
	"regexp"
	"strings"
)

// ValidateMimeType checks if a MIME type is in a valid format.
func ValidateMimeType(mimeType string) bool {
	// More robust MIME type validation using a regular expression.
	mimeRegex := regexp.MustCompile(`^[a-z]+/[a-z0-9\-\.\+]*(;\s?[a-z0-9\-\.]+\s*=\s*[a-z0-9\-\.]+)*$`)
	return mimeRegex.MatchString(mimeType)
}

// ValidateToolName checks if a tool name follows the MCP specification.
// Valid tool names are lowercase alphanumeric with underscores and should be descriptive.
func ValidateToolName(name string) bool {
	// Tool names should be lowercase alphanumeric with underscores.
	nameRegex := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	return nameRegex.MatchString(name)
}

// ValidateJSON validates if a string contains valid JSON.
// This is a lightweight check for common JSON syntax errors.
func ValidateJSON(jsonStr string) bool {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return false
	}
	
	// Very basic check - proper JSON should start with { or [ and end with } or ].
	if (strings.HasPrefix(jsonStr, "{") && strings.HasSuffix(jsonStr, "}")) ||
	   (strings.HasPrefix(jsonStr, "[") && strings.HasSuffix(jsonStr, "[")) {
		return true
	}
	
	return false
}

// ValidateRequired checks if all required fields exist in a map.
// Returns a slice of missing field names, or nil if all fields are present.
func ValidateRequired(data map[string]interface{}, requiredFields []string) []string {
	var missing []string
	
	for _, field := range requiredFields {
		if _, exists := data[field]; !exists {
			missing = append(missing, field)
		}
	}
	
	if len(missing) == 0 {
		return nil
	}
	
	return missing
}
