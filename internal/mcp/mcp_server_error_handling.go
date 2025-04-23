// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// File: internal/mcp/mcp_server_error_handling.go.
package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // Import the mcperrors package.
	// Removed schema import as mapValidationError* helpers are gone.
)

// createErrorResponse creates the byte representation of a JSON-RPC error response.
func (s *Server) createErrorResponse(msgBytes []byte, err error) ([]byte, error) {
	requestID := extractRequestID(s.logger, msgBytes) // Pass logger instance.
	// Use the mapping function from mcperrors package.
	code, message, data := mcperrors.MapMCPErrorToJSONRPC(err)
	s.logErrorDetails(code, message, requestID, data, err) // Pass original err here.

	errorPayload := struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data,omitempty"` // Keep data as interface{} for flexibility.
	}{
		Code:    code,
		Message: message,
		Data:    data, // Assign the potentially enriched data map.
	}

	errorResponse := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   interface{}     `json:"error"`
	}{
		JSONRPC: "2.0",
		ID:      requestID,
		Error:   errorPayload,
	}

	responseBytes, marshalErr := json.Marshal(errorResponse)
	if marshalErr != nil {
		// Log the marshalling error itself, including the original error context if possible.
		s.logger.Error("CRITICAL: Failed to marshal final error response.", "marshalError", fmt.Sprintf("%+v", marshalErr), "originalError", fmt.Sprintf("%+v", err))
		return nil, errors.Wrap(marshalErr, "failed to marshal error response object")
	}

	return responseBytes, nil
}

// extractRequestID attempts to get the ID from raw message bytes.
func extractRequestID(logger logging.Logger, msgBytes []byte) json.RawMessage {
	var request struct {
		ID json.RawMessage `json:"id"`
	}
	// Use a decoder for potentially better error handling if needed, but Unmarshal is fine here.
	_ = json.Unmarshal(msgBytes, &request) // Ignore error, default to null.
	if request.ID != nil {
		// Check for empty JSON array `[]` or object `{}` which are invalid IDs.
		idStr := strings.TrimSpace(string(request.ID))
		if idStr == "[]" || idStr == "{}" {
			// Use the passed-in logger instance.
			logger.Warn("Invalid JSON-RPC ID (array/object) found, treating as null.", "rawId", idStr)
			return json.RawMessage("null")
		}
		return request.ID
	}
	return json.RawMessage("null")
}

// mapErrorToJSONRPCComponents is now DEPRECATED. Use mcperrors.MapMCPErrorToJSONRPC instead.
// Keeping it here commented out temporarily for reference during transition might be useful,
// but ensure it's not called. The call site in createErrorResponse is already updated.
/*
func (s *Server) mapErrorToJSONRPCComponents(logger logging.Logger, err error) (code int, message string, data interface{}) {
	// ... implementation removed ...
}
*/

// --- getErrorString (REMOVED as unused) ---.
// func getErrorString(err error) string { ... }

// --- mapValidationErrorEx (REMOVED as unused) ---.
// func mapValidationErrorEx(validationErr *schema.ValidationError) (int, string, interface{}) { ... }

// --- mapValidationError (REMOVED as unused) ---.
// func mapValidationError(validationErr *schema.ValidationError) (int, string, map[string]interface{}) { ... }

// logErrorDetails logs detailed error information server-side.
func (s *Server) logErrorDetails(code int, message string, requestID json.RawMessage, data interface{}, err error) {
	// Prepare the core log arguments.
	args := []interface{}{
		"jsonrpcErrorCode", code, // 'code' is already an int here.
		"jsonrpcErrorMessage", message,
		"originalError", fmt.Sprintf("%+v", err), // Use %+v for stack trace.
		"requestID", string(requestID),
	}

	// Add the data field if it's not nil.
	// Check if data is a map before trying to access internalCode.
	dataMap, isMap := data.(map[string]interface{})
	if isMap {
		// FIX: Explicitly cast ErrorCode to int here if adding internalCode to logs.
		// Although MapMCPErrorToJSONRPC doesn't currently add internalCode to data map,
		// let's add the cast preventatively if it were added later.
		if internalCode, exists := dataMap["internalCode"]; exists {
			if errCode, ok := internalCode.(mcperrors.ErrorCode); ok {
				args = append(args, "internalCode", int(errCode)) // Apply cast here.
			} else {
				args = append(args, "internalCode", internalCode) // Add as is if not ErrorCode type.
			}
		}
		// Add other data fields.
		args = append(args, "errorData", data) // Log the whole data map.
	} else if data != nil {
		// If data is not a map but not nil, log it directly.
		args = append(args, "errorData", data)
	}

	// Call the logger with the prepared arguments.
	s.logger.Error("Generating JSON-RPC error response.", args...)
} // End of logErrorDetails function.
