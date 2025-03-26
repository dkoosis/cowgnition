// Package httputils provides utility functions for handling HTTP requests and responses,
// specifically tailored for JSON-RPC 2.0 communication.
// file: internal/server/httputils/response.go
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
// These codes are used to provide structured error information to clients.
type ErrorCode int

const (
	// Standard JSON-RPC 2.0 error codes.
	ParseError     ErrorCode = -32700 // Invalid JSON
	InvalidRequest ErrorCode = -32600 // Request object invalid
	MethodNotFound ErrorCode = -32601 // Method doesn't exist
	InvalidParams  ErrorCode = -32602 // Invalid method parameters
	InternalError  ErrorCode = -32603 // Internal JSON-RPC error

	// Custom MCP-specific error codes (must be above -32000).
	// These custom codes allow the MCP server to provide more specific
	// error information relevant to its domain.
	AuthError       ErrorCode = -31000 // Authentication errors
	ResourceError   ErrorCode = -31001 // Resource not found or unavailable
	RTMServiceError ErrorCode = -31002 // RTM API errors
	ToolError       ErrorCode = -31003 // Tool execution errors
	ValidationError ErrorCode = -31004 // Input validation errors
)

// ErrorResponse represents a JSON-RPC 2.0 error response.
// It encapsulates the error information to be sent back to the client.
type ErrorResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Error   ErrorObject `json:"error"`
	ID      interface{} `json:"id,omitempty"` // Can be string, number, or null
}

// ErrorObject represents the error object within a JSON-RPC 2.0 error response.
// It provides details about the error that occurred.
type ErrorObject struct {
	Code    ErrorCode   `json:"code"`           // Numerical error code
	Message string      `json:"message"`        // Human-readable description
	Data    interface{} `json:"data,omitempty"` // Additional error information
}

// DetailedError holds additional error details for logging purposes.
// It includes the original error, stack trace, and any relevant context.
// This is useful for debugging and understanding the error's origin.
type DetailedError struct {
	OriginalError error                  // The underlying error
	StackTrace    string                 // Stack trace at the point the error occurred
	Context       map[string]interface{} // Additional contextual information
}

// Error implements the error interface for DetailedError.
// This allows DetailedError to be used anywhere a standard error is expected.
func (de *DetailedError) Error() string {
	if de.OriginalError != nil {
		return de.OriginalError.Error()
	}
	return "unknown error"
}

// NewErrorResponse creates a new ErrorResponse with the specified code, message, and data.
// This function provides a convenient way to construct standardized error responses.
//
// Parameters:
//   - code ErrorCode: The error code.
//   - message string: The error message.
//   - data interface{}: Additional error data (can be nil).
//
// Returns:
//   - ErrorResponse: A new ErrorResponse instance.
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
// This is similar to fmt.Errorf but returns an ErrorResponse.
//
// Parameters:
//   - code ErrorCode: The error code.
//   - format string: A format string for the error message.
//   - args ...interface{}: Arguments to format into the error message.
//
// Returns:
//   - ErrorResponse: A new ErrorResponse instance.
func Errorf(code ErrorCode, format string, args ...interface{}) ErrorResponse {
	return NewErrorResponse(code, fmt.Sprintf(format, args...), nil)
}

// WithContext adds context to a DetailedError.
// It either updates an existing DetailedError or creates a new one.
// This function helps in enriching error information with relevant context.
//
// Parameters:
//   - err error: The original error.
//   - key string: The context key.
//   - value interface{}: The context value.
//
// Returns:
//   - *DetailedError: The DetailedError with added context.
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
// It's used internally to provide more detailed error information.
//
// Parameters:
//   - skip int: The number of stack frames to skip (used for internal calls).
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
// It sets the Content-Type to application/json and encodes the data.
// If encoding fails, it logs the error and sends an InternalError response.
//
// Parameters:
//   - w http.ResponseWriter: The HTTP response writer.
//   - _ int: (Unused) HTTP status code. The function always uses 200 OK.
//   - data interface{}: The data to be encoded as JSON.
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
// It sets the Content-Type header and determines the HTTP status code based on the error code.
// If encoding the error response fails, it logs the error and sends a 500 Internal Server Error.
//
// Parameters:
//   - w http.ResponseWriter: The HTTP response writer.
//   - errResp ErrorResponse: The JSON-RPC 2.0 error response to write.
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
// It creates a DetailedError for logging, logs it, and sends the error response to the client.
//
// Parameters:
//   - w http.ResponseWriter: The HTTP response writer.
//   - code ErrorCode: The error code.
//   - message string: The error message.
//   - data interface{}: Additional error data (can be nil).
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
// It logs the DetailedError's information if it's a DetailedError, otherwise, it logs the error directly.
//
// Parameters:
//   - err error: The error to log.
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
// This mapping is crucial for proper HTTP communication with clients.
//
// Parameters:
//   - code ErrorCode: The MCP error code.
//
// Returns:
//   - int: The corresponding HTTP status code.
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
// This is useful for correlating error responses with specific requests.
//
// Parameters:
//   - code ErrorCode: The error code.
//   - message string: The error message.
//   - data interface{}: Additional error data (can be nil).
//   - id interface{}: The request ID.
//
// Returns:
//   - ErrorResponse: A new ErrorResponse instance with the ID set.
func NewErrorWithID(code ErrorCode, message string, data interface{}, id interface{}) ErrorResponse {
	errResp := NewErrorResponse(code, message, data)
	errResp.ID = id
	return errResp
}

// WithStackTrace creates a DetailedError with a stack trace.
// This function is used to capture the stack trace when an error occurs.
//
// Parameters:
//   - err error: The original error.
//   - context map[string]interface{}: Additional contextual information.
//
// Returns:
//   - *DetailedError: A new DetailedError instance with the stack trace.
func WithStackTrace(err error, context map[string]interface{}) *DetailedError {
	de := &DetailedError{
		OriginalError: err,
		Context:       context,
	}
	de.captureStackTrace(2) // Skip this function and caller.
	return de
}

// DocEnhanced: 2025-03-25
