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
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
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
*/ // <-- Removed the extra period here.

// getErrorString extracts a string representation from an error.
// This might be moved to mcperrors if generally useful there.
func getErrorString(err error) string {
	rootErr := errors.Cause(err)
	if rootErr != nil {
		return rootErr.Error()
	} else if err != nil {
		return err.Error()
	}
	return ""
}

// mapValidationErrorEx maps schema.ValidationError to JSON-RPC components.
// Assumes mapValidationError never returns nil for the data map.
// This might be moved to mcperrors if generally useful there.
func mapValidationErrorEx(validationErr *schema.ValidationError) (int, string, interface{}) {
	code, message, validationData := mapValidationError(validationErr)
	// Ensure we never return nil data.
	if validationData == nil {
		return code, message, make(map[string]interface{})
	}
	return code, message, validationData
}

// mapValidationError maps schema.ValidationError to JSON-RPC components.
// This might be moved to mcperrors if generally useful there.
func mapValidationError(validationErr *schema.ValidationError) (int, string, map[string]interface{}) {
	data := make(map[string]interface{}) // Initialize data map.
	var code int
	var message string

	// Use specific MCP error codes defined in mcperrors package.
	if validationErr.Code == schema.ErrInvalidJSONFormat {
		code = transport.JSONRPCParseError // -32700.
		message = "Parse error."
		data["detail"] = "The received message is not valid JSON."
	} else if validationErr.InstancePath != "" && (strings.HasPrefix(validationErr.InstancePath, "/params") || strings.HasPrefix(validationErr.InstancePath, "params")) {
		code = transport.JSONRPCInvalidParams // -32602.
		message = "Invalid params."
	} else {
		code = transport.JSONRPCInvalidRequest // -32600.
		message = "Invalid Request."
	}

	// Add common validation details to the map.
	data["validationPath"] = validationErr.InstancePath
	// Use original message from validation error for more specific detail.
	data["validationError"] = validationErr.Message // Use the Message field directly.
	if validationErr.SchemaPath != "" {
		data["schemaPath"] = validationErr.SchemaPath
	}

	// Merge context from validation error if present.
	if validationErr.Context != nil {
		for k, v := range validationErr.Context {
			// Add context key-values, potentially prefixing to avoid collisions if needed.
			contextKey := "context_" + k // Example prefixing.
			if _, exists := data[contextKey]; !exists {
				data[contextKey] = v
			}
			// Handle suggestion specifically if present.
			if k == "suggestion" {
				data["suggestion"] = v // Overwrite if needed, or use the prefixed version.
			}
		}
	}
	return code, message, data
}

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
