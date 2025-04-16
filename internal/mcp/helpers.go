// Package mcp implements the Model Context Protocol server logic, including handlers and types.

package mcp

// file: internal/mcp/helpers.go

import (
	"encoding/json"
	"fmt"
)

// mustMarshalJSON marshals v to JSON and panics on error. Used for static schemas.
func mustMarshalJSON(v interface{}) json.RawMessage {
	bytes, err := json.Marshal(v)
	if err != nil {
		// Panic is acceptable here because it indicates a programming error
		// (invalid static schema definition) during initialization.
		panic(fmt.Sprintf("failed to marshal static JSON schema: %v", err))
	}
	return json.RawMessage(bytes)
}
