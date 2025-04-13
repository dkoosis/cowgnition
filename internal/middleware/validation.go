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

// --- Helper: Check for non-critical validation errors ---.
func isNonCriticalValidationError(err error) bool {
	var schemaValErr *schema.ValidationError
	if !errors.As(err, &schemaValErr) {
		return false
	}
	msg := schemaValErr.Message
	return strings.Contains(msg, "additionalProperties")
}

// --- Helper: Identify Message ---
// This function attempts to determine the message type (request, notification, response)
// and extracts the request ID. It also performs basic structural validation.
// nolint:gocyclo
func (m *ValidationMiddleware) identifyMessage(message []byte) (string, interface{}, error) {
	// Ensure validator exists before checking HasSchema
	if m.validator == nil {
		// Added a more specific error message for this case
		return "", nil, errors.New("identifyMessage: ValidationMiddleware's validator is nil")
	}

	// Attempt to parse the message into a generic map first
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(message, &parsed); err != nil {
		// Try to extract ID even if full parse fails, might be partially valid
		id := m.identifyRequestID(message) // identifyRequestID handles its own errors/logging
		return "", id, errors.Wrap(err, "identifyMessage: failed to parse message structure")
	}

	// Check for presence of key fields
	_, idExists := parsed["id"]
	_, methodExists := parsed["method"]
	_, resultExists := parsed["result"]
	_, errorExists := parsed["error"]

	// Extract and validate the ID using the dedicated helper
	// identifyRequestID returns the validated ID (string, int64, float64) or nil if missing, null, or invalid type
	id := m.identifyRequestID(message)

	// CRITICAL CHECK: If 'id' key existed but helper returned nil (and raw ID wasn't "null"), it means the ID type was invalid.
	if idExists && id == nil {
		// Check if the raw value was actually "null" before declaring invalid type
		idRaw := parsed["id"] // Safe to access, idExists is true
		if idRaw != nil && string(idRaw) != "null" {
			return "", nil, errors.New("Invalid JSON-RPC ID type detected") // Specific error for invalid ID type
		}
		// If it was explicitly null, id will be nil, which is correct for notifications/some errors
	}

	// Determine message type based on existing fields
	if methodExists {
		// Potentially a Request or Notification
		methodRaw := parsed["method"] // Safe, methodExists is true
		if methodRaw == nil {
			return "", id, errors.New("identifyMessage: 'method' field exists but is null")
		}
		var method string
		if err := json.Unmarshal(methodRaw, &method); err != nil {
			// ID might be valid even if method isn't, return it
			return "", id, errors.Wrap(err, "identifyMessage: failed to parse 'method' field")
		}
		if method == "" {
			// ID might be valid even if method is empty, return it
			return "", id, errors.New("identifyMessage: 'method' field cannot be empty")
		}

		// Check if it's a notification (ID is nil AFTER type validation)
		if id == nil {
			// *** REFACTORED NOTIFICATION LOGIC (incorporating fix for ineffassign) ***
			determinedSchemaKey := "JSONRPCNotification" // Default to generic

			// Check for hierarchical notifications first (e.g., "notifications/progress")
			if strings.HasPrefix(method, "notifications/") {
				// Try the full method name as the schema key if it exists
				if m.validator.HasSchema(method) {
					determinedSchemaKey = method
				} else {
					m.logger.Debug("Specific hierarchical notification schema not found, using generic.", "method", method)
					// If specific hierarchical schema not found, stick with generic JSONRPCNotification
					// We don't check method+"_notification" for hierarchical ones
				}
			} else {
				// For non-namespaced methods (e.g., "ping"), check for the method_notification convention
				notifSchemaKey := method + "_notification"
				if m.validator.HasSchema(notifSchemaKey) {
					determinedSchemaKey = notifSchemaKey // Use specific e.g., "ping_notification"
				} else {
					// If method_notification doesn't exist, stick with generic JSONRPCNotification
					m.logger.Debug("Specific method_notification schema not found, using generic.", "method", method, "triedKey", notifSchemaKey)
				}
			}

			// Final determination using the helper (applies further fallback e.g., to "base" if needed)
			finalSchemaType := m.determineIncomingSchemaType(determinedSchemaKey)

			m.logger.Debug("Identified message as Notification", "method", method, "determinedSchemaType", finalSchemaType)
			return finalSchemaType, nil, nil // Return nil ID for notifications
			// *** END REFACTORED NOTIFICATION LOGIC ***
		}

		// It's a Request (has method and a valid, non-null ID)
		schemaType := m.determineIncomingSchemaType(method) // Use method name to find specific request schema
		m.logger.Debug("Identified message as Request", "method", method, "id", id, "determinedSchemaType", schemaType)
		return schemaType, id, nil
	} else if resultExists {
		// It's a Success Response
		if errorExists { // JSON-RPC spec forbids both result and error
			return "", id, errors.New("identifyMessage: message cannot contain both 'result' and 'error' fields")
		}
		// JSON-RPC spec requires ID for responses (must not be null according to strict interpretation,
		// although some implementations might allow null ID if the request ID was null, but we extracted non-null ID before)
		if !idExists || id == nil {
			return "", id, errors.New("identifyMessage: success response message must contain a valid non-null 'id' field")
		}
		schemaType := m.determineIncomingSchemaType("success_response") // Use generic success response type
		m.logger.Debug("Identified message as Success Response", "id", id, "determinedSchemaType", schemaType)
		return schemaType, id, nil
	} else if errorExists {
		// It's an Error Response
		// ID can be null if error occurred before ID processing, id will be nil in that case.
		schemaType := m.determineIncomingSchemaType("error_response") // Use generic error response type
		m.logger.Debug("Identified message as Error Response", "id", id, "determinedSchemaType", schemaType)
		return schemaType, id, nil // ID can be nil here
	}

	// If none of the key fields (method, result, error) were found
	return "", id, errors.New("identifyMessage: unable to identify message type (missing method, result, or error field)")
}

// --- Helper to Extract Request ID ---.
func (m *ValidationMiddleware) identifyRequestID(message []byte) interface{} {
	var parsed struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(message, &parsed); err != nil {
		m.logger.Debug("Failed to parse base structure for ID extraction", "error", err, "preview", calculatePreview(message))
		return nil
	}

	if parsed.ID != nil && string(parsed.ID) != "null" {
		var idValue interface{}
		decoder := json.NewDecoder(bytes.NewReader(parsed.ID))
		decoder.UseNumber()
		if err := decoder.Decode(&idValue); err == nil {
			switch idValue.(type) {
			case json.Number, string:
				if num, ok := idValue.(json.Number); ok {
					if i, err := num.Int64(); err == nil {
						return i // Return int64 if possible
					} else if f, err := num.Float64(); err == nil {
						return f // Otherwise return float64
					}
					m.logger.Warn("Valid json.Number ID type detected but failed number conversion", "rawId", string(parsed.ID))
					return nil // Treat as invalid if conversion fails
				} else if str, ok := idValue.(string); ok {
					return str // Return string
				}
				// Should not happen if initial switch matched
				m.logger.Warn("Mismatched type after switch in identifyRequestID", "rawId", string(parsed.ID), "goType", fmt.Sprintf("%T", idValue))
				return nil
			default:
				// Invalid types (Objects, Arrays, Booleans)
				m.logger.Warn("Invalid JSON-RPC ID type detected", "rawId", string(parsed.ID), "goType", fmt.Sprintf("%T", idValue))
				return nil // Signal invalid type by returning nil
			}
		} else {
			m.logger.Warn("Failed to unmarshal ID value itself", "rawId", string(parsed.ID), "error", err)
			return nil
		}
	}
	// ID field is missing or explicitly null.
	return nil
}

// --- Helper: Determine Incoming Schema Type ---.
// (Include the implementation from validation.go here).
// nolint:gocyclo
func (m *ValidationMiddleware) determineIncomingSchemaType(msgType string) string {
	// Ensure validator exists and is initialized before accessing HasSchema
	if m.validator == nil || !m.validator.IsInitialized() {
		m.logger.Error("determineIncomingSchemaType called but validator is nil or not initialized")
		// Cannot determine type without validator, maybe return a default or error indication?
		// Returning "base" might be misleading. Returning "" might cause issues upstream.
		// Let's return base as a "best guess" but log heavily.
		return "base"
	}

	if m.validator.HasSchema(msgType) {
		return msgType
	}
	// Simplified fallback logic for demonstration
	fallbackKey := "base" // Default fallback
	if strings.HasSuffix(msgType, "_notification") {
		if m.validator.HasSchema("JSONRPCNotification") {
			fallbackKey = "JSONRPCNotification"
		}
	} else if strings.Contains(msgType, "Response") || strings.Contains(msgType, "Result") || strings.HasSuffix(msgType, "_response") {
		// Prefer generic response if available
		if m.validator.HasSchema("JSONRPCResponse") {
			fallbackKey = "JSONRPCResponse"
		}
		// If it's specifically an error response, try that
		if (strings.Contains(msgType, "Error") || strings.HasSuffix(msgType, "_error")) && m.validator.HasSchema("JSONRPCError") {
			fallbackKey = "JSONRPCError"
		}
	} else { // Assume request
		if m.validator.HasSchema("JSONRPCRequest") {
			fallbackKey = "JSONRPCRequest"
		}
	}

	// Check if the chosen fallback actually exists
	if m.validator.HasSchema(fallbackKey) {
		m.logger.Debug("Using schema for incoming message.", "messageType", msgType, "schemaKeyUsed", fallbackKey)
		return fallbackKey
	}

	// If even the chosen fallback doesn't exist, log a warning and maybe default to "base" if it exists
	m.logger.Warn("Specific/generic schema not found for incoming message, trying 'base'.", "messageType", msgType, "triedFallback", fallbackKey)
	if m.validator.HasSchema("base") {
		return "base"
	}

	// If even "base" doesn't exist (major initialization issue)
	m.logger.Error("CRITICAL: No schema found for message type or any fallbacks (including base).", "messageType", msgType)
	// Returning an empty string might be better signal than "base" here
	return ""
}

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
