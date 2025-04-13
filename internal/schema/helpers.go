// file: internal/schema/helpers.go
package schema

import (
	"bytes"
)

// calculatePreview generates a string preview of a byte slice, limited to a max length.
func calculatePreview(data []byte) string {
	const maxPreviewLen = 100 // Use a constant for the max length.
	previewLen := len(data)
	if previewLen > maxPreviewLen {
		previewLen = maxPreviewLen
	}
	// Replace non-printable characters for cleaner logging/error messages potentially
	// This is a simple replacement, more robust handling might be needed.
	previewBytes := bytes.Map(func(r rune) rune {
		if r < 32 || r == 127 { // Control characters + DEL
			return '.' // Replace with dot
		}
		return r
	}, data[:previewLen])

	return string(previewBytes)
}
