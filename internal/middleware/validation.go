// Package middleware provides chainable handlers for processing MCP messages, like validation.
package middleware

import (
	"context"

	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/mcp_type"
)

// ValidationMiddleware provides JSON schema validation for MCP messages.
type ValidationMiddleware struct {
	validator mcp_type.ValidatorInterface
	options   mcp_type.ValidationOptions
	logger    logging.Logger
}

// DefaultValidationOptions returns default validation options.
func DefaultValidationOptions() mcp_type.ValidationOptions {
	return mcp_type.ValidationOptions{
		StrictMode:         false,
		ValidateOutgoing:   false,
		StrictOutgoing:     false,
		MeasurePerformance: false,
	}
}

// NewValidationMiddleware creates a new validation middleware.
func NewValidationMiddleware(validator mcp_type.ValidatorInterface,
	options mcp_type.ValidationOptions, logger logging.Logger) mcp_type.MiddlewareFunc {

	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	mw := &ValidationMiddleware{
		validator: validator,
		options:   options,
		logger:    logger.WithField("component", "validation_middleware"),
	}

	return mw.Middleware
}

// Middleware returns a middleware function that validates messages.
func (mw *ValidationMiddleware) Middleware(next mcp_type.MessageHandler) mcp_type.MessageHandler {
	return func(ctx context.Context, message []byte) ([]byte, error) {
		// Placeholder for validation implementation.
		// The actual implementation would:
		// 1. Identify the message type
		// 2. Validate the incoming message
		// 3. Call the next handler
		// 4. Validate the outgoing response if configured

		// Process with the next handler.
		response, err := next(ctx, message)

		// Return the response.
		return response, err
	}
}
