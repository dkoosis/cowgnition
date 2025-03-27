// internal/httputils/response.go
package httputils

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// ErrorCode defines standardized error codes according to JSON-RPC 2.0.
type ErrorCode int

const (
	// Standard JSON-RPC 2.0 error codes.
	ParseError     ErrorCode = -32700 // Invalid JSON
	InvalidRequest ErrorCode = -32600 // Request object invalid
	MethodNotFound ErrorCode = -32601 // Method doesn't exist
	InvalidParams  ErrorCode = -32602 // Invalid method parameters
	InternalError  ErrorCode = -32603 // Internal JSON-RPC error

	// Custom MCP-specific error codes.
	AuthError       ErrorCode = -31000 // Authentication errors
	ResourceError   ErrorCode = -31001 // Resource not found or unavailable
	ServiceError    ErrorCode = -31002 // External service errors
	ToolError       ErrorCode = -31003 // Tool execution errors
	ValidationError ErrorCode = -31004 // Input validation errors
)

// ErrorResponse represents a JSON-RPC 2.0 error response.
type ErrorResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Error   ErrorObject `json:"error"`
	ID      interface{} `json:"id,omitempty"` // Can be string, number, or null
}

// ErrorObject represents the error object within a JSON-RPC 2.0 error response.
type ErrorObject struct {
	Code    ErrorCode   `json:"code"`           // Numerical error code
	Message string      `json:"message"`        // Human-readable description
	Data    interface{} `json:"data,omitempty"` // Additional error information
}

// WriteJSONResponse writes a JSON response with appropriate headers.
func WriteJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("httputils.WriteJSONResponse: failed to encode JSON response: %v", err)
		WriteErrorResponse(w, InternalError, "failed to encode response", nil)
	}
}

// WriteErrorResponse writes a JSON-RPC 2.0 error response.
func WriteErrorResponse(w http.ResponseWriter, code ErrorCode, message string, data interface{}) {
	errResp := ErrorResponse{
		JSONRPC: "2.0",
		Error: ErrorObject{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	httpStatus := httpStatusFromErrorCode(code)
	w.WriteHeader(httpStatus)

	if err := json.NewEncoder(w).Encode(errResp); err != nil {
		log.Printf("httputils.WriteErrorResponse: failed to encode error response: %v", err)
		http.Error(w, fmt.Sprintf("Internal error: %v", err), http.StatusInternalServerError)
	}
}

// httpStatusFromErrorCode maps MCP error codes to HTTP status codes.
func httpStatusFromErrorCode(code ErrorCode) int {
	switch code {
	case ParseError, InvalidRequest, InvalidParams:
		return http.StatusBadRequest
	case MethodNotFound:
		return http.StatusNotFound
	case AuthError:
		return http.StatusUnauthorized
	case ResourceError:
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// ErrorMsgEnhanced:2025-03-26
