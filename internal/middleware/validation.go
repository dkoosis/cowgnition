// Package middleware provides chainable handlers for processing MCP messages, like validation.
package middleware

// file: internal/middleware/validation.go

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/transport"
)

// SchemaValidatorInterface defines the methods needed for schema validation.
// Consider moving this interface to a more central package if used elsewhere,
// or keep it here if specific to this middleware's needs.
type SchemaValidatorInterface interface {
	Validate(ctx context.Context, messageType string, data []byte) error
	HasSchema(name string) bool
	IsInitialized() bool
	Initialize(ctx context.Context) error
	GetLoadDuration() time.Duration
	GetCompileDuration() time.Duration
	Shutdown() error
}

// ValidationOptions configures the behavior of the ValidationMiddleware.
type ValidationOptions struct {
	Enabled            bool
	SkipTypes          map[string]bool
	StrictMode         bool // Fail request on incoming validation errors.
	MeasurePerformance bool
	ValidateOutgoing   bool // Validate server responses/notifications.
	StrictOutgoing     bool // Fail request if outgoing response validation fails.
}

// contextKey is a type for context keys to avoid collisions.
type contextKey string

const (
	// contextKeyRequestMethod stores the identified method of the incoming request
	// for use during outgoing response validation.
	contextKeyRequestMethod contextKey = "requestMethod"
)

// DefaultValidationOptions returns a ValidationOptions struct with default values.
func DefaultValidationOptions() ValidationOptions {
	return ValidationOptions{
		Enabled:            true,
		SkipTypes:          map[string]bool{"ping": true}, // ping is often skipped as a basic health check.
		StrictMode:         true,                          // Fail on validation errors by default.
		MeasurePerformance: false,                         // Performance timing disabled by default.
		ValidateOutgoing:   true,                          // Validate server responses by default.
		StrictOutgoing:     false,                         // Don't fail requests due to outgoing validation errors by default.
	}
}

// ValidationMiddleware performs JSON schema validation on incoming and outgoing messages.
type ValidationMiddleware struct {
	validator SchemaValidatorInterface
	options   ValidationOptions
	next      transport.MessageHandler
	logger    logging.Logger
}

// NewValidationMiddleware creates a new ValidationMiddleware instance.
func NewValidationMiddleware(validator SchemaValidatorInterface, options ValidationOptions, logger logging.Logger) *ValidationMiddleware {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	if validator == nil {
		// Added more explicit error logging here.
		logger.Error("CRITICAL: NewValidationMiddleware called with a nil validator interface.")
		// Panic might be appropriate here if a nil validator is unrecoverable.
		// Alternatively, return an error if the function signature allows.
		// For now, log and continue, but this likely indicates a programming error.
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

// HandleMessage processes a message, performing validation before passing it on.
// It orchestrates calls to validation logic defined in other files.
func (m *ValidationMiddleware) HandleMessage(ctx context.Context, message []byte) ([]byte, error) {
	// Fast path: Validation disabled or validator not ready.
	if !m.options.Enabled || m.validator == nil || !m.validator.IsInitialized() {
		m.logSkip("Validation disabled or validator not initialized")
		return m.callNext(ctx, message)
	}

	var startTime time.Time
	if m.options.MeasurePerformance {
		startTime = time.Now()
	}

	// 1. Validate Incoming Message.
	// validateIncoming delegates parsing, identification, schema checks, and validation.
	// It returns:
	// - errorResponseBytes: Non-nil if validation failed strictly / parse error / invalid request occurred.
	// - msgType: The identified type (e.g., "initialize", "tools/list").
	// - reqID: The identified request ID (can be nil for notifications or parse errors).
	// - internalError: Non-nil if an internal error occurred during validation processing itself.
	errorResponseBytes, msgType, reqID, internalError := m.validateIncoming(ctx, message, startTime)

	// Handle internal errors during incoming validation (e.g., failed to create error response).
	if internalError != nil {
		m.logger.Error("Internal error during incoming validation.", "error", fmt.Sprintf("%+v", internalError))
		respBytes, creationErr := createInternalErrorResponse(reqID) // Use helper from validation_errors.go.
		if creationErr != nil {
			return nil, errors.Wrap(creationErr, "critical: failed to create internal error response")
		}
		return respBytes, nil // Return the generic internal error response.
	}
	// Handle strict validation failures / parse errors / invalid requests.
	if errorResponseBytes != nil {
		return errorResponseBytes, nil // Return the specific error response generated by validateIncoming.
	}

	// 2. Prepare context and call Next Handler.
	// Add identified request method to context for potential use in outgoing validation.
	ctxWithMsgType := context.WithValue(ctx, contextKeyRequestMethod, msgType)
	responseBytes, nextErr := m.callNext(ctxWithMsgType, message)

	// 3. Handle Error from Next Handler (if any).
	if nextErr != nil {
		m.logger.Debug("Error received from next handler, propagating.", "error", nextErr)
		// Forward the error; allow central error handling (e.g., in mcp_server)
		// to create the appropriate error response.
		return nil, nextErr
	}

	// 4. Validate Outgoing Response (if enabled and next handler succeeded).
	// handleOutgoing delegates the validation of the response from the next handler.
	// It returns:
	// - outgoingErrorResponseBytes: Non-nil *only* if outgoing validation failed *and* StrictOutgoing is true.
	// - outgoingInternalError: Non-nil if an internal error occurred during outgoing validation (e.g., marshalling error).
	outgoingErrorResponseBytes, outgoingInternalError := m.handleOutgoing(ctxWithMsgType, responseBytes, reqID)

	// Handle internal errors during outgoing validation.
	if outgoingInternalError != nil {
		m.logger.Error("Internal error during outgoing validation handling.", "error", fmt.Sprintf("%+v", outgoingInternalError))
		respBytes, creationErr := createInternalErrorResponse(reqID)
		if creationErr != nil {
			return nil, errors.Wrap(creationErr, "critical: failed to create internal error response for outgoing validation failure")
		}
		return respBytes, nil // Return internal error response bytes.
	}
	// Handle strict outgoing validation failures.
	if outgoingErrorResponseBytes != nil {
		// Validation failed in strict mode, return the generated error response.
		return outgoingErrorResponseBytes, nil
	}

	// 5. Return Successful (and potentially validated) Response from next handler.
	return responseBytes, nil
}

// callNext safely calls the next handler in the chain.
func (m *ValidationMiddleware) callNext(ctx context.Context, message []byte) ([]byte, error) {
	if m.next == nil {
		// This indicates a configuration error in the middleware chain setup.
		m.logger.Error("Validation middleware reached end of chain without a final handler.")
		return nil, errors.New("validation middleware has no next handler configured")
	}
	return m.next(ctx, message)
}

// logSkip logs when validation is skipped.
func (m *ValidationMiddleware) logSkip(reason string) {
	// Use Debug level as skipping is often expected behaviour.
	m.logger.Debug("Validation skipped.", "reason", reason)
}
