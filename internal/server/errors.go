// internal/server/errors.go
package server

import (
	"encoding/json"
	"log"
	"net/http"
)

// ErrorCode defines standardized error codes according to MCP protocol.
type ErrorCode int

const (
	// MCP standard error codes.
	ParseError     ErrorCode = -32700 // Invalid JSON
	InvalidRequest ErrorCode = -32600 // Request object invalid
	MethodNotFound ErrorCode = -32601 // Method doesn't exist
	InvalidParams  ErrorCode = -32602 // Invalid method parameters
	InternalError  ErrorCode = -32603 // Internal JSON-RPC error

	// Custom server error codes (must be above -32000).
	AuthError       ErrorCode = -31000 // Authentication errors
	ResourceError   ErrorCode = -31001 // Resource not found or unavailable
	RTMServiceError ErrorCode = -31002 // RTM API errors
	ToolError       ErrorCode = -31003 // Tool execution errors
	ValidationError ErrorCode = -31004 // Input validation errors
)

// MCPErrorResponse represents a standardized error response according to MCP protocol.
type MCPErrorResponse struct {
	JSONRPC string   `json:"jsonrpc"`
	ID      *string  `json:"id,omitempty"`
	Error   MCPError `json:"error"`
}

// MCPError contains error details according to MCP protocol spec.
type MCPError struct {
	Code    ErrorCode   `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// writeStandardErrorResponse writes an error response following MCP protocol spec.
func writeStandardErrorResponse(w http.ResponseWriter, code ErrorCode, message string, data interface{}) {
	// Construct error response
	errorResp := MCPErrorResponse{
		JSONRPC: "2.0",
		ID:      nil, // Always nil for our use case
		Error: MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	// Add detailed context to logs but keep user-facing message clean
	log.Printf("Error response: code=%d, message=%s, data=%v", code, message, data)

	// Set content type and status code
	w.Header().Set("Content-Type", "application/json")

	// Use appropriate HTTP status code based on error type
	httpStatus := determineHTTPStatus(code)
	w.WriteHeader(httpStatus)

	// Encode and write response
	if err := json.NewEncoder(w).Encode(errorResp); err != nil {
		log.Printf("Failed to encode error response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// determineHTTPStatus maps MCP error codes to appropriate HTTP status codes.
func determineHTTPStatus(code ErrorCode) int {
	switch code {
	case ParseError, InvalidRequest:
		return http.StatusBadRequest
	case MethodNotFound:
		return http.StatusNotFound
	case InvalidParams:
		return http.StatusBadRequest
	case AuthError:
		return http.StatusUnauthorized
	case ResourceError:
		return http.StatusNotFound
	case ValidationError:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
