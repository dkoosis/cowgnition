// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/mcp_server_error_handling.go

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging" // Ensure logging is imported.
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
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
// Added logger parameter for potential warnings during mapping.
func (s *Server) mapErrorToJSONRPCComponents(logger logging.Logger, err error) (code int, message string, data interface{}) {
	data = make(map[string]interface{}) // Initialize data as a map.

	var mcpErr *mcperrors.BaseError
	var transportErr *transport.Error
	var validationErr *schema.ValidationError

	// Use errors.Cause to get the root error before checking its string representation.
	rootErr := errors.Cause(err)
	errStr := ""
	if rootErr != nil {
		errStr = rootErr.Error() // Get the string of the root cause, handle nil rootErr.
	} else if err != nil { // Check original error if rootErr is nil but err is not.
		errStr = err.Error()
	}

	// --- Start of error mapping logic ---
	// Check for specific error strings first for method not found/sequence errors.
	if strings.Contains(errStr, "Method not found:") {
		code = transport.JSONRPCMethodNotFound // -32601.
		message = "Method not found."
		methodName := strings.TrimPrefix(errStr, "Method not found: ")
		if methodName != errStr {
			// Safely add to data map.
			if dataMap, ok := data.(map[string]interface{}); ok {
				dataMap["method"] = methodName
				dataMap["detail"] = "The requested method is not supported by this MCP server."
			}
		}
	} else if strings.Contains(errStr, "protocol sequence error:") {
		code = transport.JSONRPCMethodNotFound // -32601 to match expected test value.
		message = "Connection initialization required."
		dataMap := map[string]interface{}{"detail": errStr} // Initialize map.
		if s.connectionState != nil {                       // Add state if available.
			dataMap["state"] = s.connectionState.CurrentState()
		}
		if strings.Contains(errStr, "must first call 'initialize'") {
			dataMap["help"] = "The MCP protocol requires initialize to be called first."
			dataMap["reference"] = "https://modelcontextprotocol.io/docs/concepts/messages/#server-initialization"
		} else if strings.Contains(errStr, "can only be called once") {
			dataMap["help"] = "The initialize method can only be called once per connection."
			dataMap["reference"] = "https://modelcontextprotocol.io/docs/concepts/messages/#server-initialization"
		}
		data = dataMap // Assign the map back to data.
	} else if strings.Contains(errStr, "connection not initialized") {
		code = transport.JSONRPCMethodNotFound // -32601.
		message = "Connection initialization required."
		dataMap := map[string]interface{}{"detail": errStr} // Initialize map.
		if s.connectionState != nil {                       // Add state if available.
			dataMap["state"] = s.connectionState.CurrentState()
		}
		dataMap["help"] = "The MCP protocol requires initialize to be called first."
		data = dataMap // Assign the map back to data.
	} else if errors.As(err, &validationErr) {
		// mapValidationError now returns map[string]interface{} for data.
		var validationData map[string]interface{}
		code, message, validationData = mapValidationError(validationErr)
		data = validationData // Assign the map.
	} else if errors.As(err, &mcpErr) {
		code, message = mapMCPError(mcpErr) // mapMCPError assigns code.
		if mcpErr.Context != nil {
			// Ensure data is a map and merge contexts.
			if dataMap, ok := data.(map[string]interface{}); ok {
				for k, v := range mcpErr.Context {
					if _, exists := dataMap[k]; !exists {
						dataMap[k] = v
					}
				}
			} else if data == nil || (ok && len(dataMap) == 0) { // Check if data is nil or an empty map before overwriting.
				data = mcpErr.Context
			}
		}
	} else if errors.As(err, &transportErr) {
		// MapErrorToJSONRPC returns map[string]interface{}.
		var transportData map[string]interface{}
		code, message, transportData = transport.MapErrorToJSONRPC(transportErr)
		data = transportData // Assign the map.
	} else {
		// Handle generic Go errors.
		code, message = mapGenericGoError(err)
		// Keep data as the initialized empty map unless specific details are added below.
	}
	// --- End of error mapping logic ---

	// Add URL from context if it exists in the original error.
	// This handles cases where the error might not be one of the specific types above
	// but still has URL context added by the schema loader.
	urlContext := errors.GetAllDetails(err)
	if urlVal, ok := urlContext["url"]; ok {
		// Ensure data is a map before trying to add to it.
		if dataMap, ok := data.(map[string]interface{}); ok {
			if _, exists := dataMap["url"]; !exists { // Avoid overwriting.
				dataMap["url"] = urlVal
				data = dataMap // Ensure data points to the modified map.
			}
		} else if data == nil { // If data is nil, create a new map.
			data = map[string]interface{}{"url": urlVal}
		} else {
			// If data exists but is not a map, log a warning.
			logger.Warn("Could not add URL context because existing error data is not a map.", "existingDataType", fmt.Sprintf("%T", data))
		}
	}

	// Ensure data is nil if the map is empty after all checks.
	if dataMap, ok := data.(map[string]interface{}); ok && len(dataMap) == 0 {
		data = nil
	}

	return code, message, data
}

// mapValidationError maps schema.ValidationError to JSON-RPC components.
// Returns code, message, and map[string]interface{} for data.
func mapValidationError(validationErr *schema.ValidationError) (code int, message string, data map[string]interface{}) {
	data = make(map[string]interface{}) // Initialize data map.

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
	// Use a more generic message for the client, log specific internal message.
	// message = mcpErr.Message // Keep internal message for logging.

	// Map specific internal codes to standard or implementation-defined JSON-RPC codes.
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
	// Add mappings for other custom MCP error codes.
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
// Uses explicit key-value pairs for clarity and robustness.
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
