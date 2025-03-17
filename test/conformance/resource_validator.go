// Package conformance provides tests to verify MCP protocol compliance.
package conformance

import (
	"strings"
)

// findURLEndIndex locates the end of a URL within content starting from startIdx.
func findURLEndIndex(content string, startIdx int) int {
	endIdx := startIdx

	for i := startIdx; i < len(content); i++ {
		// URL ends at any whitespace or common ending punctuation
		if content[i] == '\n' || content[i] == '\r' || content[i] == ' ' ||
			content[i] == '"' || content[i] == ')' || content[i] == ']' {
			return i
		}
		endIdx = i
	}

	// If we reach end of content without finding endpoint
	return endIdx + 1
}

// extractFrobFromURL attempts to extract the frob parameter from a URL.
func extractFrobFromURL(authURL string) string {
	// Implementation would normally use url.Parse, but simplified here
	frobPrefix := "frob="
	idx := strings.Index(authURL, frobPrefix)
	if idx == -1 {
		return ""
	}

	startIdx := idx + len(frobPrefix)
	endIdx := findURLEndIndex(authURL, startIdx)

	return authURL[startIdx:endIdx]
}

// extractFrobFromContent tries to find a frob value within content using known patterns.
func extractFrobFromContent(content string) string {
	// Common patterns that precede a frob in content text
	patterns := []string{
		"frob ",
		"frob: ",
		"Frob: ",
		"frob=",
		"\"frob\": \"",
	}

	for _, pattern := range patterns {
		idx := strings.Index(content, pattern)
		if idx == -1 {
			continue
		}

		startIdx := idx + len(pattern)
		endIdx := findURLEndIndex(content, startIdx)

		if endIdx > startIdx {
			return content[startIdx:endIdx]
		}
	}

	return ""
}

// ExtractAuthInfoFromContent attempts to extract auth URL and frob from content.
func ExtractAuthInfoFromContent(content string) (string, string) {
	// Look for URL in content
	urlIdx := strings.Index(content, "https://www.rememberthemilk.com/services/auth/")
	if urlIdx == -1 {
		return "", ""
	}

	// Extract URL
	endURLIdx := findURLEndIndex(content, urlIdx)
	authURL := content[urlIdx:endURLIdx]

	// Try to extract frob, first from URL then from content text
	frob := extractFrobFromURL(authURL)

	// If frob not found in URL, look in content text
	if frob == "" {
		frob = extractFrobFromContent(content)
	}

	return authURL, frob
}
