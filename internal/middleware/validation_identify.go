// Package middleware provides chainable handlers for processing MCP messages, like validation.
package middleware

// file: internal/middleware/validation_identify.go

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/cockroachdb/errors"
)

// identifyMessage attempts to determine the message type (request, notification, response)
// and extracts the request ID. It performs basic structural validation.
// Returns msgType (schema key hint), reqID (interface{}), error.
// nolint:gocyclo // Function complexity is high due to handling different JSON-RPC structures.
func (m *ValidationMiddleware) identifyMessage(message []byte) (string, interface{}, error) {
	// Ensure validator exists before checking HasSchema.
	if m.validator == nil {
		return "", nil, errors.New("identifyMessage: ValidationMiddleware's validator is nil")
	}

	// Attempt to parse the message into a generic map first.
	var parsed map[string]json.RawMessage
	// Use decoder to handle potential number types correctly during initial parse.
	idDecoder := json.NewDecoder(bytes.NewReader(message))
	idDecoder.UseNumber()
	if err := idDecoder.Decode(&parsed); err != nil {
		// Try to extract ID even if full parse fails, might be partially valid JSON.
		id := m.identifyRequestID(message) // identifyRequestID handles its own errors/logging.
		return "", id, errors.Wrap(err, "identifyMessage: failed to parse message structure")
	}

	// Check for presence of key fields.
	_, idExists := parsed["id"]
	_, methodExists := parsed["method"]
	_, resultExists := parsed["result"]
	_, errorExists := parsed["error"]

	// Extract and validate the ID using the dedicated helper.
	// identifyRequestID returns the validated ID (string, int64, float64) or nil if missing, null, or invalid type.
	id := m.identifyRequestID(message)

	// CRITICAL CHECK: If 'id' key existed but helper returned nil (and raw ID wasn't "null"), it means the ID type was invalid.
	if idExists && id == nil {
		// Check if the raw value was actually "null" before declaring invalid type.
		if idRaw := parsed["id"]; idRaw != nil && string(idRaw) != "null" {
			// This is a violation of the JSON-RPC spec.
			return "", nil, errors.New("Invalid JSON-RPC ID type detected (must be string, number, or null)")
		}
		// If it was explicitly null, id will be nil, which is correct for notifications/some errors.
	}

	// Determine message type based on existing fields.
	if methodExists {
		// Potentially a Request or Notification.
		methodRaw, ok := parsed["method"] // Safe, methodExists is true.
		if !ok || methodRaw == nil {      // Check if key exists AND is not null.
			return "", id, errors.New("identifyMessage: 'method' field exists but is null or missing raw value")
		}
		var method string
		if err := json.Unmarshal(methodRaw, &method); err != nil {
			// ID might be valid even if method isn't, return it.
			return "", id, errors.Wrap(err, "identifyMessage: failed to parse 'method' field as string")
		}
		if method == "" {
			// ID might be valid even if method is empty, return it.
			return "", id, errors.New("identifyMessage: 'method' field cannot be empty string")
		}

		// Check if it's a notification (ID is nil AFTER type validation, OR the ID field was missing entirely).
		if id == nil {
			// Use method name as hint, determineIncomingSchemaType applies fallbacks.
			// The logic for choosing between "notifications/..." and "method_notification"
			// is handled within determineIncomingSchemaType.
			schemaKeyHint := method
			m.logger.Debug("Identified message structure as Notification.", "method", method)
			// Return schema key hint and nil ID for notifications.
			return schemaKeyHint, nil, nil
		}

		// It's a Request (has method and a valid, non-null ID).
		schemaKeyHint := method // Use method name as hint.
		m.logger.Debug("Identified message structure as Request.", "method", method, "id", id)
		// Return schema key hint and the valid ID.
		return schemaKeyHint, id, nil
	} else if resultExists {
		// It's a Success Response.
		if errorExists { // JSON-RPC spec forbids both result and error.
			return "", id, errors.New("identifyMessage: message cannot contain both 'result' and 'error' fields")
		}
		// JSON-RPC spec requires ID for responses (can be null if request ID was null).
		if !idExists {
			// According to spec, ID *must* exist, even if null.
			return "", id, errors.New("identifyMessage: success response message must contain an 'id' field")
		}
		schemaKeyHint := "success_response" // Generic hint for success.
		m.logger.Debug("Identified message structure as Success Response.", "id", id)
		// Return generic hint and the ID (which could be nil if original request ID was null).
		return schemaKeyHint, id, nil
	} else if errorExists {
		// It's an Error Response.
		// ID field might be missing if request was unparseable/invalid before ID extraction.
		// ID might be null if request ID was null.
		if !idExists {
			m.logger.Warn("Received error response without an 'id' field (may occur for parse errors).")
			// Proceed but use nil ID.
		}
		schemaKeyHint := "error_response" // Generic hint for error.
		m.logger.Debug("Identified message structure as Error Response.", "id", id)
		// Return generic hint and the ID (which could be nil).
		return schemaKeyHint, id, nil
	}

	// If none of the key fields (method, result, error) were found.
	return "", id, errors.New("identifyMessage: unable to identify message type (missing method, result, or error field)")
}

// identifyRequestID attempts to extract and validate the JSON-RPC ID from raw message bytes.
// Returns string, int64, float64, or nil. Nil indicates missing, explicitly null, or invalid type.
func (m *ValidationMiddleware) identifyRequestID(message []byte) interface{} {
	var parsed struct {
		ID json.RawMessage `json:"id"`
	}
	// Use decoder to preserve number types if possible.
	decoder := json.NewDecoder(bytes.NewReader(message))
	decoder.UseNumber() // Important: read numbers as json.Number.
	if err := decoder.Decode(&parsed); err != nil {
		// This means the basic structure {"id": ...} couldn't be parsed.
		// Log at debug as this can happen for malformed JSON handled elsewhere.
		m.logger.Debug("Failed to parse base structure for ID extraction", "error", err, "preview", calculatePreview(message))
		return nil // Cannot parse structure.
	}

	if parsed.ID == nil {
		// The "id" key is missing entirely.
		return nil
	}

	// Try unmarshalling the ID field specifically.
	var idValue interface{}
	idDecoder := json.NewDecoder(bytes.NewReader(parsed.ID))
	idDecoder.UseNumber() // Use number again for the specific ID field.
	if err := idDecoder.Decode(&idValue); err != nil {
		// This means the ID field itself contains invalid JSON, e.g., "id": {invalid}.
		m.logger.Warn("Failed to decode ID value itself (invalid JSON in 'id' field).", "rawId", string(parsed.ID), "error", err)
		return nil // Invalid ID content.
	}

	// Check the type of the decoded ID value according to JSON-RPC 2.0 spec.
	switch v := idValue.(type) {
	case json.Number:
		// Try converting to int64 first (most common).
		if i, err := v.Int64(); err == nil {
			return i
		}
		// Try float64 if int64 fails (spec allows numbers).
		if f, err := v.Float64(); err == nil {
			return f
		}
		// If number conversion fails (unlikely for valid json.Number).
		m.logger.Warn("Valid json.Number ID type detected but failed number conversion.", "rawId", string(parsed.ID))
		return nil // Treat as invalid if conversion fails.
	case string:
		return v // Valid string ID.
	case nil:
		// This handles the case where "id": null was explicitly sent.
		return nil // Represent null ID as Go nil.
	default:
		// Invalid types (Arrays, Objects, Booleans) according to JSON-RPC 2.0 spec.
		m.logger.Warn("Invalid JSON-RPC ID type detected.", "rawId", string(parsed.ID), "goType", fmt.Sprintf("%T", v))
		return nil // Signal invalid type by returning nil.
	}
}
