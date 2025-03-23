// Package server defines the core server-side logic for the Cowgnition MCP server.
// file: internal/server/errors.go
// It includes error handling, JSON-RPC response formatting, and logging.
package server

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
// These codes are used to provide clients with specific information about the type of error that occurred.
type ErrorCode int

const (
	// Standard JSON-RPC 2.0 error codes.
	// These are predefined codes from the JSON-RPC 2.0 specification.
	ParseError     ErrorCode = -32700 // Invalid JSON
	InvalidRequest ErrorCode = -32600 // Request object invalid
	MethodNotFound ErrorCode = -32601 // Method doesn't exist
	InvalidParams  ErrorCode = -32602 // Invalid method parameters
	InternalError  ErrorCode = -32603 // Internal JSON-RPC error

	// Custom MCP-specific error codes (must be above -32000).
	// These codes are specific to the Model Context Protocol (MCP) and provide more granular error reporting.
	// MCP error codes are defined above -32000 to avoid conflicts with standard JSON-RPC errors.
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
	ID      interface{} `json:"id,omitempty"` // ID is included to correlate the error with the original request, if applicable.
}

// ErrorObject represents the error object within a JSON-RPC 2.0 error response.
// It provides details about the error that occurred.
type ErrorObject struct {
	Code    ErrorCode   `json:"code"`           // Code is the numerical error code.
	Message string      `json:"message"`        // Message is a human-readable description of the error.
	Data    interface{} `json:"data,omitempty"` // Data may contain additional information about the error.
}

// DetailedError holds additional error details for logging purposes.
// These details (like stack traces and context) are not sent to clients to avoid potential security issues and information leakage.
type DetailedError struct {
	OriginalError error                  // OriginalError stores the underlying error.
	StackTrace    string                 // StackTrace contains the stack trace at the point the error occurred.
	Context       map[string]interface{} // Context provides additional contextual information about the error.
}

// Error implements the error interface for DetailedError.
// This allows DetailedError to be used as a standard Go error.
func (de *DetailedError) Error() string {
	if de.OriginalError != nil {
		return de.OriginalError.Error()
	}
	return "unknown error" // Returns a generic message if there's no original error.
}

// NewErrorResponse creates a new ErrorResponse with the specified code, message, and data.
// This function centralizes the creation of error responses, ensuring consistency.
func NewErrorResponse(code ErrorCode, message string, data interface{}) ErrorResponse {
	return ErrorResponse{
		JSONRPC: "2.0",
		Error: ErrorObject{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: nil, // null by default, can be set explicitly for request correlation.
	}
}

// Errorf creates a new ErrorResponse with a formatted message.
// This is useful for including dynamic information in the error message.
func Errorf(code ErrorCode, format string, args ...interface{}) ErrorResponse {
	return NewErrorResponse(code, fmt.Sprintf(format, args...), nil)
}

// WithContext adds context to a DetailedError.
// This allows for enriching error logs with relevant information for debugging.
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
	de.captureStackTrace(2) // Skip this function and caller to get the relevant stack frames.
	return de
}

// captureStackTrace captures the current stack trace.
// This helps in pinpointing the origin of an error.
func (de *DetailedError) captureStackTrace(skip int) {
	const depth = 32 // Depth limits the number of stack frames captured to prevent excessive memory usage.
	var pcs [depth]uintptr
	n := runtime.Callers(skip+1, pcs[:]) // Skip the 'captureStackTrace' and its caller.
	frames := runtime.CallersFrames(pcs[:n])

	var builder strings.Builder
	for {
		frame, more := frames.Next()
		if !strings.Contains(frame.Function, "runtime.") { // Exclude runtime functions from the stack trace for clarity.
			fmt.Fprintf(&builder, "%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		}
		if !more {
			break
		}
	}
	de.StackTrace = builder.String()
}

// WriteJSONRPCError writes a JSON-RPC 2.0 error response to the HTTP response writer.
// This function handles setting the correct content type and HTTP status code.
func WriteJSONRPCError(w http.ResponseWriter, errResp ErrorResponse) {
	w.Header().Set("Content-Type", "application/json")

	// Determine HTTP status code based on error code
	httpStatus := determineHTTPStatus(errResp.Error.Code) // Determine the appropriate HTTP status code to send to the client.
	w.WriteHeader(httpStatus)

	// Marshal error response to JSON
	if err := json.NewEncoder(w).Encode(errResp); err != nil {
		log.Printf("Failed to encode error response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError) // If encoding fails, send a generic server error.
	}
}

// LogDetailedError logs detailed error information for debugging purposes.
// This function differentiates between DetailedError and standard errors to log the maximum amount of diagnostic information.
func LogDetailedError(err error) {
	var de *DetailedError
	if errors.As(err, &de) {
		log.Printf("Detailed Error: %s\nContext: %v\nStack Trace:\n%s",
			de.Error(), de.Context, de.StackTrace) // Log detailed error information, including context and stack trace.
		return
	}
	log.Printf("Error: %v", err) // Log standard errors with a simpler format.
}

// determineHTTPStatus maps MCP error codes to appropriate HTTP status codes.
// This mapping ensures that clients receive HTTP status codes that accurately reflect the nature of the error.
func determineHTTPStatus(code ErrorCode) int {
	switch code {
	case ParseError, InvalidRequest:
		return http.StatusBadRequest // 400 Bad Request for client-side JSON or request errors.
	case MethodNotFound:
		return http.StatusNotFound // 404 Not Found if the requested method doesn't exist.
	case InvalidParams:
		return http.StatusBadRequest // 400 Bad Request for incorrect method parameters.
	case AuthError:
		return http.StatusUnauthorized // 401 Unauthorized for authentication failures.
	case ResourceError:
		return http.StatusNotFound // 404 Not Found if a resource is not found.
	case ValidationError:
		return http.StatusBadRequest // 400 Bad Request for validation errors.
	default:
		return http.StatusInternalServerError // 500 Internal Server Error for all other errors.
	}
}

// NewErrorWithID creates an ErrorResponse with the specified request ID.
// Including the ID helps correlate errors with specific requests, which is useful for debugging and tracking.
func NewErrorWithID(code ErrorCode, message string, data interface{}, id interface{}) ErrorResponse {
	errResp := NewErrorResponse(code, message, data)
	errResp.ID = id
	return errResp
}

// WriteJSONRPCErrorWithContext writes a JSON-RPC 2.0 error response and logs detailed context.
// This function combines sending an error response to the client with logging detailed information for debugging.
func WriteJSONRPCErrorWithContext(w http.ResponseWriter, code ErrorCode, message string, context map[string]interface{}) {
	// Create detailed error for logging
	detailedErr := &DetailedError{
		OriginalError: fmt.Errorf("%s", message), // Fixed: Use constant format string.  Wrap the message in an error to provide more context for logging.
		Context:       context,
	}
	detailedErr.captureStackTrace(2) // Skip this function and caller to get the relevant stack frames.

	// Log the detailed error
	LogDetailedError(detailedErr)

	// Send simplified error response to client
	errResp := NewErrorResponse(code, message, nil)
	WriteJSONRPCError(w, errResp)
}

// DocEnhanced (2025-03-22)
