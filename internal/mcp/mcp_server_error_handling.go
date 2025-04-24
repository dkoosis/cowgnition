// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // Import the mcperrors package.
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"       // Needed for JSONRPCErrorContainer
)

// createErrorResponse creates the byte representation of a JSON-RPC error response.
// MODIFIED: Now accepts responseID as an argument and uses it directly.
func (s *Server) createErrorResponse(_ []byte, originalErr error, responseID json.RawMessage) ([]byte, error) {
	// Input validation: Ensure responseID is never nil here (should be "0" if original was null)
	if responseID == nil {
		s.logger.Error("CRITICAL: createErrorResponse called with nil responseID. Defaulting to '0'.")
		responseID = json.RawMessage("0")
		// Add originalErr to the error context
		originalErr = errors.Wrap(originalErr, "programmer error: createErrorResponse called with nil ID")
	}

	// Use the mapping function from mcperrors package to get code, message, data.
	code, message, data := mcperrors.MapMCPErrorToJSONRPC(originalErr)

	// Log details before marshalling. Use the definitive responseID.
	s.logErrorDetails(code, message, responseID, data, originalErr) // Pass original err here.

	// Construct the payload part of the error
	errorPayload := mcptypes.JSONRPCErrorPayload{ // Use type from mcptypes
		Code:    code,
		Message: message,
		Data:    data, // Assign the potentially enriched data map.
	}

	// Construct the full error response container
	errorResponse := mcptypes.JSONRPCErrorContainer{ // Use type from mcptypes
		JSONRPC: "2.0",
		ID:      responseID, // <<< USE THE PASSED-IN responseID HERE
		Error:   errorPayload,
	}

	// Marshal the final response object
	responseBytes, marshalErr := json.Marshal(errorResponse)
	if marshalErr != nil {
		// Log the marshalling error itself, including the original error context if possible.
		s.logger.Error("CRITICAL: Failed to marshal final error response.",
			"targetID", string(responseID),
			"marshalError", fmt.Sprintf("%+v", marshalErr),
			"originalError", fmt.Sprintf("%+v", originalErr),
		)
		// Wrap the marshalling error but potentially include context about the original failure
		return nil, errors.Wrapf(marshalErr, "failed to marshal error response object for original error: %v", originalErr)
	}

	return responseBytes, nil
}

// --- Helper Functions (Unchanged for this specific fix) ---

// extractRequestID attempts to get the ID from raw message bytes.
// Returns json.RawMessage("null") if ID is missing, null, or invalid JSON type.
func extractRequestID(logger logging.Logger, msgBytes []byte) json.RawMessage {
	var request struct {
		ID json.RawMessage `json:"id"`
	}
	// Use a decoder for potentially better error handling if needed, but Unmarshal is fine here.
	// We ignore unmarshal error because if the message is invalid, we can't get ID anyway.
	_ = json.Unmarshal(msgBytes, &request)

	if request.ID != nil {
		// Check for empty JSON array `[]` or object `{}` which are invalid IDs according to JSON-RPC 2.0.
		idStr := strings.TrimSpace(string(request.ID))
		// Also check for boolean true/false which are also invalid.
		// Number, String, or Null are valid types for the ID field *value*.
		// MCP is stricter and disallows null in practice.
		if idStr == "[]" || idStr == "{}" || idStr == "true" || idStr == "false" {
			logger.Warn("Invalid JSON-RPC ID type (array/object/boolean) found, treating as null for extraction.", "rawId", idStr)
			return json.RawMessage("null") // Treat invalid type as null for purpose of ID determination
		}
		// Return valid-looking ID (string, number, or actual null if explicitly sent)
		return request.ID
	}
	// ID field was missing entirely
	return json.RawMessage("null")
}

// logErrorDetails logs detailed error information server-side.
func (s *Server) logErrorDetails(code int, message string, responseID json.RawMessage, data interface{}, err error) {
	// Prepare the core log arguments.
	args := []interface{}{
		"jsonrpcErrorCode", code,
		"jsonrpcErrorMessage", message,
		"originalError", fmt.Sprintf("%+v", err), // Use %+v for stack trace.
		"responseIDUsed", string(responseID), // Log the ID actually used in the response
	}

	// Add the data field if it's not nil.
	dataMap, isMap := data.(map[string]interface{})
	if isMap {
		if internalCode, exists := dataMap["internalCode"]; exists {
			if errCode, ok := internalCode.(mcperrors.ErrorCode); ok {
				args = append(args, "internalCode", int(errCode)) // Cast if it's our internal type
			} else {
				args = append(args, "internalCode", internalCode) // Add as is otherwise
			}
		}
		// Add other data fields if the map isn't too large or sensitive
		args = append(args, "errorData", data) // Log the whole data map
	} else if data != nil {
		// If data is not a map but not nil, log it directly.
		args = append(args, "errorData", data)
	}

	// Call the logger with the prepared arguments.
	s.logger.Error("Generating JSON-RPC error response.", args...)
}
