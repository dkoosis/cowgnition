// Package stringutil provides string manipulation utilities used throughout the CowGnition project.
package stringutil

import (
	"strings"
)

// CoalesceString returns the first non-empty string from the provided strings.
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
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ExtractBetween extracts a substring between two delimiter strings.
// Returns an empty string if the delimiters are not found.
func ExtractBetween(s, startDelim, endDelim string) string {
	startIdx := strings.Index(s, startDelim)
	if startIdx == -1 {
		return ""
	}

	startIdx += len(startDelim)
	endIdx := strings.Index(s[startIdx:], endDelim)
	if endIdx == -1 {
		return ""
	}

	return s[startIdx : startIdx+endIdx]
}

// ExtractFromContent tries to find a value using common patterns.
// Useful for extracting values like frobs from content text.
func ExtractFromContent(content string, patterns []string) string {
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
