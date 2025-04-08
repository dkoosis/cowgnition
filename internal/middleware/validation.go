// file: internal/middleware/validation.go
package middleware

import (
	"context"
	"encoding/json" // Added for error wrapping.
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
)

// ValidationOptions contains configuration options for the validation middleware.
// These options balance correctness, performance, and operational flexibility.
type ValidationOptions struct {
	// Enabled determines if validation is active. If false, validation is skipped entirely.
	Enabled bool

	// SkipTypes is a map of message types to skip validation for.
	// Key is the message type (e.g., "ping"), value is a boolean (always true).
	SkipTypes map[string]bool

	// StrictMode determines if validation errors result in rejection.
	// If true (default), validation failures cause messages to be rejected.
	// If false, validation errors are logged but messages still pass through.
	StrictMode bool

	// MeasurePerformance enables logging of validation performance metrics.
	MeasurePerformance bool
}

// DefaultValidationOptions returns the default validation options.
// These defaults prioritize correctness and security over performance.
func DefaultValidationOptions() ValidationOptions {
	return ValidationOptions{
		Enabled:            true,
		SkipTypes:          map[string]bool{"ping": true}, // Skip ping by default.
		StrictMode:         true,
		MeasurePerformance: false,
	}
}

// ValidationMiddleware validates incoming messages against JSON schemas.
// It serves as a guardian of protocol correctness.
type ValidationMiddleware struct {
	// validator is the schema validator used to validate messages.
	validator *schema.SchemaValidator

	// options contains the configuration options for this middleware.
	options ValidationOptions

	// next is the next handler in the middleware chain.
	next transport.MessageHandler

	// logger for validation-related events.
	logger logging.Logger
}

// NewValidationMiddleware creates a new validation middleware with the given options.
func NewValidationMiddleware(validator *schema.SchemaValidator, options ValidationOptions, logger logging.Logger) *ValidationMiddleware {
	// Use NoopLogger if no logger is provided.
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	return &ValidationMiddleware{
		validator: validator,
		options:   options,
		logger:    logger.WithField("middleware", "validation"),
	}
}

// SetNext sets the next handler in the middleware chain.
func (m *ValidationMiddleware) SetNext(next transport.MessageHandler) {
	m.next = next
}

// HandleMessage implements the MessageHandler interface.
// It validates the message if validation is enabled, then passes it to the next handler.
func (m *ValidationMiddleware) HandleMessage(ctx context.Context, message []byte) ([]byte, error) {
	// Fast path: If validation is disabled, skip directly to the next handler.
	if !m.options.Enabled {
		m.logger.Debug("Validation disabled, skipping.")
		return m.next(ctx, message)
	}

	// Start measuring performance if enabled.
	var startTime time.Time
	if m.options.MeasurePerformance {
		startTime = time.Now()
	}

	// Basic JSON syntax validation first.
	if !json.Valid(message) {
		m.logger.Warn("Invalid JSON syntax received.", "messagePreview", string(message[:min(len(message), 100)]))
		// Generate and return the Parse Error response.
		// Handle the two return values from createParseErrorResponse correctly.
		responseBytes, creationErr := createParseErrorResponse(nil, errors.New("invalid JSON syntax"))
		if creationErr != nil {
			// If creating the error response fails, return that internal error.
			return nil, creationErr
		}
		// Return the generated error response bytes and nil error (signals response is ready).
		return responseBytes, nil
	}

	// Identify the message type and extract the request ID.
	msgType, reqID, identifyErr := m.identifyMessage(message)
	if identifyErr != nil {
		m.logger.Warn("Failed to identify message type.", "error", identifyErr, "messagePreview", string(message[:min(len(message), 100)]))
		// Generate and return Invalid Request response.
		// Handle the two return values correctly.
		responseBytes, creationErr := createInvalidRequestErrorResponse(reqID, identifyErr)
		if creationErr != nil {
			return nil, creationErr
		}
		return responseBytes, nil
	}

	// Skip validation for exempted message types.
	if m.options.SkipTypes[msgType] {
		m.logger.Debug("Skipping validation for message type.", "type", msgType)
		return m.next(ctx, message)
	}

	// Perform the validation against the schema.
	// TODO: Select schema based on msgType more intelligently if needed.
	// Assuming "base" schema covers all JSON-RPC structures for now.
	validationErr := m.validator.Validate(ctx, "base", message)

	// Log performance metrics if enabled.
	if m.options.MeasurePerformance {
		elapsed := time.Since(startTime)
		m.logger.Debug("Message validation performance.",
			"messageType", msgType,
			"duration", elapsed,
			"requestID", reqID,
			"isValid", validationErr == nil)
	}

	if validationErr != nil {
		// Handle validation errors according to strict mode setting.
		if m.options.StrictMode {
			m.logger.Warn("Message validation failed (strict mode, rejecting).",
				"messageType", msgType,
				"requestID", reqID,
				"error", validationErr)
			// Generate and return the Validation Error response.
			// Handle the two return values correctly.
			responseBytes, creationErr := createValidationErrorResponse(reqID, validationErr)
			if creationErr != nil {
				return nil, creationErr
			}
			return responseBytes, nil
		}

		// In non-strict mode, log the error but allow processing to continue.
		m.logger.Warn("Validation error (passing through in non-strict mode).",
			"messageType", msgType,
			"requestID", reqID,
			"error", validationErr)
	}

	// If validation passes or we're in non-strict mode with errors, continue to next handler.
	return m.next(ctx, message)
}

// identifyMessage extracts the message type and request ID from a JSON-RPC message.
// Returns message type (method name or response type), request ID (if present), and error.
func (m *ValidationMiddleware) identifyMessage(message []byte) (string, interface{}, error) {
	// Parse just enough of the message to identify type.
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(message, &parsed); err != nil {
		return "", nil, errors.Wrap(err, "failed to parse message for identification")
	}

	// Extract ID if present.
	var id interface{}
	if idRaw, ok := parsed["id"]; ok {
		// Unmarshal preserves null, string, or number types for ID.
		if err := json.Unmarshal(idRaw, &id); err != nil {
			// If ID exists but is invalid type (e.g., object), treat as error.
			return "", nil, errors.Wrap(err, "failed to parse id")
		}
	} // If ID is absent, id remains nil (correct for notifications).

	// Check if it's a request/notification (has 'method') or response (has 'result' or 'error').
	if methodRaw, ok := parsed["method"]; ok {
		// It's a request or notification, extract the method name.
		var method string
		if err := json.Unmarshal(methodRaw, &method); err != nil {
			return "", id, errors.Wrap(err, "failed to parse method")
		}
		// Distinguish request (has ID) from notification (no ID or null ID).
		if id == nil {
			return method + "_notification", nil, nil // No ID for notification responses.
		}
		return method, id, nil // Method name and ID for request.
	}

	// If it has 'result', it's a success response.
	if _, hasResult := parsed["result"]; hasResult {
		return "success_response", id, nil
	}

	// If it has 'error', it's an error response.
	if _, hasError := parsed["error"]; hasError {
		return "error_response", id, nil
	}

	// If we can't identify the message type, return an error.
	return "", id, errors.New("unable to identify message type (not request, notification, or response)")
}

// --- Helper Functions to Create JSON-RPC Error Responses ---
// These functions now correctly return ([]byte, error).

// createParseErrorResponse creates JSON bytes for a JSON-RPC parse error (-32700).
func createParseErrorResponse(id interface{}, parseErr error) ([]byte, error) {
	if id == nil { // ID is often unknown if parsing failed early.
		id = json.RawMessage("null")
	}
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    transport.JSONRPCParseError, // -32700.
			"message": "Parse error.",
			"data": map[string]interface{}{
				"details": parseErr.Error(), // Provide underlying parse error detail.
			},
		},
	}
	return json.Marshal(response) // Returns ([]byte, error).
}

// createInvalidRequestErrorResponse creates JSON bytes for a JSON-RPC invalid request error (-32600).
func createInvalidRequestErrorResponse(id interface{}, requestErr error) ([]byte, error) {
	if id == nil {
		id = json.RawMessage("null")
	}
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    transport.JSONRPCInvalidRequest, // -32600.
			"message": "Invalid Request.",
			"data": map[string]interface{}{
				"details": requestErr.Error(),
			},
		},
	}
	return json.Marshal(response) // Returns ([]byte, error).
}

// createValidationErrorResponse creates JSON bytes for a JSON-RPC validation error.
// Maps schema validation errors to Invalid Request (-32600) or Invalid Params (-32602).
func createValidationErrorResponse(id interface{}, validationErr error) ([]byte, error) {
	if id == nil {
		id = json.RawMessage("null")
	}
	code := transport.JSONRPCInvalidRequest // -32600 Default to Invalid Request.
	message := "Invalid Request."
	var errorData interface{} // Keep nil unless details are useful/safe.

	// Check if it's a structured schema validation error.
	var schemaValErr *schema.ValidationError
	if errors.As(validationErr, &schemaValErr) {
		// Check if the error path indicates it's a parameter issue.
		if schemaValErr.InstancePath != "" && (strings.Contains(schemaValErr.InstancePath, "/params") || strings.Contains(schemaValErr.InstancePath, "params")) {
			code = transport.JSONRPCInvalidParams // -32602.
			message = "Invalid params."
		}
		// Optionally include sanitized validation context.
		// errorData = schemaValErr.Context // Example, ensure sanitization.
		errorData = map[string]interface{}{"details": schemaValErr.Error()} // Include formatted error.
	} else {
		// For generic validation errors.
		errorData = map[string]interface{}{"details": validationErr.Error()}
	}

	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
			"data":    errorData, // Include structured details if available.
		},
	}
	return json.Marshal(response) // Returns ([]byte, error).
}

// min returns the smaller of x or y.
func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}
