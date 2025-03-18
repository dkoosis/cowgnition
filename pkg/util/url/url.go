// Package url provides URL parsing and manipulation utilities used throughout the CowGnition project.
package url

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ParseResourceURI parses a resource URI into its components.
// Returns the scheme and path parts of the URI.
// For example, "tasks://all" would return "tasks", "all".
func ParseResourceURI(uri string) (scheme, path string, err error) {
	parts := strings.SplitN(uri, "://", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid resource URI format: %s", uri)
	}
	return parts[0], parts[1], nil
}

// ValidateResourceURI checks if a resource URI follows the MCP specification.
// Valid URIs have the format scheme://path or scheme://path/{param}.
func ValidateResourceURI(uri string) bool {
	// Basic regex for scheme://path[/optional/path/segments][/{param}]
	nameRegex := regexp.MustCompile(`^[a-z]+://[a-zA-Z0-9\-_\./]+(?:/\{[a-zA-Z0-9\-_]+\})?$`)
	return nameRegex.MatchString(uri)
}

// ExtractPathParam extracts a path parameter from a resource path that includes a parameter.
// For example, for path "list/{list_id}" and actual path "list/123", it returns "123".
func ExtractPathParam(templatePath, actualPath string) (string, error) {
	// Find the parameter name in the template.
	startIndex := strings.Index(templatePath, "{")
	endIndex := strings.Index(templatePath, "}")

	if startIndex == -1 || endIndex == -1 || startIndex >= endIndex {
		return "", fmt.Errorf("template path does not contain a valid parameter: %s", templatePath)
	}

	// Get the prefix before the parameter.
	prefix := templatePath[:startIndex]

	// Make sure the actual path starts with the same prefix.
	if !strings.HasPrefix(actualPath, prefix) {
		return "", fmt.Errorf("actual path %s does not match template %s", actualPath, templatePath)
	}

	// Extract the parameter value.
	paramValue := actualPath[len(prefix):]

	// If there's content after the parameter in the template, handle that too.
	if endIndex+1 < len(templatePath) {
		suffix := templatePath[endIndex+1:]
		if !strings.HasSuffix(paramValue, suffix) {
			return "", fmt.Errorf("actual path %s does not match template %s", actualPath, templatePath)
		}
		paramValue = paramValue[:len(paramValue)-len(suffix)]
	}

	return paramValue, nil
}

// FindURLEndIndex locates the end of a URL within content starting from startIdx.
func FindURLEndIndex(content string, startIdx int) int {
	endIdx := startIdx

	for i := startIdx; i < len(content); i++ {
		// URL ends at any whitespace or common ending punctuation.
		if content[i] == '\n' || content[i] == '\r' || content[i] == ' ' ||
			content[i] == '"' || content[i] == ')' || content[i] == ']' {
			return i
		}
		endIdx = i
	}

	// If we reach end of content without finding endpoint.
	return endIdx + 1
}

// ExtractQueryParam extracts a specific query parameter from a URL string.
func ExtractQueryParam(urlStr, param string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	return parsedURL.Query().Get(param), nil
}
