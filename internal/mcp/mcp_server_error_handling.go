// file: internal/mcp/mcp_server_error_handling.go.
package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // Import the shared types package.
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
)

// createErrorResponse creates the byte representation of a JSON-RPC error response.
func (s *Server) createErrorResponse(msgBytes []byte, err error) ([]byte, error) {
	requestID := extractRequestID(s.logger, msgBytes)                   // Pass logger instance.
	code, message, data := s.mapErrorToJSONRPCComponents(s.logger, err) // Pass logger instance. data is map[string]interface{} or nil.
	s.logErrorDetails(code, message, requestID, data, err)              // Pass original err here.

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

// mapErrorToJSONRPCComponents maps Go errors to JSON-RPC code, message, and optional data.
// It orchestrates multiple specialized error handlers for different error types.
func (s *Server) mapErrorToJSONRPCComponents(logger logging.Logger, err error) (code int, message string, data interface{}) {
	// Initialize empty data map.
	data = make(map[string]interface{})

	// Handle nil error case explicitly.
	if err == nil {
		code = transport.JSONRPCInternalError
		message = "An internal server error occurred (nil error passed)."
		return code, message, data
	}

	// Extract the error string for pattern matching.
	errStr := getErrorString(err)

	// Pattern-based handling first (higher precedence).
	if isMethodNotFoundError(errStr) {
		code, message, data = s.mapMethodNotFoundError(errStr)
	} else if isProtocolSequenceError(errStr) {
		code, message, data = s.mapProtocolSequenceError(errStr)
	} else if isConnectionNotInitializedError(errStr) {
		code, message, data = s.mapConnectionNotInitializedError(errStr)
	} else {
		// Type-based handling second (fallback).
		code, message, data = s.mapErrorByType(logger, err) // Calls the refactored function below.
	}

	// Enrich with URL context if present.
	// data = s.enrichWithURLContext(logger, err, data) // <<<< Call site remains commented out for clean build.

	// Clean up empty data maps.
	data = cleanupEmptyDataMap(data)

	return code, message, data
}

// getErrorString extracts a string representation from an error.
func getErrorString(err error) string {
	rootErr := errors.Cause(err)
	if rootErr != nil {
		return rootErr.Error()
	} else if err != nil {
		return err.Error()
	}
	return ""
}

// isMethodNotFoundError checks if an error string indicates a method not found error.
func isMethodNotFoundError(errStr string) bool {
	return strings.Contains(errStr, "Method not found:")
}

// isProtocolSequenceError checks if an error string indicates a protocol sequence error.
func isProtocolSequenceError(errStr string) bool {
	return strings.Contains(errStr, "protocol sequence error:")
}

// isConnectionNotInitializedError checks if an error string indicates a connection not initialized error.
func isConnectionNotInitializedError(errStr string) bool {
	return strings.Contains(errStr, "connection not initialized")
}

// mapMethodNotFoundError maps a method not found error to JSON-RPC components.
func (s *Server) mapMethodNotFoundError(errStr string) (int, string, interface{}) {
	code := transport.JSONRPCMethodNotFound // -32601.
	message := "Method not found."

	dataMap := map[string]interface{}{}
	methodName := strings.TrimPrefix(errStr, "Method not found: ")
	if methodName != errStr {
		dataMap["method"] = methodName
		dataMap["detail"] = "The requested method is not supported by this MCP server."
	}

	return code, message, dataMap
}

// mapProtocolSequenceError maps a protocol sequence error to JSON-RPC components.
func (s *Server) mapProtocolSequenceError(errStr string) (int, string, interface{}) {
	code := transport.JSONRPCMethodNotFound // -32601 to match expected test value.
	message := "Connection initialization required."

	dataMap := map[string]interface{}{
		"detail": errStr,
	}

	// Safe access to connection state.
	if s.connectionState != nil {
		dataMap["state"] = s.connectionState.CurrentState()
	}

	if strings.Contains(errStr, "must first call 'initialize'") {
		dataMap["help"] = "The MCP protocol requires initialize to be called first."
		dataMap["reference"] = "https://modelcontextprotocol.io/docs/concepts/messages/#server-initialization"
	} else if strings.Contains(errStr, "can only be called once") {
		dataMap["help"] = "The initialize method can only be called once per connection."
		dataMap["reference"] = "https://modelcontextprotocol.io/docs/concepts/messages/#server-initialization"
	}

	return code, message, dataMap
}

// mapConnectionNotInitializedError maps a connection not initialized error to JSON-RPC components.
func (s *Server) mapConnectionNotInitializedError(errStr string) (int, string, interface{}) {
	code := transport.JSONRPCMethodNotFound // -32601.
	message := "Connection initialization required."

	dataMap := map[string]interface{}{
		"detail": errStr,
		"help":   "The MCP protocol requires initialize to be called first.",
	}

	// Safe access to connection state.
	if s.connectionState != nil {
		dataMap["state"] = s.connectionState.CurrentState()
	}

	return code, message, dataMap
}

// --- REFACTORED FUNCTION ---.
// mapErrorByType maps errors by their type to JSON-RPC components.
func (s *Server) mapErrorByType(logger logging.Logger, err error) (int, string, interface{}) {
	var validationErr *schema.ValidationError
	var mcpErr *mcperrors.BaseError
	var transportErr *transport.Error

	if errors.As(err, &validationErr) {
		return mapValidationErrorEx(validationErr)
	}
	if errors.As(err, &mcpErr) { // Use if, not else if.
		return s.mapMCPErrorEx(mcpErr)
	}
	if errors.As(err, &transportErr) { // Use if, not else if.
		return mapTransportError(transportErr)
	}

	// Handle generic Go errors (this code is now outdented).
	code, message := mapGenericGoError(err)
	// Log unknown error types for debugging purposes.
	logger.Debug("Mapping generic Go error to JSON-RPC error.",
		"errorType", fmt.Sprintf("%T", err),
		"errorMessage", err.Error())
	return code, message, make(map[string]interface{})
}

// --- END REFACTORED FUNCTION ---.

// mapValidationErrorEx maps schema.ValidationError to JSON-RPC components.
// Assumes mapValidationError never returns nil for the data map.
func mapValidationErrorEx(validationErr *schema.ValidationError) (int, string, interface{}) {
	code, message, validationData := mapValidationError(validationErr)
	// Ensure we never return nil data.
	if validationData == nil {
		return code, message, make(map[string]interface{})
	}
	return code, message, validationData
}

// mapMCPErrorEx maps mcperrors.BaseError to JSON-RPC components with context merging.
func (s *Server) mapMCPErrorEx(mcpErr *mcperrors.BaseError) (int, string, interface{}) {
	code, message := mapMCPError(mcpErr)
	data := make(map[string]interface{})

	// Safely merge context if available.
	if mcpErr != nil && mcpErr.Context != nil {
		for k, v := range mcpErr.Context {
			// Skip nil values to avoid unexpected behavior.
			if v != nil {
				data[k] = v
			}
		}
	}

	return code, message, data
}

// mapTransportError maps transport.Error to JSON-RPC components.
func mapTransportError(transportErr *transport.Error) (int, string, interface{}) {
	code, message, data := transport.MapErrorToJSONRPC(transportErr)
	// Provide fallback if transport package returns nil data.
	if data == nil {
		return code, message, make(map[string]interface{})
	}
	return code, message, data
}

// enrichWithURLContext attempts to add URL information from error context (REVISED).
// nolint:unused // Currently commented out in the call site.
func (s *Server) enrichWithURLContext(logger logging.Logger, err error, data interface{}) interface{} {
	// Protect against nil inputs.
	if err == nil {
		logger.Debug("enrichWithURLContext: Input error is nil, returning original data.")
		return data
	}

	// Get details from the error.
	details := errors.GetAllDetails(err)
	if details == nil {
		logger.Debug("enrichWithURLContext: No details found in error, returning original data.")
		return data
	}

	// Convert details ([]string) into a map[string]interface{}.
	detailsMap := make(map[string]interface{})
	for _, detail := range details {
		// Assuming details are in "key=value" format.
		parts := strings.SplitN(detail, "=", 2)
		if len(parts) == 2 {
			detailsMap[parts[0]] = parts[1]
		}
	}

	// Get the URL value if it exists using the original string literal key.
	urlValue, exists := detailsMap["url"]

	if !exists {
		logger.Debug("enrichWithURLContext: 'url' key not found in error details, returning original data.")
		return data
	}
	if urlValue == nil {
		logger.Debug("enrichWithURLContext: 'url' key found but value is nil, returning original data.")
		return data
	}

	// Log the found URL value and its type.
	logger.Debug("enrichWithURLContext: Found 'url' in error details.", "urlValue", urlValue, "urlType", fmt.Sprintf("%T", urlValue))

	// Handle different data types safely.
	switch typedData := data.(type) {
	case map[string]interface{}:
		logger.Debug("enrichWithURLContext: Input data is a map.")
		// Only add URL if it doesn't already exist.
		if _, urlKeyExists := typedData["url"]; !urlKeyExists {
			logger.Debug("enrichWithURLContext: Adding 'url' key to existing map.")
			// Explicitly assign the interface{} value. No conversion should happen here.
			typedData["url"] = urlValue
		} else {
			logger.Debug("enrichWithURLContext: 'url' key already exists in map, not overwriting.")
		}
		return typedData // Return the modified or original map.
	case nil:
		logger.Debug("enrichWithURLContext: Input data is nil, creating new map for 'url'.")
		// Create new map if data is nil.
		// Explicitly assign the interface{} value. No conversion should happen here.
		newMap := map[string]interface{}{"url": urlValue}
		return newMap
	default:
		// Log warning for unsupported data type.
		logger.Warn("enrichWithURLContext: Could not add URL context because existing error data is not a map.",
			"existingDataType", fmt.Sprintf("%T", data))
		return data // Return original data if it's not a map or nil.
	}
}

// cleanupEmptyDataMap returns nil if data is an empty map.
func cleanupEmptyDataMap(data interface{}) interface{} {
	if dataMap, ok := data.(map[string]interface{}); ok && len(dataMap) == 0 {
		return nil
	}
	return data
}

// mapValidationError maps schema.ValidationError to JSON-RPC components.
func mapValidationError(validationErr *schema.ValidationError) (int, string, map[string]interface{}) {
	data := make(map[string]interface{}) // Initialize data map.
	var code int
	var message string

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
	data["validationError"] = validationErr.Message
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

// mapMCPError maps mcperrors.BaseError to JSON-RPC code and message.
func mapMCPError(mcpErr *mcperrors.BaseError) (code int, message string) {
	switch mcpErr.Code {
	case mcperrors.ErrProtocolInvalid:
		code = transport.JSONRPCInvalidRequest // -32600.
		message = "Invalid request structure."
	case mcperrors.ErrResourceNotFound:
		code = -32001                   // Example custom code.
		message = "Resource not found." // Use a standard message for this code.
	case mcperrors.ErrAuthFailure:
		code = -32002                      // Example custom code.
		message = "Authentication failed." // Use a standard message.
	case mcperrors.ErrRTMAPIFailure:
		code = -32010                           // Example custom code.
		message = "RTM API interaction failed." // Use a standard message.
	default:
		// Check if the code is already in the implementation-defined range.
		if mcpErr.Code >= -32099 && mcpErr.Code <= -32000 {
			code = mcpErr.Code
			// Keep the original message for custom codes if desired, or standardize.
			message = mcpErr.Message
		} else {
			code = -32000                                  // Fallback generic implementation-defined server error.
			message = "An internal server error occurred." // Use standard message.
		}
	}
	return code, message
}

// mapGenericGoError maps generic Go errors.
func mapGenericGoError(_ error) (code int, message string) {
	code = transport.JSONRPCInternalError // -32603.
	message = "An unexpected internal server error occurred."
	return code, message
}

// logErrorDetails logs detailed error information server-side.
func (s *Server) logErrorDetails(code int, message string, requestID json.RawMessage, data interface{}, err error) {
	// Prepare the core log arguments.
	args := []interface{}{
		"jsonrpcErrorCode", code,
		"jsonrpcErrorMessage", message,
		"originalError", fmt.Sprintf("%+v", err), // Use %+v for stack trace.
		"requestID", string(requestID),
	}

	// Add the data field if it's not nil.
	if data != nil {
		args = append(args, "errorData", data)
	}

	// Call the logger with the prepared arguments.
	s.logger.Error("Generating JSON-RPC error response.", args...)
}
