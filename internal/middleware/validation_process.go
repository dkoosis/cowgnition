// Package middleware provides chainable handlers for processing MCP messages, like validation.
package middleware

// file: internal/middleware/validation_process.go

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/schema" // Ensure schema is imported.
)

// validateIncoming handles the validation logic for incoming messages.
// It orchestrates parsing, identification, schema lookup, and validation.
// Returns: (error response bytes OR nil), msgType, reqID, (internal processing error OR nil).
func (m *ValidationMiddleware) validateIncoming(ctx context.Context, message []byte, startTime time.Time) ([]byte, string, interface{}, error) {
	// Check JSON syntax first. If invalid, we can't reliably get ID or Type.
	if !json.Valid(message) {
		preview := calculatePreview(message) // Use helper from validation_helpers.go.
		m.logger.Warn("Invalid JSON syntax received.", "messagePreview", preview)
		// ID is nil for parse error as it couldn't be reliably extracted.
		respBytes, creationErr := createParseErrorResponse(nil, errors.New("invalid JSON syntax")) // Use helper from validation_errors.go.
		if creationErr != nil {
			return nil, "", nil, errors.Wrap(creationErr, "failed to create parse error response")
		}
		return respBytes, "", nil, nil // Return parse error response.
	}

	// Identify message type (e.g., "initialize", "tools/list") and ID.
	// Fixed: Call identifyMessage directly, not as a method on m.
	msgType, reqID, identifyErr := identifyMessage(message)
	if identifyErr != nil {
		preview := calculatePreview(message)
		m.logger.Warn("Failed to identify message type/structure.", "error", identifyErr, "messagePreview", preview)
		// Use the reqID extracted *during* identification attempt, even if identification failed overall.
		respBytes, creationErr := createInvalidRequestErrorResponse(reqID, identifyErr) // Use helper from validation_errors.go.
		if creationErr != nil {
			return nil, msgType, reqID, errors.Wrap(creationErr, "failed to create invalid request error response")
		}
		return respBytes, msgType, reqID, nil // Return invalid request error response.
	}

	// Check if validation should be skipped for this type based on options.
	if m.options.SkipTypes[msgType] {
		m.logger.Debug("Skipping validation for message type per options.", "type", msgType, "requestID", reqID)
		return nil, msgType, reqID, nil // Skip validation, proceed normally.
	}

	// Determine the actual schema definition to use (might involve fallbacks).
	// Uses helper from validation_schema.go.
	schemaType := m.determineIncomingSchemaType(msgType)
	if schemaType == "" {
		// This indicates a setup issue - schema wasn't found even with fallbacks.
		missingSchemaErr := errors.Newf("Internal configuration error: No schema found for message type '%s' or its fallbacks", msgType) // Defined error here.
		m.logger.Error("CRITICAL: Could not determine schema type for incoming validation.", "error", missingSchemaErr, "messageType", msgType, "requestID", reqID)
		// Handle as internal error if strict, otherwise log warning and potentially skip.
		if m.options.StrictMode {
			// Log the specific error before creating the generic response.
			m.logger.Error("Strict mode failure due to missing schema.", "cause", missingSchemaErr)
			respBytes, creationErr := createInternalErrorResponse(reqID) // Use helper from validation_errors.go.
			if creationErr != nil {
				return nil, msgType, reqID, errors.Wrap(creationErr, "failed to create internal error for missing schema")
			}
			// Return the internal error response bytes.
			return respBytes, msgType, reqID, nil
		}
		// Non-strict: Log and skip validation for this message.
		m.logger.Warn("Skipping validation due to missing schema (non-strict mode).", "messageType", msgType, "requestID", reqID, "cause", missingSchemaErr)
		return nil, msgType, reqID, nil
	}

	// Perform validation against the determined schema.
	validationErr := m.validator.Validate(ctx, schemaType, message)

	// Log performance if enabled.
	if m.options.MeasurePerformance {
		elapsed := time.Since(startTime)
		m.logger.Debug("Incoming message validation performance.",
			"messageType", msgType, "schemaType", schemaType, "duration", elapsed, "requestID", reqID, "isValid", validationErr == nil)
	}

	// Handle validation failure.
	if validationErr != nil {
		if !m.options.StrictMode {
			// Non-strict mode: Log the error but proceed.
			m.logger.Warn("Incoming validation error ignored (non-strict mode).",
				"messageType", msgType, "requestID", reqID, "schemaTypeUsed", schemaType, "error", validationErr)
			return nil, msgType, reqID, nil // Proceed normally.
		}

		// Strict mode: Generate and return error response.
		m.logger.Warn("Incoming message validation failed (strict mode).",
			"messageType", msgType, "requestID", reqID, "schemaTypeUsed", schemaType, "error", fmt.Sprintf("%+v", validationErr))
		respBytes, creationErr := createValidationErrorResponse(reqID, validationErr) // Use helper from validation_errors.go.
		if creationErr != nil {
			// Internal error creating the response.
			return nil, msgType, reqID, errors.Wrap(creationErr, "failed to create validation error response")
		}
		return respBytes, msgType, reqID, nil // Return the validation error response.
	}

	// Validation passed.
	m.logger.Debug("Incoming message passed validation.", "messageType", msgType, "schemaTypeUsed", schemaType, "requestID", reqID)
	return nil, msgType, reqID, nil // Success, proceed normally.
}

// handleOutgoing encapsulates the logic for validating outgoing responses/notifications.
// It returns the bytes for an error response if validation fails *and* StrictOutgoing is true,
// otherwise it returns (nil, nil) to indicate the original response should be used or an internal error occurred.
// Returns: (outgoingErrorResponseBytes OR nil), (internalError OR nil).
func (m *ValidationMiddleware) handleOutgoing(ctx context.Context, responseBytes []byte, requestID interface{}) ([]byte, error) {
	// If no response or outgoing validation is disabled, do nothing.
	if responseBytes == nil || !m.options.ValidateOutgoing {
		return nil, nil
	}

	// Extract original request method from context if available.
	requestMethod, _ := ctx.Value(contextKeyRequestMethod).(string)

	// Perform the actual validation of the outgoing response bytes.
	// Delegates schema lookup and validation call.
	outgoingValidationErr := m.validateOutgoingResponse(ctx, requestMethod, responseBytes)

	// If validation passed, return nil, nil to signal using original responseBytes.
	if outgoingValidationErr == nil {
		return nil, nil
	}

	// --- Outgoing Validation Failed ---

	// If NOT in strict outgoing mode, log the error but allow the original response through.
	if !m.options.StrictOutgoing {
		m.logger.Warn("Outgoing validation error ignored (non-strict outgoing mode).",
			"requestMethod", requestMethod, // Method of original request.
			"requestID", requestID, // ID of original request.
			"error", outgoingValidationErr) // Log the actual validation error detail.
		// Return nil, nil to indicate the original responseBytes should be sent.
		return nil, nil
	}

	// --- Strict Outgoing Mode: Validation Failed ---
	// Log the failure as an error because we are replacing the response.
	m.logger.Error("Outgoing response validation failed (strict outgoing mode)! Replacing response.",
		"requestMethod", requestMethod,
		"requestID", requestID,
		"error", fmt.Sprintf("%+v", outgoingValidationErr)) // Log with stack trace if available.

	// Create a formatted JSON-RPC error response based on the validation failure.
	// Use the requestID from the *original* request for the error response ID.
	rpcErrorBytes, creationErr := createValidationErrorResponse(requestID, outgoingValidationErr) // Use helper from validation_errors.go.
	if creationErr != nil {
		// If we can't even create the validation error response, log critical and create internal error.
		m.logger.Error("CRITICAL: Failed to create validation error response for outgoing failure.",
			"creationError", fmt.Sprintf("%+v", creationErr),
			"originalValidationError", fmt.Sprintf("%+v", outgoingValidationErr))

		// Attempt to create a generic internal error response.
		internalErrResp, creationErr2 := createInternalErrorResponse(requestID) // Use helper from validation_errors.go.
		if creationErr2 != nil {
			// If even *that* fails, return the marshalling error directly. Critical failure.
			return nil, errors.Wrap(creationErr2, "critical: failed marshalling even internal error response")
		}
		// Return the generic internal error response bytes, nil error (as we handled creation).
		return internalErrResp, nil
	}

	// Return the created validation error response bytes. The 'error' return is nil because
	// we successfully created the error *response* to send back.
	return rpcErrorBytes, nil
}

// validateOutgoingResponse performs validation on the outgoing response bytes.
// It determines the schema and calls the validator.
func (m *ValidationMiddleware) validateOutgoingResponse(ctx context.Context, requestMethod string, responseBytes []byte) error {
	// Skip validation for standard JSON-RPC error responses.
	if isErrorResponse(responseBytes) { // Use helper from validation_helpers.go.
		m.logger.Debug("Skipping outgoing validation for JSON-RPC error response.")
		return nil
	}

	// Extract the "result" field for validation.
	var respWrapper struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(responseBytes, &respWrapper); err != nil {
		// This might happen if the response isn't a standard success response format.
		// Log it, but don't fail validation outright here, as it might be valid in other ways or an error itself.
		m.logger.Warn("Could not extract 'result' field from outgoing response for validation.",
			"error", err, "responsePreview", calculatePreview(responseBytes)) // Use helper.
		// Depending on strictness/requirements, could return an error here.
		// For now, we'll let it proceed, and schema determination might fallback.
	}

	// --- FIX: Simplified nil/empty check based on staticcheck S1009 ---
	// Handle cases where result is empty or explicitly null.
	if len(respWrapper.Result) == 0 || string(respWrapper.Result) == "null" {
		// --- END FIX ---
		m.logger.Debug("Skipping outgoing validation for empty or null 'result' field.", "requestMethod", requestMethod)
		return nil // Nothing to validate inside the result.
	}

	// Determine the schema type based on the original request method or response content.
	// Uses helper from validation_schema.go.
	schemaType := m.determineOutgoingSchemaType(requestMethod, responseBytes) // Pass original responseBytes for type determination heuristics.
	if schemaType == "" {
		responsePreview := calculatePreview(responseBytes) // Use helper.
		m.logger.Warn("Could not determine schema type for outgoing validation.",
			"requestMethod", requestMethod,
			"responsePreview", responsePreview)
		if m.options.StrictOutgoing {
			return NewValidationError(
				schema.ErrSchemaNotFound,
				"Could not determine schema type for outgoing validation",
				nil,
			).WithContext("requestMethod", requestMethod)
		}
		return nil // Non-strict, ignore missing schema.
	}

	// Perform validation on the extracted 'result' content.
	validationErr := m.validator.Validate(ctx, schemaType, respWrapper.Result) // Validate respWrapper.Result.

	if validationErr != nil {
		// Log detailed context if validation fails.
		responseMsgType, responseReqID, _ := identifyMessage(responseBytes) // Identify original response for logging context.
		preview := calculatePreview(responseBytes)

		// Perform specific checks for known outgoing types if validation fails (e.g., tool names).
		if strings.HasPrefix(schemaType, "tools/") && strings.HasSuffix(schemaType, "_response") {
			m.performToolNameValidation(responseBytes) // Use helper from validation_helpers.go.
		}

		m.logger.Debug("Outgoing response validation failed.",
			"requestMethod", requestMethod,
			"responseMsgType", responseMsgType,
			"responseReqID", responseReqID,
			"schemaTypeUsed", schemaType,
			"errorDetail", validationErr,
			"responsePreview", preview)

		return validationErr
	}

	// Validation passed.
	m.logger.Debug("Outgoing response passed validation.", "schemaTypeUsed", schemaType, "requestMethod", requestMethod)
	return nil
}
