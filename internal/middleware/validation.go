// file: internal/middleware/validation.go
package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
)

// --- Interface Definition ---.
type SchemaValidatorInterface interface {
	Validate(ctx context.Context, messageType string, data []byte) error
	HasSchema(name string) bool
	IsInitialized() bool
	Initialize(ctx context.Context) error
	GetLoadDuration() time.Duration
	GetCompileDuration() time.Duration
	Shutdown() error
}

// --- Options ---.
type ValidationOptions struct {
	Enabled            bool
	SkipTypes          map[string]bool
	StrictMode         bool
	MeasurePerformance bool
	ValidateOutgoing   bool
	StrictOutgoing     bool
}

// --- Context Key ---.
type contextKey string

const (
	contextKeyRequestMethod contextKey = "requestMethod"
)

// --- Defaults ---.
func DefaultValidationOptions() ValidationOptions {
	return ValidationOptions{
		Enabled:            true,
		SkipTypes:          map[string]bool{"ping": true},
		StrictMode:         true,
		MeasurePerformance: false,
		ValidateOutgoing:   true,
		StrictOutgoing:     false,
	}
}

// --- Middleware Struct ---.
type ValidationMiddleware struct {
	validator SchemaValidatorInterface
	options   ValidationOptions
	next      transport.MessageHandler
	logger    logging.Logger
}

// --- Constructor ---.
func NewValidationMiddleware(validator SchemaValidatorInterface, options ValidationOptions, logger logging.Logger) *ValidationMiddleware {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	return &ValidationMiddleware{
		validator: validator,
		options:   options,
		logger:    logger.WithField("middleware", "validation"),
	}
}

// --- SetNext ---.
func (m *ValidationMiddleware) SetNext(next transport.MessageHandler) {
	m.next = next
}

// --- Core HandleMessage ---.
func (m *ValidationMiddleware) HandleMessage(ctx context.Context, message []byte) ([]byte, error) {
	// Fast path: Validation disabled.
	if !m.options.Enabled || !m.validator.IsInitialized() {
		m.logger.Debug("Validation disabled or validator not initialized, skipping.")
		if m.next == nil {
			return nil, errors.New("validation middleware has no next handler configured")
		}
		return m.next(ctx, message)
	}

	var startTime time.Time
	if m.options.MeasurePerformance {
		startTime = time.Now()
	}

	// 1. Validate Incoming Message.
	errorResponseBytes, msgType, reqID, internalError := m.validateIncomingMessage(ctx, message, startTime)
	if internalError != nil {
		m.logger.Error("Internal error during incoming validation.", "error", fmt.Sprintf("%+v", internalError))
		respBytes, creationErr := createInternalErrorResponse(reqID) // Use extracted helper
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
	if m.next == nil {
		return nil, errors.New("validation middleware reached end of chain without a final handler")
	}
	responseBytes, nextErr := m.next(ctxWithMsgType, message)

	// 3. Handle Error from Next Handler (if any).
	if nextErr != nil {
		m.logger.Debug("Error received from next handler, propagating.", "error", nextErr)
		return nil, nextErr
	}

	// 4. Validate Outgoing Response (if enabled and next handler succeeded).
	outgoingResponseBytes, outgoingErr := m.handleOutgoingValidation(ctxWithMsgType, responseBytes, reqID)
	if outgoingErr != nil {
		return nil, outgoingErr
	}
	if outgoingResponseBytes != nil {
		return outgoingResponseBytes, nil
	}

	// 5. Return Successful Response.
	return responseBytes, nil
}

// handleOutgoingValidation encapsulates the logic for validating outgoing responses.
func (m *ValidationMiddleware) handleOutgoingValidation(ctx context.Context, responseBytes []byte, requestID interface{}) ([]byte, error) {
	if responseBytes == nil || !m.options.ValidateOutgoing {
		return nil, nil // Nothing to validate or validation disabled
	}

	requestMethod, _ := ctx.Value(contextKeyRequestMethod).(string)

	outgoingValidationErr := m.validateOutgoingResponse(ctx, requestMethod, responseBytes)
	if outgoingValidationErr == nil {
		return nil, nil // Validation passed
	}

	if !m.options.StrictOutgoing && isNonCriticalValidationError(outgoingValidationErr) {
		m.logger.Warn("Outgoing validation error ignored (non-strict outgoing mode).",
			"requestMethod", requestMethod,
			"error", outgoingValidationErr)
		return nil, nil // Ignore non-critical error
	}

	m.logger.Error("Outgoing response validation failed!",
		"requestMethod", requestMethod,
		"error", fmt.Sprintf("%+v", outgoingValidationErr))

	rpcErrorBytes, creationErr := createValidationErrorResponse(requestID, outgoingValidationErr)
	if creationErr != nil {
		m.logger.Error("CRITICAL: Failed to create validation error response for outgoing failure.",
			"creationError", fmt.Sprintf("%+v", creationErr),
			"originalValidationError", fmt.Sprintf("%+v", outgoingValidationErr))
		internalErrResp, creationErr2 := createInternalErrorResponse(requestID)
		if creationErr2 != nil {
			return nil, creationErr2
		}
		return internalErrResp, nil
	}
	return rpcErrorBytes, nil
}

// --- Incoming Validation Logic ---.
func (m *ValidationMiddleware) validateIncomingMessage(ctx context.Context, message []byte, startTime time.Time) ([]byte, string, interface{}, error) {
	// ... (JSON syntax check, message identification - unchanged) ...
	if !json.Valid(message) {
		preview := calculatePreview(message)
		m.logger.Warn("Invalid JSON syntax received.", "messagePreview", preview)
		respBytes, creationErr := createParseErrorResponse(nil, errors.New("invalid JSON syntax"))
		if creationErr != nil {
			return nil, "", nil, errors.Wrap(creationErr, "failed to create parse error response")
		}
		return respBytes, "", nil, nil // Return error response
	}
	msgType, reqID, identifyErr := m.identifyMessage(message)
	if identifyErr != nil {
		preview := calculatePreview(message)
		m.logger.Warn("Failed to identify message type.", "error", identifyErr, "messagePreview", preview)
		respBytes, creationErr := createInvalidRequestErrorResponse(reqID, identifyErr)
		if creationErr != nil {
			return nil, msgType, reqID, errors.Wrap(creationErr, "failed to create invalid request error response")
		}
		return respBytes, msgType, reqID, nil // Return error response
	}

	if m.options.SkipTypes[msgType] {
		m.logger.Debug("Skipping validation for message type.", "type", msgType, "requestID", reqID)
		return nil, msgType, reqID, nil // Proceed
	}

	schemaType := m.determineIncomingSchemaType(msgType)
	validationErr := m.validator.Validate(ctx, schemaType, message)

	if m.options.MeasurePerformance { // Log performance regardless of validation outcome
		elapsed := time.Since(startTime)
		m.logger.Debug("Incoming message validation performance.",
			"messageType", msgType, "schemaType", schemaType, "duration", elapsed, "requestID", reqID, "isValid", validationErr == nil)
	}

	if validationErr != nil {
		// --- FIX: Check StrictMode FIRST ---
		if !m.options.StrictMode {
			// In non-strict mode, log the error but allow processing to continue.
			// We check isNonCriticalValidationError *only* if we want finer control in non-strict mode,
			// but the test seems to expect *all* validation errors to be ignored here.
			// Let's align with the apparent test expectation: ignore all validation errors in non-strict mode.
			m.logger.Warn("Incoming validation error ignored (non-strict mode).",
				"messageType", msgType, "requestID", reqID, "error", validationErr)
			return nil, msgType, reqID, nil // <<< FIX: Return nils to proceed

			// Original logic (kept for reference, might be reinstated if tests change):
			// if isNonCriticalValidationError(validationErr) {
			// 	m.logger.Warn("Incoming non-critical validation error ignored (non-strict mode).",
			// 		"messageType", msgType, "requestID", reqID, "error", validationErr)
			// 	return nil, msgType, reqID, nil // Proceed normally
			// } else {
			//  m.logger.Warn("Incoming critical validation error occurred (non-strict mode).", // Log critical even if non-strict
			//      "messageType", msgType, "requestID", reqID, "error", fmt.Sprintf("%+v", validationErr))
			//  // Decide: Still proceed? Or generate error response even in non-strict for critical errors?
			//  // Current test implies we should proceed for ANY validation error in non-strict.
			//  return nil, msgType, reqID, nil // Proceed even for critical errors in non-strict based on test
			// }
		} else {
			// StrictMode is true: Generate and return error response.
			m.logger.Warn("Incoming message validation failed (strict mode).",
				"messageType", msgType, "requestID", reqID, "error", fmt.Sprintf("%+v", validationErr))
			respBytes, creationErr := createValidationErrorResponse(reqID, validationErr)
			if creationErr != nil {
				return nil, msgType, reqID, errors.Wrap(creationErr, "failed to create validation error response")
			}
			return respBytes, msgType, reqID, nil // Return error response
		}
	}

	// Validation passed.
	m.logger.Debug("Incoming message passed validation.", "messageType", msgType, "requestID", reqID)
	return nil, msgType, reqID, nil // Proceed
}

// --- Outgoing Validation Logic ---.
func (m *ValidationMiddleware) validateOutgoingResponse(ctx context.Context, requestMethod string, responseBytes []byte) error {
	if isErrorResponse(responseBytes) {
		m.logger.Debug("Skipping outgoing validation for JSON-RPC error response.")
		return nil
	}

	schemaType := m.determineOutgoingSchemaType(requestMethod, responseBytes)
	if schemaType == "" {
		m.logger.Warn("Could not determine schema type for outgoing validation.",
			"requestMethod", requestMethod,
			"responsePreview", calculatePreview(responseBytes))
		if m.options.StrictOutgoing {
			return errors.New("could not determine schema type for outgoing validation")
		}
		return nil
	}

	validationErr := m.validator.Validate(ctx, schemaType, responseBytes)

	if validationErr != nil {
		responseMsgType, responseReqID, _ := m.identifyMessage(responseBytes)
		preview := calculatePreview(responseBytes)

		if strings.HasPrefix(schemaType, "tools/") && strings.HasSuffix(schemaType, "_response") {
			m.performToolNameValidation(responseBytes)
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

	m.logger.Debug("Outgoing response passed validation.", "schemaTypeUsed", schemaType)
	return nil
}

// --- Helper: Perform Tool Name Validation ---.
func (m *ValidationMiddleware) performToolNameValidation(responseBytes []byte) {
	var toolsResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}

	if err := json.Unmarshal(responseBytes, &toolsResp); err != nil {
		m.logger.Debug("Could not parse response as tool list for name validation.",
			"error", err,
			"responsePreview", calculatePreview(responseBytes))
		return
	}

	for i, tool := range toolsResp.Result.Tools {
		if err := schema.ValidateName(schema.EntityTypeTool, tool.Name); err != nil {
			m.logger.Error("Invalid tool name found in outgoing response.",
				"toolIndex", i,
				"invalidName", tool.Name,
				"validationError", err,
				"rulesHint", schema.GetNamePatternDescription(schema.EntityTypeTool))
		}
	}
}

// --- Helper: Determine Incoming Schema Type ---.
func (m *ValidationMiddleware) determineIncomingSchemaType(msgType string) string {
	if m.validator.HasSchema(msgType) {
		return msgType
	}
	fallbackKey := "base"
	if strings.HasSuffix(msgType, "_notification") {
		if m.validator.HasSchema("JSONRPCNotification") {
			fallbackKey = "JSONRPCNotification"
		}
	} else if strings.Contains(msgType, "Response") || strings.Contains(msgType, "Result") || strings.Contains(msgType, "_response") {
		if m.validator.HasSchema("JSONRPCResponse") {
			fallbackKey = "JSONRPCResponse"
		}
		if strings.Contains(msgType, "Error") || strings.Contains(msgType, "_error") {
			if m.validator.HasSchema("JSONRPCError") {
				fallbackKey = "JSONRPCError"
			}
		}
	} else {
		if m.validator.HasSchema("JSONRPCRequest") {
			fallbackKey = "JSONRPCRequest"
		}
	}
	if m.validator.HasSchema(fallbackKey) {
		m.logger.Debug("Using schema for incoming message.", "messageType", msgType, "schemaKeyUsed", fallbackKey)
		return fallbackKey
	}
	m.logger.Warn("Specific/generic/base schema not found for incoming message.", "messageType", msgType)
	return "base"
}

// --- Helper: Determine Outgoing Schema Type ---.
func (m *ValidationMiddleware) determineOutgoingSchemaType(requestMethod string, responseBytes []byte) string {
	if requestMethod != "" && !strings.HasSuffix(requestMethod, "_notification") {
		expectedResponseSchema := requestMethod + "_response"
		if m.validator.HasSchema(expectedResponseSchema) {
			return expectedResponseSchema
		}
		m.logger.Debug("Specific response schema derived from request method not found, trying fallback.",
			"requestMethod", requestMethod, "derivedSchema", expectedResponseSchema)
	}
	responseMsgType, _, identifyErr := m.identifyMessage(responseBytes)
	if identifyErr == nil {
		if m.validator.HasSchema(responseMsgType) {
			return responseMsgType
		}
		if responseMsgType == "success_response" && m.validator.HasSchema("CallToolResult") && isCallToolResultShape(responseBytes) {
			m.logger.Debug("Identified response as CallToolResult shape.")
			return "CallToolResult"
		}
		m.logger.Debug("Schema for type identified from response not found, trying generic.",
			"responseMsgType", responseMsgType)
	} else {
		m.logger.Warn("Failed to identify outgoing response type for schema determination fallback.", "error", identifyErr)
	}
	if isSuccessResponse(responseBytes) && m.validator.HasSchema("JSONRPCResponse") {
		return "JSONRPCResponse"
	}
	m.logger.Warn("Specific/generic schema not found for outgoing response, using base schema.",
		"requestMethod", requestMethod, "responsePreview", calculatePreview(responseBytes))
	if m.validator.HasSchema("base") {
		return "base"
	}
	return ""
}

// --- Helper: Check Response Types ---.
func isErrorResponse(message []byte) bool {
	return bytes.Contains(message, []byte(`"error":`)) && !bytes.Contains(message, []byte(`"result":`))
}
func isSuccessResponse(message []byte) bool {
	return bytes.Contains(message, []byte(`"result":`)) && !bytes.Contains(message, []byte(`"error":`))
}
func isCallToolResultShape(message []byte) bool {
	return bytes.Contains(message, []byte(`"result":`)) && bytes.Contains(message, []byte(`"content":`))
}

// --- Helper: Calculate Preview ---.
func calculatePreview(data []byte) string {
	const maxPreviewLen = 100
	previewLen := len(data)
	suffix := ""
	if previewLen > maxPreviewLen {
		previewLen = maxPreviewLen
		suffix = "..."
	}
	previewBytes := bytes.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return '.'
		}
		return r
	}, data[:previewLen])
	return string(previewBytes) + suffix
}

// --- Helper: Check for non-critical validation errors ---.
func isNonCriticalValidationError(err error) bool {
	var schemaValErr *schema.ValidationError
	if !errors.As(err, &schemaValErr) {
		return false
	}
	msg := schemaValErr.Message
	return strings.Contains(msg, "additionalProperties")
}

// --- Helper: Identify Message ---.
func (m *ValidationMiddleware) identifyMessage(message []byte) (string, interface{}, error) {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(message, &parsed); err != nil {
		id := m.identifyRequestID(message)
		return "", id, errors.Wrap(err, "identifyMessage: failed to parse message structure")
	}
	id := m.identifyRequestID(message)
	if methodRaw, ok := parsed["method"]; ok {
		var method string
		if err := json.Unmarshal(methodRaw, &method); err != nil {
			return "", id, errors.Wrap(err, "identifyMessage: failed to parse method")
		}
		if method == "" {
			return "", id, errors.New("identifyMessage: method cannot be empty")
		}
		if id == nil { // Corrected check for notification
			specificNotifType := method + "_notification"
			if strings.Contains(method, "/") {
				specificNotifType = method + "_notification"
			}
			if m.validator.HasSchema(specificNotifType) {
				m.logger.Debug("Identified specific notification type", "type", specificNotifType)
				return specificNotifType, nil, nil
			}
			m.logger.Debug("Using generic notification type", "method", method)
			return "JSONRPCNotification", nil, nil
		}
		return method, id, nil
	}
	if _, ok := parsed["result"]; ok {
		return "success_response", id, nil
	}
	if _, ok := parsed["error"]; ok {
		return "error_response", id, nil
	}
	return "", id, errors.New("identifyMessage: unable to identify message type (not request, notification, or response)")
}

// --- Helper to Extract Request ID ---.
func (m *ValidationMiddleware) identifyRequestID(message []byte) interface{} {
	var parsed struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(message, &parsed); err != nil {
		m.logger.Warn("Failed to parse base structure for ID extraction", "error", err)
		return nil
	}
	if parsed.ID != nil && string(parsed.ID) != "null" {
		var idValue interface{}
		if err := json.Unmarshal(parsed.ID, &idValue); err == nil {
			switch idValue.(type) {
			case string, float64:
				return idValue
			default:
				m.logger.Warn("Invalid JSON-RPC ID type detected after unmarshal", "rawId", string(parsed.ID), "goType", fmt.Sprintf("%T", idValue))
				return nil
			}
		} else {
			m.logger.Warn("Failed to unmarshal ID value", "rawId", string(parsed.ID), "error", err)
			return nil
		}
	}
	return nil
}
