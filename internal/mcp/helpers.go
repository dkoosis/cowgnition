// file: internal/mcp/helpers.go

package mcp

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

// mustMarshalJSONToString marshals v to a JSON string (indented) and panics on error.
// Used for returning JSON resource contents as text.
func mustMarshalJSONToString(v interface{}) string {
	bytes, err := json.MarshalIndent(v, "", "  ") // Use indent for readability.
	if err != nil {
		// Panic is acceptable here if the input v is expected to be always marshalable.
		panic(fmt.Sprintf("failed to marshal JSON to string: %v", err))
	}
	return string(bytes)
}
