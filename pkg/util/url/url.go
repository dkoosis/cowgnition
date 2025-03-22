// Package url provides URL parsing and manipulation utilities used throughout the CowGnition project.
// These utilities are designed to simplify common URL operations and ensure consistency across the codebase.
package url

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ParseResourceURI parses a resource URI into its components.
// It splits the URI string by "://" to separate the scheme and path.
// This function is used to interpret resource URIs according to the MCP specification.
// Returns the scheme and path parts of the URI.
// For example, "tasks://all" would return "tasks", "all".
//
// uri string: The resource URI to parse.
//
// Returns:
// scheme string: The scheme part of the URI.
// path string: The path part of the URI.
// error: An error if the URI format is invalid.
func ParseResourceURI(uri string) (scheme, path string, err error) {
	parts := strings.SplitN(uri, "://", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("ParseResourceURI: invalid resource URI format: %s", uri)
	}
	return parts[0], parts[1], nil
}

// ValidateResourceURI checks if a resource URI follows the MCP specification.
// Valid URIs have the format scheme://path or scheme://path/{param}.
// This validation is crucial for ensuring compatibility with MCP clients and preventing unexpected behavior.
//
// uri string: The resource URI to validate.
//
// Returns:
// bool: True if the URI is valid, false otherwise.
func ValidateResourceURI(uri string) bool {
	// Basic regex for scheme://path[/optional/path/segments][/{param}]
	nameRegex := regexp.MustCompile(`^[a-z]+://[a-zA-Z0-9\-_\./]+(?:/\{[a-zA-Z0-9\-_]+\})?$`)
	return nameRegex.MatchString(uri)
}

// ExtractPathParam extracts a path parameter from a resource path that includes a parameter.
// For example, for path "list/{list_id}" and actual path "list/123", it returns "123".
// This function is used to retrieve specific parameter values from resource paths, which is necessary for handling dynamic resource requests.
//
// templatePath string: The path template with the parameter (e.g., "list/{list_id}").
// actualPath string: The actual path with the parameter value (e.g., "list/123").
//
// Returns:
// string: The extracted parameter value.
// error: An error if the template path is invalid or the actual path doesn't match the template.
func ExtractPathParam(templatePath, actualPath string) (string, error) {
	// Find the parameter name in the template.
	startIndex := strings.Index(templatePath, "{")
	endIndex := strings.Index(templatePath, "}")

	if startIndex == -1 || endIndex == -1 || startIndex >= endIndex {
		return "", fmt.Errorf("ExtractPathParam: template path does not contain a valid parameter: %s", templatePath)
	}

	// Get the prefix before the parameter.
	prefix := templatePath[:startIndex]

	// Make sure the actual path starts with the same prefix.
	if !strings.HasPrefix(actualPath, prefix) {
		return "", fmt.Errorf("ExtractPathParam: actual path %s does not match template %s", actualPath, templatePath)
	}

	// Extract the parameter value.
	paramValue := actualPath[len(prefix):]

	// If there's content after the parameter in the template, handle that too.
	if endIndex+1 < len(templatePath) {
		suffix := templatePath[endIndex+1:]
		if !strings.HasSuffix(paramValue, suffix) {
			return "", fmt.Errorf("ExtractPathParam: actual path %s does not match template %s", actualPath, templatePath)
		}
		paramValue = paramValue[:len(paramValue)-len(suffix)]
	}

	return paramValue, nil
}

// FindURLEndIndex locates the end of a URL within content starting from startIdx.
// This is used to correctly parse URLs from larger text blocks, which is necessary for interpreting user input or data from external sources.
//
// content string: The string containing the URL.
// startIdx int: The index in the string where the URL starts.
//
// Returns:
// int: The index of the end of the URL.
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
// This function is used to easily access query parameters within a URL, avoiding manual string parsing.
//
// urlStr string: The URL string to extract from.
// param string: The name of the query parameter to extract.
//
// Returns:
// string: The value of the extracted query parameter.
// error: An error if the URL parsing fails.
func ExtractQueryParam(urlStr, param string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("ExtractQueryParam: failed to parse URL: %w", err)
	}

	return parsedURL.Query().Get(param), nil
}

// DocEnhanced:2025-03-21
