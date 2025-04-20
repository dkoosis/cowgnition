// Package middleware provides chainable handlers for processing MCP messages, like validation.
package middleware

// file: internal/middleware/validation_identify.go.

import (
	"encoding/json"
	// Keep fmt import.
	"github.com/cockroachdb/errors"
)

// identifyMessage analyzes the message structure to determine its type (method name or response type) and ID.
// It checks for basic JSON-RPC validity but relies on schema validation for full parameter/result checking.
// Returns: msgTypeHint (string), requestID (interface{}), error (if basic structure is invalid).
// nolint:gocyclo // Complexity is inherent in checking multiple valid JSON-RPC structures.
func identifyMessage(message []byte) (string, interface{}, error) {
	var baseMsg map[string]json.RawMessage // Use RawMessage for flexibility.
	if err := json.Unmarshal(message, &baseMsg); err != nil {
		// This shouldn't happen if json.Valid() passed earlier, but handle defensively.
		return "", nil, errors.Wrap(err, "failed to parse previously validated JSON")
	}

	// Extract ID - must be string, number, or null if present.
	var reqID interface{}
	if idJSON, ok := baseMsg["id"]; ok {
		// Need to unmarshal the RawMessage to check the actual type.
		var idValue interface{}
		if err := json.Unmarshal(idJSON, &idValue); err != nil {
			return "", nil, errors.Wrap(err, "failed to parse message 'id' field")
		}

		switch idVal := idValue.(type) {
		case string:
			reqID = idVal
		case float64: // JSON numbers are decoded as float64 by default.
			reqID = idVal
		case nil:
			reqID = nil // Explicit null is allowed.
		default:
			// Reject bool, object, array IDs.
			return "", nil, errors.Newf("Invalid JSON-RPC ID type detected: %T", idVal)
		}
	} else {
		// No ID present. This is okay for Notifications, but not Responses.
		reqID = nil
	}

	// Check for method (Request/Notification) vs result/error (Response).
	_, hasMethod := baseMsg["method"]
	_, hasResult := baseMsg["result"]
	_, hasError := baseMsg["error"]

	if hasMethod {
		// --- Request or Notification ---
		if hasResult || hasError {
			return "", reqID, errors.New("request/notification message cannot contain 'result' or 'error'")
		}
		var methodStr string
		if err := json.Unmarshal(baseMsg["method"], &methodStr); err != nil || methodStr == "" {
			return "", reqID, errors.Wrap(err, "invalid or missing 'method' field for request/notification")
		}

		// Type hint is the method name itself.
		// ID presence distinguishes Request from Notification implicitly for schema lookup later if needed.
		return methodStr, reqID, nil
	} else if hasResult || hasError {
		// --- Response ---
		if hasResult && hasError {
			return "", reqID, errors.New("response message cannot contain both 'result' and 'error'")
		}
		// JSON-RPC spec requires responses (success or error) to have an ID, though null is allowed.
		// Let's check if ID is present *at all* (even if null).
		if _, idExists := baseMsg["id"]; !idExists {
			// Distinguish missing ID for success vs error response types.
			if hasResult {
				return "", reqID, errors.New("success response message must contain an 'id' field (even if null)")
			}
			// Allow error responses without ID, per JSON-RPC spec flexibility, treat ID as null.
			reqID = nil // Ensure reqID is nil if explicitly missing in error response.
		}

		if hasResult {
			// It's a success response. Schema lookup might use request method from context if available.
			return "success_response", reqID, nil // Generic type hint.
		}
		// It's an error response.
		return "error_response", reqID, nil // Generic type hint.
	}
	// Neither method nor result/error found - invalid structure.
	return "", reqID, errors.New("message must contain 'method' (for request/notification) or 'result'/'error' (for response)")
}
