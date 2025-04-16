// Package schema handles loading, validation, and error reporting against JSON schemas, specifically MCP.
package schema

// file: internal/schema/helpers.go

import (
	"bytes"
)

// calculatePreview needs to be defined, likely in helpers.go
// Ensure it handles potential errors gracefully.
func calculatePreview(data []byte) string {
	const maxPreviewLen = 100
	if len(data) > maxPreviewLen {
		// Consider replacing control characters for cleaner previews
		previewBytes := bytes.Map(func(r rune) rune {
			if r < 32 || r == 127 {
				return '.'
			}
			return r
		}, data[:maxPreviewLen])
		return string(previewBytes) + "..."
	}
	// Consider replacing control characters here too
	previewBytes := bytes.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return '.'
		}
		return r
	}, data)
	return string(previewBytes)
}
