// internal/server/httputils/response.go
package httputils

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"
)

// ErrorCode defines standardized error codes according to the JSON-RPC 2.0 specification.
type ErrorCode int

const (
	// Standard JSON-RPC 2.0 error codes.
	ParseError     ErrorCode = -32700 // Invalid JSON
	InvalidRequest ErrorCode = -32600 // Request object invalid
	MethodNotFound ErrorCode = -32601 // Method doesn't exist
	InvalidParams  ErrorCode = -32602 // Invalid method parameters
	InternalError  ErrorCode = -32603 // Internal JSON-RPC error

	// Custom MCP-specific error codes (must be above -32000).
	AuthError       ErrorCode = -31000 // Authentication errors
	ResourceError   ErrorCode = -31001 // Resource not found or unavailable
	RTMServiceError ErrorCode = -31002 // RTM API errors
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

// DetailedError holds additional error details for logging purposes.
type DetailedError struct {
	OriginalError error                  // The underlying error
	StackTrace    string                 // Stack trace at the point the error occurred
	Context       map[string]interface{} // Additional contextual information
}

// Error implements the error interface for DetailedError.
func (de *DetailedError) Error() string {
	if de.OriginalError != nil {
		return de.OriginalError.Error()
	}
	return "unknown error"
}

// NewErrorResponse creates a new ErrorResponse with the specified code, message, and data.
func NewErrorResponse(code ErrorCode, message string, data interface{}) ErrorResponse {
	return ErrorResponse{
		JSONRPC: "2.0",
		Error: ErrorObject{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: nil, // null by default
	}
}

// Errorf creates a new ErrorResponse with a formatted message.
func Errorf(code ErrorCode, format string, args ...interface{}) ErrorResponse {
	return NewErrorResponse(code, fmt.Sprintf(format, args...), nil)
}

// WithContext adds context to a DetailedError.
func WithContext(err error, key string, value interface{}) *DetailedError {
	var de *DetailedError
	if errors.As(err, &de) {
		// Update existing DetailedError's context
		if de.Context == nil {
			de.Context = make(map[string]interface{})
		}
		de.Context[key] = value
		return de
	}

	// Create new DetailedError with context
	de = &DetailedError{
		OriginalError: err,
		Context:       map[string]interface{}{key: value},
	}
	de.captureStackTrace(2) // Skip this function and caller
	return de
}

// captureStackTrace captures the current stack trace.
func (de *DetailedError) captureStackTrace(skip int) {
	const depth = 32
	var pcs [depth]uintptr
	n := runtime.Callers(skip+1, pcs[:]) // Skip specified number of frames.
	frames := runtime.CallersFrames(pcs[:n])

	var builder strings.Builder
	for {
		frame, more := frames.Next()
		if !strings.Contains(frame.Function, "runtime.") {
			fmt.Fprintf(&builder, "%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		}
		if !more {
			break
		}
	}
	de.StackTrace = builder.String()
}

// WriteJSONResponse writes a JSON response with appropriate headers.
func WriteJSONResponse(w http.ResponseWriter, _ int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // Always use 200 OK for successful responses

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		errResp := NewErrorResponse(InternalError, "Error encoding response", nil)
		WriteJSONRPCError(w, errResp)
	}
}

// WriteJSONRPCError writes a JSON-RPC 2.0 error response to the HTTP response writer.
func WriteJSONRPCError(w http.ResponseWriter, errResp ErrorResponse) {
	w.Header().Set("Content-Type", "application/json")

	// Determine HTTP status code based on error code
	httpStatus := determineHTTPStatus(errResp.Error.Code)
	w.WriteHeader(httpStatus)

	// Marshal error response to JSON
	if err := json.NewEncoder(w).Encode(errResp); err != nil {
		log.Printf("Failed to encode error response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// WriteStandardErrorResponse writes a standardized error response.
func WriteStandardErrorResponse(w http.ResponseWriter, code ErrorCode, message string, data interface{}) {
	// Create detailed error for logging
	detailedErr := &DetailedError{
		OriginalError: fmt.Errorf("%s", message),
		Context:       nil,
	}
	if contextMap, ok := data.(map[string]interface{}); ok {
		detailedErr.Context = contextMap
	}
	detailedErr.captureStackTrace(2) // Skip this function and caller.

	// Log the detailed error
	LogDetailedError(detailedErr)

	// Send error response to client
	errResp := NewErrorResponse(code, message, data)
	WriteJSONRPCError(w, errResp)
}

// LogDetailedError logs detailed error information for debugging purposes.
func LogDetailedError(err error) {
	var de *DetailedError
	if errors.As(err, &de) {
		log.Printf("Detailed Error: %s\nContext: %v\nStack Trace:\n%s",
			de.Error(), de.Context, de.StackTrace)
		return
	}
	log.Printf("Error: %v", err)
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

// NewErrorWithID creates an ErrorResponse with the specified request ID.
func NewErrorWithID(code ErrorCode, message string, data interface{}, id interface{}) ErrorResponse {
	errResp := NewErrorResponse(code, message, data)
	errResp.ID = id
	return errResp
}

// WithStackTrace creates a DetailedError with a stack trace.
func WithStackTrace(err error, context map[string]interface{}) *DetailedError {
	de := &DetailedError{
		OriginalError: err,
		Context:       context,
	}
	de.captureStackTrace(2) // Skip this function and caller.
	return de
}
