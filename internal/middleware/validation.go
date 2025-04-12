// file: internal/middleware/validation.go.
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
	GetLoadDuration() time.Duration    // Keep if needed by logging.
	GetCompileDuration() time.Duration // Keep if needed by logging.
	Shutdown() error                   // Added Shutdown for completeness.
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
// HandleMessage orchestrates the validation and message handling flow.
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
	// errorResponseBytes will be non-nil if validation fails strictly.
	// internalError indicates a problem during validation itself.
	errorResponseBytes, msgType, reqID, internalError := m.validateIncomingMessage(ctx, message, startTime)
	if internalError != nil {
		// Log the internal error, but create a standard response for the client.
		m.logger.Error("Internal error during incoming validation.", "error", internalError)
		// Pass the original error to potentially include sanitized details if the helper function supports it.
		respBytes, creationErr := createInternalErrorResponse(reqID /* removed internalErr */)
		if creationErr != nil {
			return nil, creationErr // Failed to even create the error response.
		}
		return respBytes, nil
	}
	if errorResponseBytes != nil {
		return errorResponseBytes, nil // Return the pre-formatted JSON-RPC error.
	}

	// 2. Prepare context and call Next Handler.
	// Store the identified request method type in context for potential outgoing validation.
	ctxWithMsgType := context.WithValue(ctx, contextKeyRequestMethod, msgType)
	if m.next == nil {
		return nil, errors.New("validation middleware reached end of chain without a final handler")
	}
	responseBytes, nextErr := m.next(ctxWithMsgType, message)

	// 3. Handle Error from Next Handler (if any).
	if nextErr != nil {
		// If the next handler errored, we generally don't validate the error response it *might* have generated.
		// We just pass the error along (or potentially wrap it).
		// The main server loop is responsible for creating the final JSON-RPC error response based on nextErr.
		return nil, nextErr
	}

	// 4. Validate Outgoing Response (if enabled and next handler succeeded).
	if responseBytes != nil && m.options.ValidateOutgoing {
		// Retrieve the original request method from context for accurate schema selection.
		requestMethod, _ := ctxWithMsgType.Value(contextKeyRequestMethod).(string) // Okay if not found, will fallback.

		outgoingValidationErr := m.validateOutgoingResponse(ctx, requestMethod, responseBytes)
		if outgoingValidationErr != nil {
			// If strict outgoing mode is enabled, this validation error should fail the request.
			// We return the error itself, and the main server loop will create the final JSON-RPC error response.
			return nil, errors.Wrap(outgoingValidationErr, "outgoing response validation failed")
		}
		// If not strict or validation passed, continue to return the original responseBytes.
	}

	// 5. Return Successful Response.
	return responseBytes, nil
}

// --- Incoming Validation Logic ---.
// validateIncomingMessage handles the validation steps for an incoming message.
// Returns: (errorResponseBytes []byte, msgType string, reqID interface{}, internalError error).
// - errorResponseBytes: Non-nil pre-formatted JSON-RPC error if validation fails strictly.
// - msgType: The identified message type (method name or generic).
// - reqID: The request ID (can be nil).
// - internalError: An error encountered *during* the validation process itself.
func (m *ValidationMiddleware) validateIncomingMessage(ctx context.Context, message []byte, startTime time.Time) ([]byte, string, interface{}, error) {
	// Basic JSON syntax check.
	if !json.Valid(message) {
		preview := calculatePreview(message)
		m.logger.Warn("Invalid JSON syntax received.", "messagePreview", preview)
		// Attempt to create a standard parse error response.
		respBytes, creationErr := createParseErrorResponse(nil, errors.New("invalid JSON syntax"))
		if creationErr != nil {
			// Return internal error if we can't even create the error response.
			return nil, "", nil, errors.Wrap(creationErr, "failed to create parse error response")
		}
		// Return the formatted error response, no message type/ID applicable.
		return respBytes, "", nil, nil
	}

	// Identify message type and ID.
	msgType, reqID, identifyErr := m.identifyMessage(message)
	if identifyErr != nil {
		preview := calculatePreview(message)
		m.logger.Warn("Failed to identify message type.", "error", identifyErr, "messagePreview", preview)
		// Use the extracted reqID (even if partial/invalid type) when creating the error response.
		respBytes, creationErr := createInvalidRequestErrorResponse(reqID, identifyErr)
		if creationErr != nil {
			return nil, msgType, reqID, errors.Wrap(creationErr, "failed to create invalid request error response")
		}
		// Return the formatted error response, preserving identified type/ID for context.
		return respBytes, msgType, reqID, nil
	}

	// Check if validation should be skipped for this type.
	if m.options.SkipTypes[msgType] {
		m.logger.Debug("Skipping validation for message type.", "type", msgType, "requestID", reqID)
		return nil, msgType, reqID, nil // Proceed without validation.
	}

	// Determine the schema to use for validation.
	schemaType := m.determineIncomingSchemaType(msgType)

	// Perform validation.
	validationErr := m.validator.Validate(ctx, schemaType, message)

	// Log performance if enabled.
	if m.options.MeasurePerformance {
		elapsed := time.Since(startTime)
		m.logger.Debug("Incoming message validation performance.",
			"messageType", msgType,
			"schemaType", schemaType,
			"duration", elapsed,
			"requestID", reqID,
			"isValid", validationErr == nil)
	}

	// Handle validation result.
	if validationErr != nil {
		if m.options.StrictMode {
			m.logger.Warn("Incoming message validation failed (strict mode, rejecting).",
				"messageType", msgType, "requestID", reqID, "error", validationErr)
			respBytes, creationErr := createValidationErrorResponse(reqID, validationErr)
			if creationErr != nil {
				return nil, msgType, reqID, errors.Wrap(creationErr, "failed to create validation error response")
			}
			// Return the formatted error response.
			return respBytes, msgType, reqID, nil
		} else {
			// Non-strict mode: Log the error but allow processing to continue.
			m.logger.Warn("Incoming validation error ignored (non-strict mode).",
				"messageType", msgType, "requestID", reqID, "error", validationErr)
			// Proceed normally.
			return nil, msgType, reqID, nil
		}
	}

	// Validation passed.
	m.logger.Debug("Incoming message passed validation.", "messageType", msgType, "requestID", reqID)
	return nil, msgType, reqID, nil
}

// --- Outgoing Validation Logic ---.
// validateOutgoingResponse handles validation for an outgoing response message.
// It returns an error only if StrictOutgoing is true and validation fails.
func (m *ValidationMiddleware) validateOutgoingResponse(ctx context.Context, requestMethod string, responseBytes []byte) error {
	// Don't validate JSON-RPC error responses we generated ourselves.
	if isErrorResponse(responseBytes) {
		m.logger.Debug("Skipping outgoing validation for JSON-RPC error response.")
		return nil
	}

	// Determine the schema type for the response.
	schemaType := m.determineOutgoingSchemaType(requestMethod, responseBytes)
	if schemaType == "" {
		// This indicates an issue determining the schema, log and potentially skip/error based on policy.
		m.logger.Warn("Could not determine schema type for outgoing validation.", "requestMethod", requestMethod, "responsePreview", calculatePreview(responseBytes))
		if m.options.StrictOutgoing {
			return errors.New("could not determine schema type for outgoing validation")
		}
		return nil // Skip validation if type determination fails in non-strict mode.
	}

	// Perform validation.
	validationErr := m.validator.Validate(ctx, schemaType, responseBytes)

	if validationErr != nil {
		responseMsgType, responseReqID, _ := m.identifyMessage(responseBytes) // Get type/ID for logging context.
		preview := calculatePreview(responseBytes)
		m.logger.Error("Outgoing response validation failed!",
			"requestMethod", requestMethod, // Method of the original request.
			"responseMsgType", responseMsgType, // Type identified from the response itself.
			"responseReqID", responseReqID, // ID from the response.
			"schemaTypeUsed", schemaType,
			"error", validationErr,
			"responsePreview", preview)

		// Perform specific checks if it looks like a tools response.
		if strings.HasPrefix(schemaType, "tools/") && strings.HasSuffix(schemaType, "_response") {
			m.performToolNameValidation(responseBytes) // Log errors internally if names are invalid.
		}

		// If strict outgoing mode is enabled, return the error to fail the request.
		if m.options.StrictOutgoing {
			return validationErr // The caller (HandleMessage) will wrap this.
		}
		// Otherwise (non-strict), log the error but don't prevent the response.
		m.logger.Warn("Outgoing validation error ignored (non-strict outgoing mode).")
	} else {
		m.logger.Debug("Outgoing response passed validation.", "schemaTypeUsed", schemaType)
	}

	return nil // No error to return (either passed or non-strict failure).
}

// --- Helper: Determine Incoming Schema Type ---.
func (m *ValidationMiddleware) determineIncomingSchemaType(msgType string) string {
	// For requests/notifications, use method-specific schemas if available.
	if m.validator.HasSchema(msgType) {
		return msgType
	} else if strings.HasSuffix(msgType, "_notification") {
		// Fallback to generic notification schema.
		if m.validator.HasSchema("notification") {
			return "notification"
		}
	} else {
		// Fallback to generic request schema.
		if m.validator.HasSchema("request") {
			return "request"
		}
	}

	// Last resort: Use the base schema if no specific or generic schema found.
	m.logger.Warn("Specific/generic schema not found for incoming message, using base schema.", "messageType", msgType)
	if m.validator.HasSchema("base") {
		return "base"
	}

	// Should not happen if base schema compiled correctly, but return original type as ultimate fallback.
	return msgType
}

// --- Helper: Determine Outgoing Schema Type ---.
func (m *ValidationMiddleware) determineOutgoingSchemaType(requestMethod string, responseBytes []byte) string {
	// 1. Try deriving from the original request method (most reliable).
	if requestMethod != "" && !strings.HasSuffix(requestMethod, "_notification") {
		// Construct expected response schema name (e.g., "tools/list" -> "tools/list_response").
		expectedResponseSchema := requestMethod + "_response"
		if m.validator.HasSchema(expectedResponseSchema) {
			return expectedResponseSchema
		}
		m.logger.Debug("Specific response schema derived from request method not found, trying fallback.",
			"requestMethod", requestMethod, "derivedSchema", expectedResponseSchema)
	}

	// 2. Fallback: Identify type from the response content itself.
	responseMsgType, _, identifyErr := m.identifyMessage(responseBytes)
	if identifyErr == nil {
		// Use the identified type if a schema exists for it (e.g., "success_response").
		if m.validator.HasSchema(responseMsgType) {
			return responseMsgType
		}
		m.logger.Debug("Schema for type identified from response not found, trying generic.",
			"responseMsgType", responseMsgType)
	} else {
		m.logger.Warn("Failed to identify outgoing response type for schema determination fallback.", "error", identifyErr)
	}

	// 3. Generic Fallbacks (based on simple structure checks).
	if isSuccessResponse(responseBytes) && m.validator.HasSchema("success_response") {
		return "success_response"
	}
	// Note: isErrorResponse check happens earlier in validateOutgoingResponse.

	// 4. Last Resort: Base schema.
	m.logger.Warn("Specific/generic schema not found for outgoing response, using base schema.",
		"requestMethod", requestMethod, "responsePreview", calculatePreview(responseBytes))
	if m.validator.HasSchema("base") {
		return "base"
	}

	// Indicate failure to determine type.
	return ""
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

	// Attempt to parse; log debug message if parsing fails, don't halt validation.
	if err := json.Unmarshal(responseBytes, &toolsResp); err != nil {
		m.logger.Debug("Could not parse response as tool list for name validation.", "error", err, "responsePreview", calculatePreview(responseBytes))
		return
	}

	// Validate names if parsing succeeded.
	for i, tool := range toolsResp.Result.Tools {
		if err := schema.ValidateName(schema.EntityTypeTool, tool.Name); err != nil {
			// Log invalid names as errors, as this violates MCP constraints.
			m.logger.Error("Invalid tool name found in outgoing response.",
				"toolIndex", i,
				"invalidName", tool.Name,
				"validationError", err,
				"rulesHint", schema.GetNamePatternDescription(schema.EntityTypeTool))
			// Note: We log but don't fail the request here unless StrictOutgoing is on.
			// (which is handled by the main schema validation returning an error).
		}
	}
}

// --- Helper: Identify Message --- Refactored for lower complexity. ---

// parseAndValidateID extracts and validates the ID field from a parsed message.
// Returns the parsed ID, raw ID bytes (only if parsing fails), and error if invalid.
// Note: The isValid flag is removed as error presence indicates invalidity.
func (m *ValidationMiddleware) parseAndValidateID(parsed map[string]json.RawMessage) (parsedID interface{}, rawID json.RawMessage, err error) {
	idRaw, idExists := parsed["id"]
	if !idExists || string(idRaw) == "null" {
		return nil, nil, nil // No ID or null ID is valid.
	}

	// Unmarshal the ID.
	if err := json.Unmarshal(idRaw, &parsedID); err != nil {
		// Malformed ID JSON.
		// Return raw bytes as parsedID is likely garbage.
		return string(idRaw), idRaw, errors.Wrap(err, "identifyMessage: failed to parse id")
	}

	// Check the Go type after unmarshalling.
	switch parsedID.(type) {
	case string, float64, json.Number:
		return parsedID, idRaw, nil // Valid type.
	default:
		// Invalid type (e.g., object, array). Return the parsed value along with the error.
		return parsedID, idRaw, errors.New(fmt.Sprintf("identifyMessage: invalid type for id: expected string, number, or null, got %T", parsedID))
	}
}

// identifyMessage extracts the message type and request ID from a JSON-RPC message.
// Returns message type (method name or response type), request ID (if present), and error.
func (m *ValidationMiddleware) identifyMessage(message []byte) (string, interface{}, error) {
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(message, &parsed); err != nil {
		return "", nil, errors.Wrap(err, "identifyMessage: failed to parse message")
	}

	// Parse and validate the ID first.
	id, _, idErr := m.parseAndValidateID(parsed) // Use the new helper.
	if idErr != nil {
		// Return the specific ID parsing/validation error.
		return "", id, idErr // Return parsed ID (even if invalid type) for context in error response.
	}
	// If idErr is nil, the ID is considered valid (null, string, or number).

	// Check for method (Request or Notification).
	if methodRaw, ok := parsed["method"]; ok {
		var method string
		if err := json.Unmarshal(methodRaw, &method); err != nil {
			return "", id, errors.Wrap(err, "identifyMessage: failed to parse method")
		}
		if id == nil {
			return method + "_notification", nil, nil
		}
		return method, id, nil
	}

	// Check for result (Success Response).
	if _, ok := parsed["result"]; ok {
		// ID must have existed and been valid for a response.
		// idErr check handles invalid types, absence handled by logic flow.
		return "success_response", id, nil
	}

	// Check for error (Error Response).
	if _, ok := parsed["error"]; ok {
		// ID must have existed and been valid for a response.
		return "error_response", id, nil
	}

	// If no method/result/error found.
	return "", id, errors.New("identifyMessage: unable to identify message type (not request, notification, or response)")
}

// --- Helper: Check Response Types ---.
func isErrorResponse(message []byte) bool {
	// Quick check without full parsing. Robustness depends on JSON structure.
	// Assumes `"error": { ... }` structure for errors.
	return bytes.Contains(message, []byte(`"error":`)) && !bytes.Contains(message, []byte(`"result":`))
}

func isSuccessResponse(message []byte) bool {
	// Quick check without full parsing. Assumes `"result": ...` for success.
	return bytes.Contains(message, []byte(`"result":`)) && !bytes.Contains(message, []byte(`"error":`))
}

// --- Helper: Calculate Preview ---.
func calculatePreview(data []byte) string {
	const maxPreviewLen = 100
	if len(data) > maxPreviewLen {
		return string(data[:maxPreviewLen]) + "..." // Indicate truncation.
	}
	return string(data)
}

// --- Helper Functions to Create JSON-RPC Error Responses ---.

func createParseErrorResponse(id interface{}, parseErr error) ([]byte, error) {
	return createGenericErrorResponse(id, transport.JSONRPCParseError, "Parse error.", parseErr)
}

// createInvalidRequestErrorResponse adjusts data based on error content.
func createInvalidRequestErrorResponse(id interface{}, requestErr error) ([]byte, error) {
	errMsg := requestErr.Error()
	data := map[string]interface{}{"details": errMsg} // Default data.
	if strings.Contains(errMsg, "invalid type for id") {
		data["reason"] = "Invalid JSON-RPC ID type"
		// Potentially include the problematic ID value if safe
		// data["receivedId"] = id
	} else if strings.Contains(errMsg, "failed to parse id") {
		data["reason"] = "Malformed JSON in ID field"
	} else if strings.Contains(errMsg, "unable to identify message type") {
		data["reason"] = "Message structure doesn't match request, notification, or response"
	}
	// Add more specific reasons based on identifyMessage errors if needed.
	return createGenericErrorResponseWithData(id, transport.JSONRPCInvalidRequest, "Invalid Request.", data)
}

func createValidationErrorResponse(id interface{}, validationErr error) ([]byte, error) {
	code := transport.JSONRPCInvalidRequest // Default code.
	message := "Invalid Request."
	var errorData interface{} = map[string]interface{}{"details": validationErr.Error()} // Default data.

	var schemaValErr *schema.ValidationError
	if errors.As(validationErr, &schemaValErr) {
		// Add specific validation context.
		errorData = map[string]interface{}{
			"details":       schemaValErr.Message, // Use the core message.
			"instancePath":  schemaValErr.InstancePath,
			"schemaPath":    schemaValErr.SchemaPath,
			"originalError": validationErr.Error(), // Include full formatted error string.
		}
		// Determine if it's an invalid params error based on path.
		if schemaValErr.InstancePath != "" && (strings.Contains(schemaValErr.InstancePath, "/params") || strings.HasPrefix(schemaValErr.InstancePath, "params")) {
			code = transport.JSONRPCInvalidParams // -32602.
			message = "Invalid params."
		}
	}

	// Use the generic creator with potentially updated code/message/data.
	return createGenericErrorResponseWithData(id, code, message, errorData)
}

// createInternalErrorResponse is used for errors during processing, *not* validation errors.
// Removed unused internalErr parameter.
func createInternalErrorResponse(id interface{} /* removed internalErr error */) ([]byte, error) {
	// Avoid exposing internal error details by default.
	data := map[string]interface{}{"details": "An internal server error occurred."}
	// Log the actual internalErr server-side (done by the caller).
	return createGenericErrorResponseWithData(id, transport.JSONRPCInternalError, "Internal error.", data)
}

// createGenericErrorResponse creates a standard JSON-RPC error response.
func createGenericErrorResponse(id interface{}, code int, message string, cause error) ([]byte, error) {
	var data interface{}
	if cause != nil {
		// Include basic cause in data, but be careful not to leak sensitive info.
		data = map[string]interface{}{"details": cause.Error()}
	}
	return createGenericErrorResponseWithData(id, code, message, data)
}

// createGenericErrorResponseWithData creates a standard JSON-RPC error response with provided data.
func createGenericErrorResponseWithData(id interface{}, code int, message string, data interface{}) ([]byte, error) {
	if id == nil {
		id = json.RawMessage("null") // Ensure ID is json null if nil.
	}
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	// Only include data field if it's not nil.
	if data != nil {
		response["error"].(map[string]interface{})["data"] = data
	}
	return json.Marshal(response)
}
