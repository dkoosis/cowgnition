// Package stringutil provides string manipulation utilities used throughout the CowGnition project.
// file: pkg/util/stringutil/stringutil.go
// pkg/util/stringutil/stringutil.go
package stringutil

import (
	"strings"
)

// CoalesceString returns the first non-empty string from the provided strings.
// This is useful for providing default values.
// If all strings are empty, it returns an empty string.
func CoalesceString(strs ...string) string {
	for _, str := range strs {
		if str != "" {
			return str
		}
	}
	return ""
}

// TruncateString truncates a string to the specified length, adding an ellipsis if truncated.
// This function is designed to prevent buffer overflows and provide user-friendly output.
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ExtractBetween extracts a substring between two delimiter strings.
// Returns an error with a message if the delimiters are not found.
// This is used to parse structured data within strings.
func ExtractBetween(s, startDelim, endDelim string) (string, error) {
	startIdx := strings.Index(s, startDelim)
	if startIdx == -1 {
		return "", fmt.Errorf("ExtractBetween: start delimiter '%s' not found", startDelim)
	}

	startIdx += len(startDelim)
	endIdx := strings.Index(s[startIdx:], endDelim)
	if endIdx == -1 {
		return "", fmt.Errorf("ExtractBetween: end delimiter '%s' not found after start delimiter", endDelim)
	}

	return s[startIdx : startIdx+endIdx], nil
}

// ExtractFromContent tries to find a value using common patterns.
// Useful for extracting values like frobs from content text.
// This function is designed to be flexible and handle various input formats.
func ExtractFromContent(content string, patternsstring) string {
	for _, pattern := range patterns {
		idx := strings.Index(content, pattern)
		if idx == -1 {
			continue
		}

		startIdx := idx + len(pattern)
		endIdx := startIdx

		// Find the end of the value.
		for i := startIdx; i < len(content); i++ {
			// Value ends at any whitespace or common ending punctuation.
			if content[i] == '\n' || content[i] == '\r' || content[i] == ' ' ||
				content[i] == '"' || content[i] == ')' || content[i] == ']' {
				endIdx = i
				break
			}
			endIdx = i + 1
		}

		if endIdx > startIdx {
			return content[startIdx:endIdx]
		}
	}

	return ""
}

// DocEnhanced:2025-03-25
