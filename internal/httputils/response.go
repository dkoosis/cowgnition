// internal/httputils/response.go
// Package httputils provides helper functions for handling HTTP requests and responses,
// specifically focusing on JSON and JSON-RPC 2.0 error formatting.
package httputils

import (
	"encoding/json"
	"fmt"      // Used for error formatting.
	"net/http" // Provides HTTP client and server implementations.

	"github.com/cockroachdb/errors"                           // Error handling library.
	"github.com/dkoosis/cowgnition/internal/logging"          // Project's logging helper.
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors" // Project's MCP error types.
)

// logger initializes the structured logger for the httputils package.
var logger = logging.GetLogger("httputils")

// ErrorCode defines standardized error codes, primarily following the JSON-RPC 2.0 specification,
// with custom codes for specific application (MCP) errors.
type ErrorCode int

// Constants defining standard JSON-RPC 2.0 and custom MCP error codes.
const (
	// Standard JSON-RPC 2.0 error codes.

	// ParseError indicates invalid JSON was received by the server.
	// An error occurred on the server while parsing the JSON text.
	ParseError ErrorCode = -32700
	// InvalidRequest indicates the JSON sent is not a valid Request object.
	InvalidRequest ErrorCode = -32600
	// MethodNotFound indicates the method does not exist or is not available.
	MethodNotFound ErrorCode = -32601
	// InvalidParams indicates invalid method parameters were provided.
	InvalidParams ErrorCode = -32602
	// InternalError indicates an internal JSON-RPC error occurred.
	InternalError ErrorCode = -32603

	// Custom MCP-specific error codes (outside the reserved JSON-RPC range -32000 to -32099).

	// AuthError signifies issues related to authentication or authorization.
	AuthError ErrorCode = -31000
	// ResourceError indicates problems accessing a required resource (e.g., not found, unavailable).
	ResourceError ErrorCode = -31001
	// ServiceError represents errors originating from external services relied upon by the application.
	ServiceError ErrorCode = -31002
	// ToolError denotes errors occurring during the execution of specific tools or operations.
	ToolError ErrorCode = -31003
	// ValidationError flags issues with input data validation (e.g., format, range).
	ValidationError ErrorCode = -31004
)

// ErrorResponse represents a JSON-RPC 2.0 error response structure.
// It conforms to the standard structure for reporting errors over JSON-RPC.
type ErrorResponse struct {
	JSONRPC string      `json:"jsonrpc"`      // Must be "2.0".
	Error   ErrorObject `json:"error"`        // The error object containing details.
	ID      interface{} `json:"id,omitempty"` // Request ID (string, number, or null) if available, omitted otherwise.
}

// ErrorObject represents the structured error information within a JSON-RPC 2.0 error response.
type ErrorObject struct {
	Code    ErrorCode   `json:"code"`           // A number indicating the error type that occurred.
	Message string      `json:"message"`        // A string providing a short description of the error.
	Data    interface{} `json:"data,omitempty"` // Additional data about the error, if available.
}

// WriteJSONResponse marshals the provided data into JSON and writes it to the
// http.ResponseWriter with a StatusOK header and application/json content type.
// If encoding fails after the headers have been written, it logs the error
// but cannot change the response status code.
func WriteJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // Note: This is called before potential error handling below.

	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Although status OK was already sent, we log the encoding error.
		wrappedErr := errors.Wrap(err, "failed to encode JSON response")

		// Log the encoding error with detailed context.
		logger.Error("Failed to encode JSON response after headers sent.",
			"error", fmt.Sprintf("%+v", wrappedErr),
			"data_type", fmt.Sprintf("%T", data),
		)
		// Cannot send a different HTTP status or error body because headers/status are already sent.
	}
}

// WriteErrorResponse constructs a standard JSON-RPC 2.0 error response and writes it
// to the http.ResponseWriter. It sets the Content-Type to application/json and determines
// an appropriate HTTP status code based on the ErrorCode.
// If headers have already been written (e.g., by a previous call or middleware),
// it logs a warning and attempts to write the JSON error body anyway, but cannot
// set the headers or status code. If encoding the error response itself fails,
// it logs the encoding error and attempts a plain text HTTP error fallback
// only if headers haven't already been written.
func WriteErrorResponse(w http.ResponseWriter, code ErrorCode, message string, data interface{}) {
	errResp := ErrorResponse{
		JSONRPC: "2.0",
		Error: ErrorObject{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	// Check if headers have already been written to prevent conflicts.
	headersWritten := hasWrittenHeaders(w)
	if !headersWritten {
		w.Header().Set("Content-Type", "application/json")
		httpStatus := httpStatusFromErrorCode(code)
		w.WriteHeader(httpStatus)
	} else {
		logger.Warn("WriteErrorResponse called after headers already written.", "original_code", code, "original_message", message)
		// Cannot set headers or status code now, will attempt to write the body anyway.
	}

	if err := json.NewEncoder(w).Encode(errResp); err != nil {
		// Wrap the encoding error with context.
		wrappedErr := cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to encode error response"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"original_error_code":    int(code),
				"original_error_message": message,
			},
		)

		// Log the detailed error.
		logger.Error("Failed to encode error response.", "error", fmt.Sprintf("%+v", wrappedErr))

		// Provide a fallback plain text error response ONLY if headers weren't already sent.
		if !headersWritten {
			// Avoid writing headers again; this uses the standard http.Error mechanism.
			// Note: This fallback might overwrite the Content-Type set earlier if headers weren't flushed yet.
			http.Error(w, fmt.Sprintf("Internal server error: %v", err), http.StatusInternalServerError)
		}
		// If headers were already written, we can't send a different http.Error. Logging is the main recourse.
	}
}

// httpStatusFromErrorCode maps JSON-RPC/MCP error codes to appropriate HTTP status codes.
// This provides a reasonable mapping for standard HTTP clients.
func httpStatusFromErrorCode(code ErrorCode) int {
	switch code {
	case ParseError, InvalidRequest, InvalidParams:
		return http.StatusBadRequest // 400.
	case MethodNotFound:
		return http.StatusNotFound // 404.
	case AuthError:
		return http.StatusUnauthorized // 401.
	case ResourceError:
		// Often maps to 404, but could be others depending on context. 404 is a common default.
		return http.StatusNotFound // 404.
	default:
		// Includes InternalError, ServiceError, ToolError, ValidationError, etc.
		return http.StatusInternalServerError // 500.
	}
}

// hasWrittenHeaders attempts to check if the response headers have already been written.
// WARNING: This function relies on inspecting the internal state of the http.ResponseWriter
// using fmt.Sprintf. This is a fragile approach and may break with future Go releases
// or different ResponseWriter implementations (e.g., middleware wrappers).
// A more robust solution typically involves using a custom ResponseWriter wrapper
// that explicitly tracks the state of header writing. Use with caution.
func hasWrittenHeaders(w http.ResponseWriter) bool {
	// This is a heuristic and depends on implementation details.
	// It compares the formatted string representation of the ResponseWriter
	// against a newly initialized ResponseController, assuming differences imply state changes (like header writes).
	// A dedicated wrapper type is the recommended way to track this state reliably.
	// Placeholder check; needs verification or replacement with a robust method.
	// Consider implementing a ResponseWriter wrapper to track `wroteHeader` state explicitly.
	//const responseHeaderWrittenMarker = "wroteHeader=true" // Example marker; actual check might differ.
	// A simple length check or comparing against zero value might be insufficient.
	// A more sophisticated check might look for specific fields indicating header write.
	// For demonstration, let's keep the original placeholder logic, emphasizing its fragility.
	return fmt.Sprintf("%#v", w) != fmt.Sprintf("%#v", &http.ResponseController{}) // Fragile placeholder heuristic.
}
