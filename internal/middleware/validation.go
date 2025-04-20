// Package middleware provides chainable handlers for processing MCP messages, like validation.
package middleware

// file: internal/middleware/validation.go.

import (
	"context"
	"time"

	"github.com/dkoosis/cowgnition/internal/logging"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
)

// ValidationMiddleware provides JSON schema validation for MCP messages.
// It relies on a ValidatorInterface implementation to perform the actual schema checks.
type ValidationMiddleware struct {
	validator mcptypes.ValidatorInterface // Use type from mcptypes.
	options   mcptypes.ValidationOptions  // Use type from mcptypes.
	logger    logging.Logger
}

// DefaultValidationOptions returns default validation options.
// Note: This function now returns the type defined in mcptypes.
func DefaultValidationOptions() mcptypes.ValidationOptions { // Return type from mcptypes.
	return mcptypes.ValidationOptions{
		Enabled:            true,
		StrictMode:         false,
		ValidateOutgoing:   false,
		StrictOutgoing:     false,
		MeasurePerformance: false,
		SkipTypes:          make(map[string]bool),
	}
}

// contextKey is a type for context keys to avoid collisions.
type contextKey string

// contextKeyRequestMethod holds the MCP method name for correlating requests/responses.
const contextKeyRequestMethod contextKey = "requestMethod"

// NewValidationMiddleware creates a new validation middleware function.
// It takes a ValidatorInterface, options, and a logger.
// Returns a MiddlewareFunc that can be added to a processing chain.
func NewValidationMiddleware(validator mcptypes.ValidatorInterface, // Use type from mcptypes.
	options mcptypes.ValidationOptions, logger logging.Logger) mcptypes.MiddlewareFunc { // Use type from mcptypes.
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	// Create the middleware struct instance.
	mw := &ValidationMiddleware{
		validator: validator,
		options:   options,
		logger:    logger.WithField("component", "validation_middleware"),
	}

	// Return the actual middleware function (closure).
	return mw.Middleware // Return the method itself.
}

// Middleware is the core middleware function logic.
// It implements the MiddlewareFunc signature.
func (m *ValidationMiddleware) Middleware(next mcptypes.MessageHandler) mcptypes.MessageHandler { // Use types from mcptypes.
	return func(ctx context.Context, message []byte) ([]byte, error) {
		// 1. Check if validation is enabled globally.
		if !m.options.Enabled {
			m.logger.Debug("Validation skipped (globally disabled).")
			return next(ctx, message)
		}

		// 2. Check if validator is initialized.
		if m.validator == nil || !m.validator.IsInitialized() {
			m.logger.Warn("Validation skipped (validator not initialized or nil).")
			// Depending on strictness, could return error here, but for now, bypass.
			return next(ctx, message)
		}

		// 3. Incoming Validation.
		startTime := time.Now() // For performance measurement.
		// validateIncoming returns potential error response bytes, msgType, reqID, and internal processing error.
		errorRespBytes, msgType, reqID, incomingErr := m.validateIncoming(ctx, message, startTime) // Uses helper from validation_process.go.

		// If validateIncoming encountered an internal processing error (e.g., failed to create error response).
		if incomingErr != nil {
			m.logger.Error("Internal error during incoming validation.", "error", incomingErr)
			// Attempt to create a generic internal error response.
			// Use extracted reqID if available, otherwise null.
			internalErrRespBytes, creationErr := createInternalErrorResponse(reqID) // Use helper from validation_errors.go.
			if creationErr != nil {
				// Critical failure: Can't even create the internal error response.
				// Return the creation error directly, likely terminating the connection handler.
				return nil, creationErr
			}
			// Successfully created internal error bytes, return them.
			return internalErrRespBytes, nil
		}

		// If validateIncoming returned error response bytes (due to invalid JSON or strict validation failure).
		if errorRespBytes != nil {
			// A validation/parse error occurred, and strict mode required returning an error response.
			// Do not call the next handler. Return the generated error response directly.
			m.logger.Debug("Returning JSON-RPC error due to incoming validation failure.", "requestID", reqID, "messageType", msgType)
			return errorRespBytes, nil
		}
		// If we reach here: incoming message was valid OR non-strict mode allowed proceeding despite errors.

		// Add message type to context for outgoing validation correlation.
		if msgType != "" {
			ctx = context.WithValue(ctx, contextKeyRequestMethod, msgType)
		}

		// 4. Call Next Handler.
		responseBytes, nextErr := next(ctx, message)

		// 5. Handle errors from the next handler (application logic errors).
		if nextErr != nil {
			m.logger.Warn("Error returned from next handler.", "error", nextErr, "requestID", reqID, "messageType", msgType)
			// Map this application error to a JSON-RPC error response.
			// It's crucial *not* to validate this error response itself.
			// Removed unused variables appErrRespBytes, creationErr and replaced undefined createErrorResponse call.
			// The original intent here might need revisiting to properly map nextErr to a JSON-RPC response,
			// possibly outside this middleware as noted in the code comments previously.
			// For now, just propagating the error.
			_ /* appErrRespBytes */, creationErr := createInternalErrorResponse(reqID) // Placeholder call to a defined func, ignoring result for now.
			if creationErr != nil {
				m.logger.Error("Failed to create placeholder error response while handling nextErr.", "error", creationErr)
			}

			// Propagate the error from the next handler.
			return nil, nextErr
		}

		// 6. Outgoing Validation (only if enabled and response exists).
		outgoingErrRespBytes, outgoingValidationErr := m.handleOutgoing(ctx, responseBytes, reqID) // Uses helper from validation_process.go.

		// If handleOutgoing encountered an internal error (e.g., failed marshalling its own error response).
		if outgoingValidationErr != nil {
			m.logger.Error("Internal error during outgoing validation handling.", "error", outgoingValidationErr)
			// Return the internal error; this is a server problem.
			return nil, outgoingValidationErr
		}

		// If outgoing validation failed in strict mode and generated an error response.
		if outgoingErrRespBytes != nil {
			m.logger.Debug("Replacing original response with error due to outgoing validation failure.", "requestID", reqID, "messageType", msgType)
			return outgoingErrRespBytes, nil // Return the generated error response instead of original.
		}

		// If we reach here: Outgoing validation passed OR failed in non-strict mode.
		// Return the original response bytes from the `next` handler.
		return responseBytes, nil
	}
}

// SetNext is a helper method to set the next handler in the chain.
// Note: This method is deprecated as it doesn't fit the standard MiddlewareFunc pattern.
func (m *ValidationMiddleware) SetNext(_ mcptypes.MessageHandler) {
	// This method is deprecated in favor of chain composition using NewChain.
	m.logger.Warn("SetNext called on ValidationMiddleware; this method is deprecated.")
}
