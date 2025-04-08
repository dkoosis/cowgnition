// file: internal/middleware/validation.go
package middleware

import (
	"context"
	"encoding/json"
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
	//
	// Motivation:
	// - Performance: In high-throughput production environments, validation might be too costly
	// - A/B Testing: Measure real-world performance impact by enabling/disabling for different traffic
	// - Emergency Override: Provides an escape hatch if validation issues occur in production
	// - Development vs Production: Different environments may have different validation needs
	Enabled bool

	// SkipTypes is a map of message types to skip validation for.
	// Key is the message type (e.g., "ping"), value is a boolean (always true).
	//
	// Motivation:
	// - Performance: High-frequency messages (like heartbeats) can bypass validation to reduce overhead
	// - Latency-Sensitive: Time-critical operations can skip validation to minimize processing time
	// - Well-Tested Paths: For message types that rarely change and are well-tested, validation adds little value
	// - Operational Safety: Critical management commands might need to work even with schema issues
	SkipTypes map[string]bool

	// StrictMode determines if validation errors result in rejection.
	// If true (default), validation failures cause messages to be rejected.
	// If false, validation errors are logged but messages still pass through.
	//
	// Motivation:
	// - Development Environment: Log issues but don't block testing with minor schema violations
	// - Graceful Degradation: Better to process a slightly malformed message than reject it entirely
	// - Migration Periods: Temporarily accommodate clients using older schemas during upgrades
	// - Telemetry: Identify problematic clients without disrupting their functionality
	// - Robustness: Some systems prioritize availability over strict correctness
	StrictMode bool

	// MeasurePerformance enables logging of validation performance metrics.
	// This helps identify which message types or schemas have high validation costs.
	//
	// Motivation:
	// - Performance Optimization: Identify which message types are expensive to validate
	// - Capacity Planning: Understand validation overhead for infrastructure sizing
	// - Schema Complexity Analysis: Detect when schema changes significantly increase validation time
	// - Runtime Monitoring: Track validation performance in production
	MeasurePerformance bool
}

// DefaultValidationOptions returns the default validation options.
// These defaults prioritize correctness and security over performance.
func DefaultValidationOptions() ValidationOptions {
	return ValidationOptions{
		// Enabled by default as validation provides important guarantees for protocol correctness
		Enabled: true,

		// Skip validation for simple, frequent messages by default
		// Ping is a standard health check in JSON-RPC that rarely contains complex data
		SkipTypes: map[string]bool{"ping": true},

		// Default to strict mode for maximum correctness and security
		// This ensures all messages fully conform to the protocol specification
		StrictMode: true,

		// Performance measurement disabled by default to avoid logging overhead
		// Enable this when specifically analyzing validation performance
		MeasurePerformance: false,
	}
}

// ValidationMiddleware validates incoming and outgoing messages against JSON schemas.
// It serves as a guardian of protocol correctness while providing flexibility for
// different operational requirements.
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
	// Use NoopLogger if no logger is provided
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
// The behavior is controlled by the options provided during initialization.
// HandleMessage implements the MessageHandler interface.
func (m *ValidationMiddleware) HandleMessage(ctx context.Context, message []byte) ([]byte, error) {
	// Fast path: If validation is disabled, skip directly to the next handler.
	if !m.options.Enabled {
		return m.next(ctx, message)
	}

	// Start measuring performance if enabled.
	var startTime time.Time
	if m.options.MeasurePerformance {
		startTime = time.Now()
	}

	// Basic JSON syntax validation
	if !json.Valid(message) {
		m.logger.Error("Invalid JSON syntax", "message", string(message[:min(len(message), 100)]))
		return createParseErrorResponse(nil, errors.New("invalid JSON syntax")), nil
	}

	// Identify the message type and extract the request ID.
	msgType, reqID, err := m.identifyMessage(message)
	if err != nil {
		m.logger.Error("Failed to identify message type", "error", err)
		return createParseErrorResponse(reqID, err), nil
	}

	// Skip validation for exempted message types.
	if m.options.SkipTypes[msgType] {
		m.logger.Debug("Skipping validation for message type", "type", msgType)
		return m.next(ctx, message)
	}

	// Perform the validation against the schema.
	err = m.validator.Validate(ctx, "JSONRPCRequest", message) // Use a generic type for now

	// Log performance metrics if enabled.
	if m.options.MeasurePerformance {
		elapsed := time.Since(startTime)
		m.logger.Debug("Message validation performance",
			"messageType", msgType,
			"duration", elapsed)
	}

	if err != nil {
		// Handle validation errors according to strict mode setting.
		if m.options.StrictMode {
			m.logger.Warn("Message validation failed (strict mode, rejecting)",
				"messageType", msgType,
				"error", err)
			return createValidationErrorResponse(reqID, err), nil
		}

		// In non-strict mode, log the error but allow processing to continue.
		m.logger.Warn("Validation error (passing through in non-strict mode)",
			"messageType", msgType,
			"error", err)
	}

	// If validation passes or we're in non-strict mode with errors, continue to next handler.
	return m.next(ctx, message)
}

// identifyMessage extracts the message type and request ID from a JSON-RPC message.
// Returns message type (method name or response type), request ID (if present), and error.
func (m *ValidationMiddleware) identifyMessage(message []byte) (string, interface{}, error) {
	// Parse just enough of the message to identify type
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(message, &parsed); err != nil {
		return "", nil, errors.Wrap(err, "failed to parse message for identification")
	}

	// Extract ID if present
	var id interface{}
	if idRaw, ok := parsed["id"]; ok {
		if err := json.Unmarshal(idRaw, &id); err != nil {
			return "", id, errors.Wrap(err, "failed to parse id")
		}
	}

	// Check if it's a request/notification (has 'method') or response (has 'result' or 'error')
	if methodRaw, ok := parsed["method"]; ok {
		// It's a request or notification, extract the method name
		var method string
		if err := json.Unmarshal(methodRaw, &method); err != nil {
			return "", id, errors.Wrap(err, "failed to parse method")
		}

		// Special case for JSON-RPC notifications (no ID)
		if _, hasID := parsed["id"]; !hasID {
			return method + "_notification", nil, nil
		}

		return method, id, nil
	}

	// If it has 'result', it's a success response
	if _, hasResult := parsed["result"]; hasResult {
		return "success_response", id, nil
	}

	// If it has 'error', it's an error response
	if _, hasError := parsed["error"]; hasError {
		return "error_response", id, nil
	}

	// If we can't identify the message type, return an error
	return "", id, errors.New("unable to identify message type")
}

// createParseErrorResponse creates a JSON-RPC parse error response.
// Parse errors (-32700) occur when the message is not valid JSON or cannot be interpreted.
func createParseErrorResponse(id interface{}, err error) ([]byte, error) {
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id, // This might be nil for notifications, which is fine
		"error": map[string]interface{}{
			"code":    -32700, // JSON-RPC parse error
			"message": "Parse error",
			"data": map[string]interface{}{
				"details": err.Error(),
			},
		},
	}

	responseJSON, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		// This shouldn't happen, but if it does, we have a serious problem
		return nil, errors.Wrap(marshalErr, "failed to marshal error response")
	}

	return responseJSON, nil
}

// createValidationErrorResponse creates a JSON-RPC validation error response.
// Different error codes are used based on the nature of the validation failure:
// - Invalid Request (-32600): For structural/protocol-level validation errors.
// - Invalid Params (-32602): For parameter-specific validation errors.
func createValidationErrorResponse(id interface{}, err error) ([]byte, error) {
	code := -32600 // Default to Invalid Request
	message := "Invalid Request"

	// If the error path indicates it's a parameter issue, use -32602
	var valErr *schema.ValidationError
	if errors.As(err, &valErr) {
		// Check if the error is in the params
		if valErr.InstancePath != "" && (strings.Contains(valErr.InstancePath, "/params") ||
			strings.Contains(valErr.InstancePath, "params")) {
			code = -32602
			message = "Invalid params"
		}
	}

	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id, // This might be nil for notifications, which is fine
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
			"data": map[string]interface{}{
				"details": err.Error(),
			},
		},
	}

	responseJSON, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		return nil, errors.Wrap(marshalErr, "failed to marshal error response")
	}

	return responseJSON, nil
}
