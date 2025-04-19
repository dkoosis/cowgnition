// Package middleware provides chainable handlers for processing MCP messages, like validation.
package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"

	// Import schema package to use the interface.
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
)

// SchemaValidatorInterface is now defined in the schema package.
// type SchemaValidatorInterface interface { ... } // REMOVED.

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
	contextKeyRequestMethod contextKey = "requestMethod"
)

// DefaultValidationOptions returns a ValidationOptions struct with default values.
func DefaultValidationOptions() ValidationOptions {
	return ValidationOptions{
		Enabled:            true,
		SkipTypes:          map[string]bool{"ping": true}, // ping is often skipped.
		StrictMode:         true,                          // Fail on validation errors by default.
		MeasurePerformance: false,                         // Performance timing disabled by default.
		ValidateOutgoing:   true,                          // Validate server responses by default.
		StrictOutgoing:     false,                         // Don't fail requests due to outgoing validation errors by default.
	}
}

// ValidationMiddleware performs JSON schema validation on incoming and outgoing messages.
type ValidationMiddleware struct {
	// Corrected: Use the interface defined in the schema package (now ValidatorInterface).
	validator schema.ValidatorInterface
	options   ValidationOptions
	next      transport.MessageHandler
	logger    logging.Logger
}

// NewValidationMiddleware creates a new ValidationMiddleware instance.
// Corrected: Accepts the interface from the schema package (now ValidatorInterface).
func NewValidationMiddleware(validator schema.ValidatorInterface, options ValidationOptions, logger logging.Logger) *ValidationMiddleware {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	if validator == nil {
		logger.Error("CRITICAL: NewValidationMiddleware called with a nil validator interface.")
		// Panic or return error might be appropriate depending on application guarantees.
		// For now, return the struct, but it will likely panic later.
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
func (m *ValidationMiddleware) HandleMessage(ctx context.Context, message []byte) ([]byte, error) {
	// Fast path: Validation disabled or validator not ready.
	if !m.options.Enabled || m.validator == nil || !m.validator.IsInitialized() {
		m.logSkip("Validation disabled or validator not initialized.")
		return m.callNext(ctx, message)
	}

	var startTime time.Time
	if m.options.MeasurePerformance {
		startTime = time.Now()
	}

	// 1. Validate Incoming Message.
	errorResponseBytes, msgType, reqID, internalError := m.validateIncoming(ctx, message, startTime)

	if internalError != nil {
		m.logger.Error("Internal error during incoming validation.", "error", fmt.Sprintf("%+v", internalError))
		respBytes, creationErr := createInternalErrorResponse(reqID)
		if creationErr != nil {
			return nil, errors.Wrap(creationErr, "critical: failed to create internal error response")
		}
		return respBytes, nil
	}
	if errorResponseBytes != nil {
		return errorResponseBytes, nil
	}

	// 2. Prepare context and call Next Handler.
	ctxWithMsgType := context.WithValue(ctx, contextKeyRequestMethod, msgType)
	responseBytes, nextErr := m.callNext(ctxWithMsgType, message)

	// 3. Handle Error from Next Handler (if any).
	if nextErr != nil {
		m.logger.Debug("Error received from next handler, propagating.", "error", nextErr)
		return nil, nextErr // Let central error handling create JSON-RPC error.
	}

	// 4. Validate Outgoing Response (if enabled and next handler succeeded).
	outgoingErrorResponseBytes, outgoingInternalError := m.handleOutgoing(ctxWithMsgType, responseBytes, reqID)

	if outgoingInternalError != nil {
		m.logger.Error("Internal error during outgoing validation handling.", "error", fmt.Sprintf("%+v", outgoingInternalError))
		respBytes, creationErr := createInternalErrorResponse(reqID)
		if creationErr != nil {
			return nil, errors.Wrap(creationErr, "critical: failed to create internal error response for outgoing validation failure")
		}
		return respBytes, nil
	}
	if outgoingErrorResponseBytes != nil {
		return outgoingErrorResponseBytes, nil
	}

	// 5. Return Successful (and potentially validated) Response from next handler.
	return responseBytes, nil
}

// callNext safely calls the next handler in the chain.
func (m *ValidationMiddleware) callNext(ctx context.Context, message []byte) ([]byte, error) {
	if m.next == nil {
		m.logger.Error("Validation middleware reached end of chain without a final handler.")
		return nil, errors.New("validation middleware has no next handler configured")
	}
	return m.next(ctx, message)
}

// logSkip logs when validation is skipped.
func (m *ValidationMiddleware) logSkip(reason string) {
	m.logger.Debug("Validation skipped.", "reason", reason)
}

// Other files in middleware (validation_*.go) should already be using the interface.
// via the `validator` field of the ValidationMiddleware struct, so they likely don't need changes.
// besides potentially importing the schema package if they reference specific error types from there.
