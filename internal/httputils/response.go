// internal/httputils/response.go
package httputils

import (
	"encoding/json"
	"fmt" // Import slog.
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging" // Import project logging helper.
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// Initialize the logger at the package level.
var logger = logging.GetLogger("httputils")

// ErrorCode defines standardized error codes according to JSON-RPC 2.0.
type ErrorCode int

const (
	// Standard JSON-RPC 2.0 error codes.
	ParseError     ErrorCode = -32700 // Invalid JSON.
	InvalidRequest ErrorCode = -32600 // Request object invalid.
	MethodNotFound ErrorCode = -32601 // Method doesn't exist.
	InvalidParams  ErrorCode = -32602 // Invalid method parameters.
	InternalError  ErrorCode = -32603 // Internal JSON-RPC error.

	// Custom MCP-specific error codes.
	AuthError       ErrorCode = -31000 // Authentication errors.
	ResourceError   ErrorCode = -31001 // Resource not found or unavailable.
	ServiceError    ErrorCode = -31002 // External service errors.
	ToolError       ErrorCode = -31003 // Tool execution errors.
	ValidationError ErrorCode = -31004 // Input validation errors.
)

// ErrorResponse represents a JSON-RPC 2.0 error response.
type ErrorResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Error   ErrorObject `json:"error"`
	ID      interface{} `json:"id,omitempty"` // Can be string, number, or null.
}

// ErrorObject represents the error object within a JSON-RPC 2.0 error response.
type ErrorObject struct {
	Code    ErrorCode   `json:"code"`           // Numerical error code.
	Message string      `json:"message"`        // Human-readable description.
	Data    interface{} `json:"data,omitempty"` // Additional error information.
}

// WriteJSONResponse writes a JSON response with appropriate headers.
func WriteJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // Note: This is called before potential error handling below

	if err := json.NewEncoder(w).Encode(data); err != nil {
		// Although status OK was already sent, we log the encoding error.
		wrappedErr := errors.Wrap(err, "failed to encode JSON response")

		// Replace log.Printf with logger.Error, including full error detail and context
		logger.Error("Failed to encode JSON response after headers sent",
			"error", fmt.Sprintf("%+v", wrappedErr),
			"data_type", fmt.Sprintf("%T", data),
		)

		// The original log.Printf("Error details:...") is redundant now as data_type is in the structured log.
		// We cannot effectively write an error response here because headers/status are already sent.
		// The logging above is the main action we can take.
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

	// Prevent header conflicts if WriteJSONResponse already wrote headers before encountering an error
	if !hasWrittenHeaders(w) {
		w.Header().Set("Content-Type", "application/json")
		httpStatus := httpStatusFromErrorCode(code)
		w.WriteHeader(httpStatus)
	} else {
		logger.Warn("WriteErrorResponse called after headers already written", "original_code", code, "original_message", message)
		// Cannot set headers or status code now, attempt to write body anyway if possible.
	}

	if err := json.NewEncoder(w).Encode(errResp); err != nil {
		// Create the detailed error object
		wrappedErr := cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to encode error response"),
			cgerr.CategoryRPC,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"original_error_code":    int(code),
				"original_error_message": message,
			},
		)

		// Replace log.Printf with logger.Error, including full error detail
		logger.Error("Failed to encode error response", "error", fmt.Sprintf("%+v", wrappedErr))

		// Fallback only if headers haven't been written
		if !hasWrittenHeaders(w) {
			// Avoid writing headers again if already written
			http.Error(w, fmt.Sprintf("Internal error: %v", err), http.StatusInternalServerError)
		}
		// If headers were already written, we can't send a different http.Error. Logging is the main recourse.
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

// hasWrittenHeaders checks if the response headers have been written.
// Note: This is a simple check using a non-exported field via fmt.Sprintf,
// which is fragile and might break in future Go versions.
// A more robust solution might involve middleware or custom ResponseWriter wrappers.
func hasWrittenHeaders(w http.ResponseWriter) bool {
	// This is a heuristic and might not be perfectly reliable.
	// It relies on the internal structure of http.response potentially revealed by fmt.
	// A ResponseWriter wrapper is generally the cleaner way to track this.
	return fmt.Sprintf("%#v", w) != fmt.Sprintf("%#v", &http.ResponseController{}) // Placeholder heuristic
}
