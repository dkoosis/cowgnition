// Package middleware provides chainable handlers for processing MCP messages, like validation.
package middleware

// file: internal/middleware/validation_helpers.go

import (
	"bytes"
	"encoding/json"

	"github.com/dkoosis/cowgnition/internal/schema"
)

// calculatePreview generates a short, safe preview of byte data for logging.
func calculatePreview(data []byte) string {
	const maxPreviewLen = 100
	previewLen := len(data)
	suffix := ""
	if previewLen > maxPreviewLen {
		previewLen = maxPreviewLen
		suffix = "..."
	}
	// Replace non-printable characters (including newline, tab etc.) with '.'
	// for cleaner single-line logs.
	previewBytes := bytes.Map(func(r rune) rune {
		if r < ' ' || r == 127 { // Control characters and DEL.
			return '.'
		}
		return r
	}, data[:previewLen])
	return string(previewBytes) + suffix
}

// isErrorResponse checks if a message appears to be a JSON-RPC error response.
func isErrorResponse(message []byte) bool {
	// Simple check for presence of "error" key at the top level, absence of "result".
	// A more robust check would parse the JSON, but this is usually sufficient for skipping.
	// Use Contains because the value could be null, an object, etc.
	return bytes.Contains(message, []byte(`"error":`)) && !bytes.Contains(message, []byte(`"result":`))
}

// isSuccessResponse checks if a message appears to be a JSON-RPC success response.
// Note: Currently unused but kept for completeness.
// nolint:unused
func isSuccessResponse(message []byte) bool {
	// Simple check for presence of "result" key at the top level, absence of "error".
	return bytes.Contains(message, []byte(`"result":`)) && !bytes.Contains(message, []byte(`"error":`))
}

// isCallToolResultShape checks heuristically if a response looks like a CallToolResult.
// Used in determineOutgoingSchemaType as a fallback check.
func isCallToolResultShape(message []byte) bool {
	// Checks for `"result": { ... "content": ... }` structure.
	// This is a heuristic and might match other responses, but good enough for a fallback.
	// A full parse would be more accurate but slower.
	return bytes.Contains(message, []byte(`"result":`)) && bytes.Contains(message, []byte(`"content":`))
}

// performToolNameValidation checks tool names within a `tools/list` response.
// This is called specifically when outgoing validation fails for a tools response,
// providing extra diagnostic logging.
func (m *ValidationMiddleware) performToolNameValidation(responseBytes []byte) {
	// Attempt to parse only the relevant part of the response.
	var toolsResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}

	// Use decoder for potentially better error messages than Unmarshal.
	decoder := json.NewDecoder(bytes.NewReader(responseBytes))
	if err := decoder.Decode(&toolsResp); err != nil {
		// Log at debug level as this is supplementary diagnostics.
		m.logger.Debug("Could not parse response as tool list for name validation.",
			"error", err,
			"responsePreview", calculatePreview(responseBytes))
		return
	}

	// Iterate through the tools and validate names using schema package rules.
	for i, tool := range toolsResp.Result.Tools {
		if err := schema.ValidateName(schema.EntityTypeTool, tool.Name); err != nil {
			// Log as error because it indicates non-compliance found *during* failure analysis.
			m.logger.Error("Invalid tool name found in outgoing response during failure diagnostics.",
				"toolIndex", i,
				"invalidName", tool.Name,
				"validationError", err,
				"rulesHint", schema.GetNamePatternDescription(schema.EntityTypeTool))
			// NOTE: This currently only logs. Standard MCP doesn't strictly enforce name format,
			// but this helps debug why a tools/list_response might fail validation.
		}
	}
}
