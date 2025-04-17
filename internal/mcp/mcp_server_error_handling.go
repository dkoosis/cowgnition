// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/mcp_server_error_handling.go

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
)

// createErrorResponse creates the byte representation of a JSON-RPC error response.
func (s *Server) createErrorResponse(msgBytes []byte, err error) ([]byte, error) {
	requestID := extractRequestID(msgBytes)
	code, message, data := s.mapErrorToJSONRPCComponents(err)
	s.logErrorDetails(code, message, requestID, data, err)

	errorPayload := struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data,omitempty"`
	}{
		Code:    code,
		Message: message,
		Data:    data,
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
		s.logger.Error("CRITICAL: Failed to marshal final error response.", "marshalError", fmt.Sprintf("%+v", marshalErr))
		return nil, errors.Wrap(marshalErr, "failed to marshal error response object")
	}

	return responseBytes, nil
}

// extractRequestID attempts to get the ID from raw message bytes.
func extractRequestID(msgBytes []byte) json.RawMessage {
	var request struct {
		ID json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal(msgBytes, &request) // Ignore error, default to null.
	if request.ID != nil {
		return request.ID
	}
	return json.RawMessage("null")
}

// mapErrorToJSONRPCComponents maps Go errors to JSON-RPC code, message, and optional data.
func (s *Server) mapErrorToJSONRPCComponents(err error) (code int, message string, data interface{}) {
	data = nil // Initialize data.

	var mcpErr *mcperrors.BaseError
	var transportErr *transport.Error
	var validationErr *schema.ValidationError

	// Use errors.Cause to get the root error before checking its string representation.
	rootErr := errors.Cause(err)
	errStr := rootErr.Error() // Get the string of the root cause.

	// Check for specific error strings first for method not found/sequence errors.
	if strings.Contains(errStr, "Method not found:") {
		code = transport.JSONRPCMethodNotFound // -32601.
		message = "Method not found."
		methodName := strings.TrimPrefix(errStr, "Method not found: ")
		if methodName != errStr {
			data = map[string]interface{}{
				"method": methodName,
				"detail": "The requested method is not supported by this MCP server.",
			}
		}
	} else if strings.Contains(errStr, "protocol sequence error:") {
		code = transport.JSONRPCMethodNotFound // -32601 to match expected test value.
		message = "Connection initialization required."
		dataMap := map[string]interface{}{"detail": errStr} // Initialize map
		if s.connectionState != nil {                       // Add state if available
			dataMap["state"] = s.connectionState.CurrentState()
		}
		if strings.Contains(errStr, "must first call 'initialize'") {
			dataMap["help"] = "The MCP protocol requires initialize to be called first."
			dataMap["reference"] = "https://modelcontextprotocol.io/docs/concepts/messages/#server-initialization"
		} else if strings.Contains(errStr, "can only be called once") {
			dataMap["help"] = "The initialize method can only be called once per connection."
			dataMap["reference"] = "https://modelcontextprotocol.io/docs/concepts/messages/#server-initialization"
		}
		data = dataMap // Assign the map back to data
	} else if strings.Contains(errStr, "connection not initialized") {
		code = transport.JSONRPCMethodNotFound // -32601.
		message = "Connection initialization required."
		dataMap := map[string]interface{}{"detail": errStr} // Initialize map
		if s.connectionState != nil {                       // Add state if available
			dataMap["state"] = s.connectionState.CurrentState()
		}
		dataMap["help"] = "The MCP protocol requires initialize to be called first."
		data = dataMap // Assign the map back to data
	} else if errors.As(err, &validationErr) {
		code, message, data = mapValidationError(validationErr)
	} else if errors.As(err, &mcpErr) {
		code, message = mapMCPError(mcpErr) // mapMCPError assigns code
		if mcpErr.Context != nil {
			data = mcpErr.Context
		}
	} else if errors.As(err, &transportErr) {
		code, message, data = transport.MapErrorToJSONRPC(transportErr)
	} else {
		code, message = mapGenericGoError(err)
	}

	return code, message, data
}

// mapValidationError maps schema.ValidationError to JSON-RPC components.
func mapValidationError(validationErr *schema.ValidationError) (code int, message string, data interface{}) {
	if validationErr.Code == schema.ErrInvalidJSONFormat {
		code = transport.JSONRPCParseError
		message = "Parse error."
	} else if validationErr.InstancePath != "" && (strings.Contains(validationErr.InstancePath, "/params") || strings.Contains(validationErr.InstancePath, "params")) {
		code = transport.JSONRPCInvalidParams
		message = "Invalid params."
	} else {
		code = transport.JSONRPCInvalidRequest
		message = "Invalid request."
	}
	// Include specific validation details in data
	dataMap := map[string]interface{}{ // Initialize map
		"validationPath":  validationErr.InstancePath,
		"validationError": validationErr.Message,
		"schemaPath":      validationErr.SchemaPath, // Add schema path if available
	}
	// Merge context from validation error if present
	if validationErr.Context != nil {
		for k, v := range validationErr.Context {
			if _, exists := dataMap[k]; !exists {
				dataMap["context_"+k] = v
			}
			if k == "suggestion" {
				dataMap["suggestion"] = v
			}
		}
	}
	data = dataMap // Assign map back to data
	return code, message, data
}

// mapMCPError maps mcperrors.BaseError to JSON-RPC code and message.
func mapMCPError(mcpErr *mcperrors.BaseError) (code int, message string) {
	message = mcpErr.Message // Use the message from the MCP error
	// code = transport.JSONRPCInternalError // REMOVED Initial assignment

	// Map specific internal codes to standard or implementation-defined JSON-RPC codes.
	switch mcpErr.Code {
	case mcperrors.ErrProtocolInvalid:
		code = transport.JSONRPCInvalidRequest // -32600
	case mcperrors.ErrResourceNotFound:
		code = -32001 // Example custom code
	case mcperrors.ErrAuthFailure:
		code = -32002 // Example custom code
	case mcperrors.ErrRTMAPIFailure:
		code = -32010 // Example custom code
	// Add mappings for other custom MCP error codes
	default:
		// Check if the code is already in the implementation-defined range
		if mcpErr.Code >= -32099 && mcpErr.Code <= -32000 {
			code = mcpErr.Code
		} else {
			code = -32000 // Fallback generic implementation-defined server error
		}
	}
	return code, message
}

// mapGenericGoError maps generic Go errors.
func mapGenericGoError(_ error) (code int, message string) {
	code = transport.JSONRPCInternalError // -32603
	message = "An unexpected internal server error occurred."
	return code, message
}

// logErrorDetails logs detailed error information server-side.
func (s *Server) logErrorDetails(code int, message string, requestID json.RawMessage, data interface{}, err error) {
	logArgs := []interface{}{
		"jsonrpcErrorCode", code,
		"jsonrpcErrorMessage", message,
		"originalError", fmt.Sprintf("%+v", err), // Use %+v for stack trace from cockroachdb/errors
		"requestID", string(requestID),
	}
	if data != nil {
		logArgs = append(logArgs, "responseData", data)
	}

	// Add context from specific error types
	var mcpErr *mcperrors.BaseError
	var transportErr *transport.Error
	var validationErr *schema.ValidationError

	if errors.As(err, &mcpErr) {
		logArgs = append(logArgs, "internalErrorCode", mcpErr.Code)
		if len(mcpErr.Context) > 0 {
			logArgs = append(logArgs, "internalErrorContext", mcpErr.Context)
		}
	} else if errors.As(err, &transportErr) {
		logArgs = append(logArgs, "transportErrorCode", transportErr.Code)
		if len(transportErr.Context) > 0 {
			logArgs = append(logArgs, "transportErrorContext", transportErr.Context)
		}
	} else if errors.As(err, &validationErr) {
		logArgs = append(logArgs, "validationErrorCode", validationErr.Code)
		logArgs = append(logArgs, "validationInstancePath", validationErr.InstancePath)
		logArgs = append(logArgs, "validationSchemaPath", validationErr.SchemaPath)
		if len(validationErr.Context) > 0 {
			logArgs = append(logArgs, "validationErrorContext", validationErr.Context)
		}
	}

	s.logger.Error("Generating JSON-RPC error response.", logArgs...)
}
