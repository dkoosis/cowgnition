// Package middleware provides chainable handlers for processing MCP messages, like validation.
package middleware

// file: internal/middleware/validation.go

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/schema" // Ensure schema is imported
	"github.com/dkoosis/cowgnition/internal/transport"
)

// SchemaValidatorInterface defines the methods needed for schema validation.
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
	StrictMode         bool
	MeasurePerformance bool
	ValidateOutgoing   bool
	StrictOutgoing     bool
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
		SkipTypes:          map[string]bool{"ping": true}, // ping is often skipped as a basic health check
		StrictMode:         true,                          // Fail on validation errors by default
		MeasurePerformance: false,                         // Performance timing disabled by default
		ValidateOutgoing:   true,                          // Validate server responses by default
		StrictOutgoing:     false,                         // Don't fail requests due to outgoing validation errors by default
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
		// Forward the error; allow central error handling to create response if necessary
		return nil, nextErr
	}

	// 4. Validate Outgoing Response (if enabled and next handler succeeded).
	// Note: handleOutgoingValidation returns response bytes *only* on strict failure.
	outgoingErrorResponseBytes, outgoingErr := m.handleOutgoingValidation(ctxWithMsgType, responseBytes, reqID)
	if outgoingErr != nil {
		// This indicates a critical internal error during outgoing validation (e.g., marshalling the error response)
		m.logger.Error("Internal error during outgoing validation handling.", "error", fmt.Sprintf("%+v", outgoingErr))
		respBytes, creationErr := createInternalErrorResponse(reqID)
		if creationErr != nil {
			return nil, errors.Wrap(creationErr, "critical: failed to create internal error response for outgoing validation failure")
		}
		return respBytes, nil // Return internal error response bytes
	}
	if outgoingErrorResponseBytes != nil {
		// Validation failed in strict mode, return the generated error response
		return outgoingErrorResponseBytes, nil
	}

	// 5. Return Successful (and potentially validated) Response from next handler.
	return responseBytes, nil
}

// handleOutgoingValidation encapsulates the logic for validating outgoing responses.
// It returns the bytes for an error response if validation fails *and* StrictOutgoing is true,
// otherwise it returns nil, nil (indicating the original response should be used or an internal error occurred).
// file: internal/middleware/validation.go.

// handleOutgoingValidation encapsulates the logic for validating outgoing responses.
// It returns the bytes for an error response if validation fails *and* StrictOutgoing is true,
// otherwise it returns nil, nil (indicating the original response should be used).
func (m *ValidationMiddleware) handleOutgoingValidation(ctx context.Context, responseBytes []byte, requestID interface{}) ([]byte, error) {
	// If no response or outgoing validation is disabled, do nothing.
	if responseBytes == nil || !m.options.ValidateOutgoing {
		return nil, nil
	}

	requestMethod, _ := ctx.Value(contextKeyRequestMethod).(string)

	// Perform the actual validation of the outgoing response bytes.
	outgoingValidationErr := m.validateOutgoingResponse(ctx, requestMethod, responseBytes)

	// If validation passed, return nil, nil to signal using original responseBytes.
	if outgoingValidationErr == nil {
		return nil, nil
	}

	// --- Validation Failed ---

	// If NOT in strict outgoing mode, log the error but allow the original response through.
	if !m.options.StrictOutgoing {
		m.logger.Warn("Outgoing validation error ignored (non-strict outgoing mode).",
			"requestMethod", requestMethod,
			"requestID", requestID,
			"error", outgoingValidationErr) // Log the actual validation error detail.
		// Return nil, nil to indicate the original responseBytes should be sent.
		return nil, nil
	}

	// --- Strict Outgoing Mode: Validation Failed ---
	// Log the failure as an error because we are replacing the response.
	m.logger.Error("Outgoing response validation failed (strict outgoing mode)!",
		"requestMethod", requestMethod,
		"requestID", requestID,
		"error", fmt.Sprintf("%+v", outgoingValidationErr)) // Log with stack trace if available.

	// Create a formatted JSON-RPC error response based on the validation failure.
	rpcErrorBytes, creationErr := createValidationErrorResponse(requestID, outgoingValidationErr)
	if creationErr != nil {
		// If we can't even create the validation error response, log critical and create internal error.
		m.logger.Error("CRITICAL: Failed to create validation error response for outgoing failure.",
			"creationError", fmt.Sprintf("%+v", creationErr),
			"originalValidationError", fmt.Sprintf("%+v", outgoingValidationErr))

		// Attempt to create a generic internal error response.
		internalErrResp, creationErr2 := createInternalErrorResponse(requestID)
		if creationErr2 != nil {
			// If even *that* fails, return the marshalling error directly. Critical failure.
			// Wrap it for context.
			return nil, errors.Wrap(creationErr2, "critical: failed marshalling even internal error response")
		}
		// Return the generic internal error response bytes.
		return internalErrResp, nil
	}

	// Return the created validation error response bytes. The 'error' return is nil because
	// we successfully created the error *response* to send back.
	return rpcErrorBytes, nil
}

// validateIncomingMessage handles the validation logic for incoming messages.
// Returns: (error response bytes OR nil), msgType, reqID, (internal processing error OR nil).
func (m *ValidationMiddleware) validateIncomingMessage(ctx context.Context, message []byte, startTime time.Time) ([]byte, string, interface{}, error) {
	// Check JSON syntax first.
	if !json.Valid(message) {
		preview := calculatePreview(message)
		m.logger.Warn("Invalid JSON syntax received.", "messagePreview", preview)
		respBytes, creationErr := createParseErrorResponse(nil, errors.New("invalid JSON syntax")) // ID is nil for parse error
		if creationErr != nil {
			// Internal error creating the response
			return nil, "", nil, errors.Wrap(creationErr, "failed to create parse error response")
		}
		// Return the error response bytes, no further processing needed
		return respBytes, "", nil, nil
	}

	// Identify message type and ID.
	msgType, reqID, identifyErr := m.identifyMessage(message)
	if identifyErr != nil {
		preview := calculatePreview(message)
		m.logger.Warn("Failed to identify message type.", "error", identifyErr, "messagePreview", preview)
		// Use the reqID extracted *during* identification attempt, even if identification failed overall
		respBytes, creationErr := createInvalidRequestErrorResponse(reqID, identifyErr)
		if creationErr != nil {
			// Internal error creating the response
			return nil, msgType, reqID, errors.Wrap(creationErr, "failed to create invalid request error response")
		}
		// Return the error response bytes
		return respBytes, msgType, reqID, nil
	}

	// Check if validation should be skipped for this type.
	if m.options.SkipTypes[msgType] {
		m.logger.Debug("Skipping validation for message type.", "type", msgType, "requestID", reqID)
		// Return nils indicating successful skip, proceed to next handler
		return nil, msgType, reqID, nil
	}

	// Determine the actual schema definition to use (might involve fallbacks).
	schemaType := m.determineIncomingSchemaType(msgType)
	// Perform validation against the schema.
	validationErr := m.validator.Validate(ctx, schemaType, message)

	// Log performance if enabled.
	if m.options.MeasurePerformance {
		elapsed := time.Since(startTime)
		m.logger.Debug("Incoming message validation performance.",
			"messageType", msgType, "schemaType", schemaType, "duration", elapsed, "requestID", reqID, "isValid", validationErr == nil)
	}

	// Handle validation failure.
	if validationErr != nil {
		// Corrected: Apply indent-error-flow fix from revive
		if !m.options.StrictMode {
			// Non-strict mode: Log the error but proceed.
			m.logger.Warn("Incoming validation error ignored (non-strict mode).",
				"messageType", msgType, "requestID", reqID, "error", validationErr)
			// Return nils to proceed normally
			return nil, msgType, reqID, nil
		}

		// Strict mode: Generate and return error response.
		m.logger.Warn("Incoming message validation failed (strict mode).",
			"messageType", msgType, "requestID", reqID, "error", fmt.Sprintf("%+v", validationErr))
		respBytes, creationErr := createValidationErrorResponse(reqID, validationErr)
		if creationErr != nil {
			// Internal error creating the response
			return nil, msgType, reqID, errors.Wrap(creationErr, "failed to create validation error response")
		}
		// Return the error response bytes
		return respBytes, msgType, reqID, nil
	}

	// Validation passed.
	m.logger.Debug("Incoming message passed validation.", "messageType", msgType, "requestID", reqID)
	// Return nils indicating success, proceed to next handler
	return nil, msgType, reqID, nil
}

// validateOutgoingResponse performs validation on the outgoing response bytes.
func (m *ValidationMiddleware) validateOutgoingResponse(ctx context.Context, requestMethod string, responseBytes []byte) error {
	// Skip validation for standard JSON-RPC error responses.
	if isErrorResponse(responseBytes) {
		m.logger.Debug("Skipping outgoing validation for JSON-RPC error response.")
		return nil
	}

	// Determine the schema type based on the original request method or response content.
	schemaType := m.determineOutgoingSchemaType(requestMethod, responseBytes)
	if schemaType == "" {
		m.logger.Warn("Could not determine schema type for outgoing validation.",
			"requestMethod", requestMethod,
			"responsePreview", calculatePreview(responseBytes))
		// In non-strict outgoing mode, we ignore this. In strict, we fail.
		if m.options.StrictOutgoing {
			// Corrected: Use schema.ErrSchemaNotFound
			// Return a generic internal error? Or a validation error? Let's use validation.
			return NewValidationError(
				schema.ErrSchemaNotFound, // Use appropriate code from schema package
				"Could not determine schema type for outgoing validation",
				nil,
			).WithContext("requestMethod", requestMethod)
		}
		// Non-strict, ignore the inability to find a schema.
		return nil
	}

	// Perform validation.
	validationErr := m.validator.Validate(ctx, schemaType, responseBytes)

	if validationErr != nil {
		responseMsgType, responseReqID, _ := m.identifyMessage(responseBytes) // Identify for logging context
		preview := calculatePreview(responseBytes)

		// Perform specific checks for known outgoing types if validation fails
		if strings.HasPrefix(schemaType, "tools/") && strings.HasSuffix(schemaType, "_response") {
			m.performToolNameValidation(responseBytes) // Log specific tool name errors if applicable
		}

		// Log the primary validation failure details
		m.logger.Debug("Outgoing response validation failed.",
			"requestMethod", requestMethod, // Method of the original request
			"responseMsgType", responseMsgType, // Type identified from the response itself
			"responseReqID", responseReqID, // ID from the response itself
			"schemaTypeUsed", schemaType, // Schema we tried to validate against
			"errorDetail", validationErr, // The actual validation error
			"responsePreview", preview)

		// Return the validation error to be handled by the caller (handleOutgoingValidation)
		return validationErr
	}

	// Validation passed.
	m.logger.Debug("Outgoing response passed validation.", "schemaTypeUsed", schemaType)
	return nil
}

// performToolNameValidation checks tool names within a `tools/list` response.
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
			// NOTE: This currently only logs. In very strict modes, this could potentially
			// trigger an error response, but standard MCP doesn't mandate name format.
		}
	}
}

// determineOutgoingSchemaType heuristics to find the schema for an outgoing response.
func (m *ValidationMiddleware) determineOutgoingSchemaType(requestMethod string, responseBytes []byte) string {
	// 1. Try specific response schema based on request method
	if requestMethod != "" && !strings.HasSuffix(requestMethod, "_notification") {
		// Construct expected response type, e.g., "tools/list" -> "tools/list_response"
		// Handle potential variations if needed (e.g., methods with slashes)
		parts := strings.Split(requestMethod, "/")
		expectedResponseSchema := ""
		if len(parts) > 1 {
			// Simple heuristic: assume last part is method name if slash present
			methodName := parts[len(parts)-1]
			basePath := strings.Join(parts[:len(parts)-1], "/")
			expectedResponseSchema = basePath + "/" + methodName + "_response" // e.g. tools/list_response
		} else {
			expectedResponseSchema = requestMethod + "_response" // e.g. initialize_response
		}

		if m.validator.HasSchema(expectedResponseSchema) {
			m.logger.Debug("Using specific response schema derived from request.", "requestMethod", requestMethod, "schemaKeyUsed", expectedResponseSchema)
			return expectedResponseSchema
		}
		m.logger.Debug("Specific response schema derived from request method not found, trying fallback.",
			"requestMethod", requestMethod, "derivedSchema", expectedResponseSchema)
	}

	// 2. Identify the response type structurally
	responseMsgType, _, identifyErr := m.identifyMessage(responseBytes) // Identify based on result/error fields
	if identifyErr == nil {
		// Check if the structurally identified type has a specific schema
		if m.validator.HasSchema(responseMsgType) {
			m.logger.Debug("Using schema based on identified response type.", "responseMsgType", responseMsgType, "schemaKeyUsed", responseMsgType)
			return responseMsgType
		}

		// Specific heuristic for CallToolResult shape if identified as generic success
		if responseMsgType == "success_response" && m.validator.HasSchema("CallToolResult") && isCallToolResultShape(responseBytes) {
			m.logger.Debug("Identified response as CallToolResult shape, using CallToolResult schema.")
			return "CallToolResult" // Use the specific schema if shape matches
		}
		m.logger.Debug("Schema for type identified from response not found, trying generic.",
			"responseMsgType", responseMsgType)
	} else {
		m.logger.Warn("Failed to identify outgoing response type for schema determination fallback.", "error", identifyErr)
	}

	// 3. Fallback to generic JSON-RPC response schema
	if isSuccessResponse(responseBytes) && m.validator.HasSchema("JSONRPCResponse") {
		m.logger.Debug("Using generic JSONRPCResponse schema as fallback.", "requestMethod", requestMethod)
		return "JSONRPCResponse"
	}
	// Note: isErrorResponse is skipped earlier in validateOutgoingResponse

	// 4. Final fallback to base schema
	m.logger.Warn("Specific/generic schema not found for outgoing response, trying base schema.",
		"requestMethod", requestMethod, "responsePreview", calculatePreview(responseBytes))
	if m.validator.HasSchema("base") {
		return "base"
	}

	// 5. No schema found
	return ""
}

// isErrorResponse checks if a message appears to be a JSON-RPC error response.
func isErrorResponse(message []byte) bool {
	// Simple check for presence of "error" key at the top level, absence of "result".
	// A more robust check would parse the JSON.
	return bytes.Contains(message, []byte(`"error":`)) && !bytes.Contains(message, []byte(`"result":`))
}

// isSuccessResponse checks if a message appears to be a JSON-RPC success response.
func isSuccessResponse(message []byte) bool {
	// Simple check for presence of "result" key at the top level, absence of "error".
	return bytes.Contains(message, []byte(`"result":`)) && !bytes.Contains(message, []byte(`"error":`))
}

// isCallToolResultShape checks heuristically if a response looks like a CallToolResult.
func isCallToolResultShape(message []byte) bool {
	// Checks for `"result":` and `"content":` within the message.
	// Assumes "content" is a distinguishing feature of CallToolResult payload.
	return bytes.Contains(message, []byte(`"result":`)) && bytes.Contains(message, []byte(`"content":`))
}

// isNonCriticalValidationError checks if a validation error might be considered non-critical
// (e.g., allowing extra properties) in non-strict mode.
// nolint:unused // Kept for potential future use or reference
func isNonCriticalValidationError(err error) bool {
	var schemaValErr *schema.ValidationError
	if !errors.As(err, &schemaValErr) {
		return false // Not a schema validation error
	}
	// Example: Consider errors about additional properties non-critical
	return strings.Contains(schemaValErr.Message, "additionalProperties") ||
		strings.Contains(schemaValErr.Message, "unevaluatedProperties")
}

// identifyMessage attempts to determine the message type (request, notification, response)
// and extracts the request ID. It also performs basic structural validation.
// nolint:gocyclo // Function complexity is high due to handling different JSON-RPC structures
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
		if idRaw := parsed["id"]; idRaw != nil && string(idRaw) != "null" {
			return "", nil, errors.New("Invalid JSON-RPC ID type detected") // Specific error for invalid ID type
		}
		// If it was explicitly null, id will be nil, which is correct for notifications/some errors
	}

	// Determine message type based on existing fields
	if methodExists {
		// Potentially a Request or Notification
		methodRaw, ok := parsed["method"] // Safe, methodExists is true
		if !ok || methodRaw == nil {      // Check if key exists AND is not null
			return "", id, errors.New("identifyMessage: 'method' field exists but is null or missing raw value")
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
			// Determine schema key: check specific notification schemas first
			determinedSchemaKey := ""
			if strings.HasPrefix(method, "notifications/") {
				// For hierarchical, try full name first
				if m.validator.HasSchema(method) {
					determinedSchemaKey = method
				}
			} else {
				// For non-hierarchical, try "method_notification"
				notifSchemaKey := method + "_notification"
				if m.validator.HasSchema(notifSchemaKey) {
					determinedSchemaKey = notifSchemaKey
				}
			}
			// If specific not found, use generic fallback logic
			if determinedSchemaKey == "" {
				determinedSchemaKey = "notification" // Generic fallback name
			}
			// Final determination applies further fallback (e.g., to "base") if needed
			finalSchemaType := m.determineIncomingSchemaType(determinedSchemaKey)
			m.logger.Debug("Identified message as Notification", "method", method, "determinedSchemaType", finalSchemaType)
			return finalSchemaType, nil, nil // Return nil ID for notifications
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
		// JSON-RPC spec requires ID for responses
		if !idExists { // Allow null ID here as per relaxed interpretation for responses matching null request IDs
			// Correction: Even for success, ID field must exist. It can be null, but must be present.
			return "", id, errors.New("identifyMessage: success response message must contain an 'id' field")
		}
		schemaType := m.determineIncomingSchemaType("success_response") // Use generic success response type
		m.logger.Debug("Identified message as Success Response", "id", id, "determinedSchemaType", schemaType)
		return schemaType, id, nil
	} else if errorExists {
		// It's an Error Response
		// ID can be null if error occurred before ID processing, or if request ID was null.
		// ID field might be missing if request was unparseable/invalid before ID extraction
		if !idExists {
			m.logger.Warn("Received error response without an 'id' field.")
			// Technically allowed if request ID couldn't be determined, proceed but use nil ID
		}
		schemaType := m.determineIncomingSchemaType("error_response") // Use generic error response type
		m.logger.Debug("Identified message as Error Response", "id", id, "determinedSchemaType", schemaType)
		return schemaType, id, nil // ID can be nil here
	}

	// If none of the key fields (method, result, error) were found
	return "", id, errors.New("identifyMessage: unable to identify message type (missing method, result, or error field)")
}

// identifyRequestID attempts to extract and validate the JSON-RPC ID.
// Returns string, int64, float64, or nil. Nil indicates missing, null, or invalid type.
func (m *ValidationMiddleware) identifyRequestID(message []byte) interface{} {
	var parsed struct {
		ID json.RawMessage `json:"id"`
	}
	// Use decoder to preserve number types if possible
	decoder := json.NewDecoder(bytes.NewReader(message))
	decoder.UseNumber() // Important: read numbers as json.Number
	if err := decoder.Decode(&parsed); err != nil {
		m.logger.Debug("Failed to parse base structure for ID extraction", "error", err, "preview", calculatePreview(message))
		return nil // Cannot parse structure
	}

	if parsed.ID == nil {
		return nil // ID field is missing
	}

	// Try unmarshalling the ID field specifically
	var idValue interface{}
	idDecoder := json.NewDecoder(bytes.NewReader(parsed.ID))
	idDecoder.UseNumber() // Use number again for the specific ID field
	if err := idDecoder.Decode(&idValue); err != nil {
		// This means the ID field itself contains invalid JSON, e.g., "id": {invalid}
		m.logger.Warn("Failed to decode ID value itself", "rawId", string(parsed.ID), "error", err)
		return nil // Invalid ID content
	}

	// Check the type of the decoded ID value
	switch v := idValue.(type) {
	case json.Number:
		// Try converting to int64 first
		if i, err := v.Int64(); err == nil {
			return i
		}
		// Try float64 if int64 fails
		if f, err := v.Float64(); err == nil {
			return f
		}
		// If number conversion fails (should be rare for valid json.Number)
		m.logger.Warn("Valid json.Number ID type detected but failed number conversion", "rawId", string(parsed.ID))
		return nil // Treat as invalid if conversion fails
	case string:
		return v // Valid string ID
	case nil:
		// This handles the case where "id": null
		return nil // Represent null ID as nil
	default:
		// Invalid types (Arrays, Objects, Booleans) according to JSON-RPC 2.0 spec
		m.logger.Warn("Invalid JSON-RPC ID type detected", "rawId", string(parsed.ID), "goType", fmt.Sprintf("%T", v))
		return nil // Signal invalid type by returning nil
	}
}

// determineIncomingSchemaType applies fallback logic to find the schema for an incoming message.
// nolint:gocyclo // Complexity is high due to multiple fallback checks
func (m *ValidationMiddleware) determineIncomingSchemaType(msgType string) string {
	// Ensure validator exists and is initialized before accessing HasSchema
	if m.validator == nil || !m.validator.IsInitialized() {
		m.logger.Error("determineIncomingSchemaType called but validator is nil or not initialized")
		return "base" // Return base as a "best guess" but log heavily.
	}

	// 1. Try exact match first
	if m.validator.HasSchema(msgType) {
		return msgType
	}

	// 2. Apply fallback logic based on common patterns
	fallbackKey := ""
	switch {
	case strings.HasSuffix(msgType, "_notification"):
		// Try generic notification schema
		if m.validator.HasSchema("JSONRPCNotification") {
			fallbackKey = "JSONRPCNotification"
		} else if m.validator.HasSchema("notification") { // Simpler generic name
			fallbackKey = "notification"
		}
	case strings.Contains(msgType, "Response") || strings.Contains(msgType, "Result") || strings.HasSuffix(msgType, "_response"):
		// Prefer specific error response schema if applicable
		if (strings.Contains(msgType, "Error") || strings.HasSuffix(msgType, "_error")) && m.validator.HasSchema("JSONRPCError") {
			fallbackKey = "JSONRPCError"
		} else if m.validator.HasSchema("JSONRPCResponse") { // Then try generic success response
			fallbackKey = "JSONRPCResponse"
		} else if m.validator.HasSchema("success_response") { // Simpler generic name
			fallbackKey = "success_response"
		} else if m.validator.HasSchema("error_response") { // Simpler generic error name
			fallbackKey = "error_response"
		}
	default: // Assume request
		if m.validator.HasSchema("JSONRPCRequest") {
			fallbackKey = "JSONRPCRequest"
		} else if m.validator.HasSchema("request") { // Simpler generic name
			fallbackKey = "request"
		}
	}

	// 3. Check if the chosen fallback exists
	if fallbackKey != "" && m.validator.HasSchema(fallbackKey) {
		m.logger.Debug("Using fallback schema for incoming message.", "messageType", msgType, "schemaKeyUsed", fallbackKey)
		return fallbackKey
	}

	// 4. If specific type and fallbacks failed, try the absolute base schema
	m.logger.Warn("Specific/generic schema not found for incoming message, trying 'base'.", "messageType", msgType, "triedFallback", fallbackKey)
	if m.validator.HasSchema("base") {
		return "base"
	}

	// 5. If even "base" doesn't exist (major initialization issue)
	m.logger.Error("CRITICAL: No schema found for message type or any fallbacks (including base).", "messageType", msgType)
	return "" // Return empty string to signal complete failure to find schema
}

// calculatePreview generates a short, safe preview of byte data for logging.
func calculatePreview(data []byte) string {
	const maxPreviewLen = 100
	previewLen := len(data)
	suffix := ""
	if previewLen > maxPreviewLen {
		previewLen = maxPreviewLen
		suffix = "..."
	}
	// Replace non-printable characters with '.'
	previewBytes := bytes.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return '.'
		}
		return r
	}, data[:previewLen])
	return string(previewBytes) + suffix
}

// NewValidationError creates a schema validation error.
// Note: This helper might be better placed in the schema package itself.
func NewValidationError(code schema.ErrorCode, message string, cause error) *schema.ValidationError {
	// Ensure cause is wrapped with stack trace if it's not nil and not already wrapped.
	wrappedCause := cause
	if cause != nil {
		wrappedCause = errors.WithStack(cause)
	}

	return &schema.ValidationError{
		Code:    code,
		Message: message,
		Cause:   wrappedCause, // Use the potentially wrapped cause
		Context: map[string]interface{}{
			"timestamp": time.Now().UTC(),
		},
	}
}
