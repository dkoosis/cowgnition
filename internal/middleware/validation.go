// file: internal/middleware/validation.go.
package middleware

import (
	"context"
	"encoding/json"
	"fmt" // Imported fmt for error formatting.
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/schema" // Keep this import for error types.
	"github.com/dkoosis/cowgnition/internal/transport"
)

// --- Interface Definition ---
// This interface defines the contract needed by ValidationMiddleware.
// It allows for mocking the validator in tests. It should be defined ONLY here.
type SchemaValidatorInterface interface {
	Validate(ctx context.Context, messageType string, data []byte) error
	HasSchema(name string) bool
	IsInitialized() bool
	Initialize(ctx context.Context) error // *** Ensure Initialize is included ***.
	// Add other methods if needed by this middleware (e.g., Shutdown, GetLoadDuration).
}

// ValidationOptions contains configuration options for the validation middleware.
type ValidationOptions struct {
	// Enabled determines if validation is active. If false, validation is skipped entirely.
	Enabled bool

	// SkipTypes is a map of message types to skip validation for.
	// Key is the message type (e.g., "ping"), value is a boolean (always true).
	SkipTypes map[string]bool

	// StrictMode determines if validation errors result in rejection.
	// If true (default), validation failures cause messages to be rejected.
	// If false, validation errors are logged but messages still pass through.
	StrictMode bool

	// MeasurePerformance enables logging of validation performance metrics.
	MeasurePerformance bool

	// ValidateOutgoing determines if outgoing messages should be validated.
	// If true, responses will be validated against appropriate schemas before being sent.
	ValidateOutgoing bool

	// StrictOutgoing determines if outgoing validation failures should prevent responses.
	// If true, validation failures for outgoing messages will cause the request to fail.
	// If false (default), outgoing validation errors are logged but messages are still sent.
	StrictOutgoing bool
}

// Define a context key for passing request method through the middleware chain.
type contextKey string

// Constants related to validation.
const (
	// SchemaRelationshipPattern documents how request and response schemas are related.
	// Request method names like "tools/list" should validate responses against "tools/list_response".
	// This naming convention is critical for correct validation and must be maintained.
	SchemaRelationshipPattern = "%s_response"

	// contextKeyRequestMethod is used to track the original request method through the middleware chain
	// to ensure proper schema validation of the corresponding response.
	contextKeyRequestMethod contextKey = "requestMethod"
)

// DefaultValidationOptions returns the default validation options.
// These defaults prioritize correctness and security over performance.
func DefaultValidationOptions() ValidationOptions {
	return ValidationOptions{
		Enabled:            true,
		SkipTypes:          map[string]bool{"ping": true}, // Skip ping by default.
		StrictMode:         true,
		MeasurePerformance: false,
		ValidateOutgoing:   true,  // Enable outgoing validation by default.
		StrictOutgoing:     false, // But don't fail on outgoing validation by default.
	}
}

// ValidationMiddleware validates incoming messages against JSON schemas.
// It serves as a guardian of protocol correctness.
type ValidationMiddleware struct {
	// validator is the schema validator used to validate messages.
	// *** USES THE INTERFACE TYPE NOW ***.
	validator SchemaValidatorInterface

	// options contains the configuration options for this middleware.
	options ValidationOptions

	// next is the next handler in the middleware chain.
	next transport.MessageHandler

	// logger for validation-related events.
	logger logging.Logger
}

// NewValidationMiddleware creates a new validation middleware with the given options.
// *** ACCEPTS THE INTERFACE TYPE NOW ***.
func NewValidationMiddleware(validator SchemaValidatorInterface, options ValidationOptions, logger logging.Logger) *ValidationMiddleware {
	// Use NoopLogger if no logger is provided.
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	return &ValidationMiddleware{
		validator: validator, // Store the provided interface.
		options:   options,
		logger:    logger.WithField("middleware", "validation"),
	}
}

// SetNext sets the next handler in the middleware chain.
func (m *ValidationMiddleware) SetNext(next transport.MessageHandler) {
	m.next = next
}

// calculatePreview generates a string preview of a byte slice, limited to a max length.
func calculatePreview(data []byte) string {
	const maxPreviewLen = 100 // Use a constant for the max length.
	previewLen := len(data)
	if previewLen > maxPreviewLen {
		previewLen = maxPreviewLen
	}
	return string(data[:previewLen])
}

// handleIncomingValidation performs validation steps for an incoming message.
// It returns either an error response to send immediately, or nil if validation passes.
func (m *ValidationMiddleware) handleIncomingValidation(ctx context.Context, message []byte, startTime time.Time) ([]byte, error) {
	// Basic JSON syntax validation first.
	if !json.Valid(message) {
		preview := calculatePreview(message) // Updated call.
		m.logger.Warn("Invalid JSON syntax received.", "messagePreview", preview)
		responseBytes, creationErr := createParseErrorResponse(nil, errors.New("invalid JSON syntax"))
		if creationErr != nil {
			return nil, creationErr // Internal error creating response.
		}
		return responseBytes, nil // Return error response.
	}

	// Identify the message type and extract the request ID.
	msgType, reqID, identifyErr := m.identifyMessage(message)
	if identifyErr != nil {
		preview := calculatePreview(message) // Updated call.
		m.logger.Warn("Failed to identify message type.", "error", identifyErr, "messagePreview", preview)
		// Use the extracted reqID (even if partial) when creating the error response.
		responseBytes, creationErr := createInvalidRequestErrorResponse(reqID, identifyErr)
		if creationErr != nil {
			return nil, creationErr
		}
		return responseBytes, nil // Return error response.
	}

	// Skip validation for exempted message types.
	if m.options.SkipTypes[msgType] {
		m.logger.Debug("Skipping validation for message type.", "type", msgType, "requestID", reqID)
		return nil, nil // Validation skipped, proceed normally.
	}

	// Determine the schema to validate against based on message type.
	schemaType := m.determineSchemaType(msgType, false) // false = incoming message.

	// Perform the validation against the schema using the interface method.
	validationErr := m.validator.Validate(ctx, schemaType, message)

	// Log performance metrics if enabled.
	if m.options.MeasurePerformance {
		elapsed := time.Since(startTime)
		// Cannot get Load/Compile duration from interface unless added.
		m.logger.Debug("Message validation performance.",
			"messageType", msgType,
			"schemaType", schemaType,
			"duration", elapsed,
			"requestID", reqID,
			"isValid", validationErr == nil)
	}

	if validationErr != nil {
		// Handle validation errors according to strict mode setting.
		if m.options.StrictMode {
			m.logger.Warn("Message validation failed (strict mode, rejecting).",
				"messageType", msgType,
				"requestID", reqID,
				"error", validationErr)
			responseBytes, creationErr := createValidationErrorResponse(reqID, validationErr)
			if creationErr != nil {
				return nil, creationErr
			}
			return responseBytes, nil // Return error response.
		}

		// In non-strict mode, log the error but allow processing to continue.
		m.logger.Warn("Validation error (passing through in non-strict mode).",
			"messageType", msgType,
			"requestID", reqID,
			"error", validationErr)
	}

	// Validation passed or non-strict mode with error.
	return nil, nil
}

// handleOutgoingValidation performs validation steps for an outgoing response.
// It logs errors but does not prevent the response from being sent unless configured to do so.
func (m *ValidationMiddleware) handleOutgoingValidation(ctx context.Context, responseBytes []byte) error {
	// Don't validate error responses - they're generated by our framework.
	// and should already be compliant.
	if isErrorResponse(responseBytes) {
		return nil
	}

	outMsgType, _, outIdentifyErr := m.identifyMessage(responseBytes) // Remove reqID from here.
	if outIdentifyErr != nil {
		preview := calculatePreview(responseBytes)
		m.logger.Warn("Failed to identify outgoing message type for validation.",
			"error", outIdentifyErr,
			"messagePreview", preview)
		outMsgType = "unknown" // Use generic type for validation if specific type can't be determined.
	}

	// Determine appropriate schema for response validation.
	outSchemaType := m.determineSchemaType(outMsgType, true) // true = outgoing response.

	// Check for tools response for specialized validation.
	isToolsResponse := strings.Contains(outMsgType, "tools") || strings.Contains(outMsgType, "tool")

	outValidationErr := m.validator.Validate(ctx, outSchemaType, responseBytes)
	if outValidationErr != nil {
		preview := calculatePreview(responseBytes)
		m.logger.Error("Outgoing message validation failed!",
			"messageType", outMsgType,
			"schemaType", outSchemaType,
			"error", outValidationErr,
			"messagePreview", preview)

		// Perform additional checks for tools responses.
		if isToolsResponse {
			m.performToolNameValidation(responseBytes)
		}

		// In strict outgoing mode, return the error to prevent sending invalid responses.
		if m.options.StrictOutgoing {
			return outValidationErr
		}
		// Otherwise, log but don't fail.
	}

	return nil
}

// HandleMessage implements the MessageHandler interface.
// It validates the message if validation is enabled, then passes it to the next handler.
// The returned response will also be validated if outgoing validation is enabled.
// HandleMessage implements the MessageHandler interface.
func (m *ValidationMiddleware) HandleMessage(ctx context.Context, message []byte) ([]byte, error) {
	// Fast path: If validation is disabled, skip directly to the next handler.
	if !m.options.Enabled {
		m.logger.Debug("Validation disabled, skipping.")
		// Ensure 'next' is not nil before calling.
		if m.next == nil {
			return nil, errors.New("validation middleware has no next handler configured")
		}
		return m.next(ctx, message)
	}

	// Start measuring performance if enabled.
	var startTime time.Time
	if m.options.MeasurePerformance {
		startTime = time.Now()
	}

	// Extract the message method for tracking request/response relationship
	msgType, _, err := m.identifyMessage(message)
	if err != nil {
		// Handle identification error
		m.logger.Warn("Failed to identify message type.", "error", err)
		// Continue with validation attempt anyway
	}

	// Perform incoming validation.
	errorResponseBytes, validationErr := m.handleIncomingValidation(ctx, message, startTime)
	if validationErr != nil {
		// Internal error occurred during validation or response creation.
		return nil, validationErr
	}
	if errorResponseBytes != nil {
		// Validation failed and an error response was generated. Send it back.
		return errorResponseBytes, nil
	}

	// If validation passed or non-strict mode allowed it, continue to the next handler.
	// Ensure 'next' is not nil before calling.
	if m.next == nil {
		return nil, errors.New("validation middleware reached end of chain without a final handler")
	}

	// Store message type in context for response validation
	ctxWithMsgType := context.WithValue(ctx, contextKeyRequestMethod, msgType)
	responseBytes, err := m.next(ctxWithMsgType, message)

	// If we got a response and outgoing validation is enabled, validate the response.
	if err == nil && responseBytes != nil && m.options.ValidateOutgoing {
		// Extract the original request method from context
		requestMethod, ok := ctxWithMsgType.Value(contextKeyRequestMethod).(string)
		if !ok || requestMethod == "" {
			requestMethod = msgType // Fallback to what we extracted earlier
		}

		// This is the key change - directly construct the expected response schema type
		outSchemaType := ""

		// For tools and other namespaced methods, preserve the namespace
		if strings.Contains(requestMethod, "/") {
			outSchemaType = requestMethod + "_response"
		} else if requestMethod != "" && !strings.HasSuffix(requestMethod, "_notification") {
			outSchemaType = requestMethod + "_response"
		} else {
			// For error responses or other types, call identifyMessage
			outMsgType, _, outIdentifyErr := m.identifyMessage(responseBytes)
			if outIdentifyErr == nil {
				outSchemaType = outMsgType
			} else {
				// Fallback to generic schema types
				if isErrorResponse(responseBytes) {
					outSchemaType = "error_response"
				} else {
					outSchemaType = "success_response"
				}
			}
		}

		// Skip validation for error responses (generated by framework)
		if !isErrorResponse(responseBytes) {
			// Perform the actual validation with the determined schema type
			if outgoingErr := m.validator.Validate(ctx, outSchemaType, responseBytes); outgoingErr != nil && m.options.StrictOutgoing {
				return nil, errors.Wrap(outgoingErr, "failed outgoing message validation")
			}
		}
	}

	return responseBytes, err
}

// determineSchemaType selects the appropriate schema name for validation based on the message type.
// It handles both incoming requests and outgoing responses.
func (m *ValidationMiddleware) determineSchemaType(msgType string, isResponse bool) string {
	var schemaType string // Declare variable.

	if isResponse {
		// For responses, use specific response schemas if available.
		if strings.HasSuffix(msgType, "_response") {
			// Already has _response suffix.
			schemaType = msgType
		} else {
			// Add response suffix for method-specific response schemas.
			schemaType = msgType + "_response"
		}

		// Check if this schema exists in the validator.
		if !m.validator.HasSchema(schemaType) {
			// Fall back to generic response schema.
			if strings.Contains(msgType, "error") {
				schemaType = "error_response"
			} else {
				schemaType = "success_response"
			}
		}
	} else {
		// For requests/notifications, use method-specific schemas if available.
		if m.validator.HasSchema(msgType) {
			schemaType = msgType
		} else if strings.HasSuffix(msgType, "_notification") {
			// Check for generic notification if specific one doesn't exist.
			if m.validator.HasSchema("notification") {
				schemaType = "notification" // Generic notification schema.
			} else {
				schemaType = msgType // Keep original if generic doesn't exist either.
			}
		} else {
			// Fallback for unknown request types.
			if m.validator.HasSchema("request") {
				schemaType = "request" // Generic request schema.
			} else {
				schemaType = msgType // Keep original if generic doesn't exist.
			}
		}
	}

	// Fallback one more time if the determined type doesn't exist.
	if !m.validator.HasSchema(schemaType) {
		m.logger.Warn("Specific schema type not found, falling back to base schema.", "attemptedType", schemaType)
		schemaType = "base" // Assuming "base" is always a valid fallback.
	}

	return schemaType
}

// isErrorResponse checks if the given JSON-RPC message is an error response.
func isErrorResponse(message []byte) bool {
	// Quick check without full parsing.
	return strings.Contains(string(message), `"error":`)
}

// identifyMessage extracts the message type and request ID from a JSON-RPC message.
// Returns message type (method name or response type), request ID (if present), and error.
func (m *ValidationMiddleware) identifyMessage(message []byte) (string, interface{}, error) {
	// Parse just enough of the message to identify type.
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(message, &parsed); err != nil {
		return "", nil, errors.Wrap(err, "failed to parse message for identification")
	}

	// Extract ID if present and VALIDATE ITS TYPE.
	var id interface{} // Will store the actual string, number, or nil.
	// var idTypeValid bool = true // This variable is no longer needed with the check inside.
	idRaw, idExists := parsed["id"]
	if idExists && string(idRaw) != "null" { // Check presence and non-null before unmarshalling.
		// Unmarshal preserves null, string, or number types for ID.
		if err := json.Unmarshal(idRaw, &id); err != nil {
			// If ID exists but is invalid format (e.g., malformed string).
			// Pass the raw ID back in the error context if possible.
			return "", string(idRaw), errors.Wrap(err, "failed to parse id")
		}
		// *** TYPE CHECK ***.
		switch id.(type) {
		case string, float64, json.Number:
			// Valid types.
		default:
			// Invalid type (e.g., object, array).
			// Return error immediately if type is invalid.
			return "", id, errors.New(fmt.Sprintf("invalid type for id: expected string, number, or null, got %T", id))
		}
	} else if idExists && string(idRaw) == "null" {
		// Explicit null ID is treated as null.
		id = nil
	} else {
		// ID is absent (valid for notifications).
		id = nil
	}

	// Check if it's a request/notification (has 'method') or response (has 'result' or 'error').
	if methodRaw, ok := parsed["method"]; ok {
		// It's a request or notification, extract the method name.
		var method string
		if err := json.Unmarshal(methodRaw, &method); err != nil {
			return "", id, errors.Wrap(err, "failed to parse method")
		}
		// Distinguish request (has ID) from notification (no ID or null ID).
		if id == nil {
			return method + "_notification", nil, nil // Return nil ID for notifications.
		}
		return method, id, nil // Method name and ID for request.
	}

	// If it has 'result', it's a success response.
	if _, hasResult := parsed["result"]; hasResult {
		// Try to identify the original request method from context if possible,
		// otherwise return generic success.
		// This part is complex without passing the original method info down.
		// For now, return a generic type.
		return "success_response", id, nil
	}

	// If it has 'error', it's an error response.
	if _, hasError := parsed["error"]; hasError {
		return "error_response", id, nil
	}

	// If we can't identify the message type, return an error.
	return "", id, errors.New("unable to identify message type (not request, notification, or response)")
}

// --- Helper Functions to Create JSON-RPC Error Responses ---.
// These functions now correctly return ([]byte, error).

// createParseErrorResponse creates JSON bytes for a JSON-RPC parse error (-32700).
func createParseErrorResponse(id interface{}, parseErr error) ([]byte, error) {
	if id == nil { // ID is often unknown if parsing failed early.
		id = json.RawMessage("null")
	}
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    transport.JSONRPCParseError, // -32700.
			"message": "Parse error.",
			"data": map[string]interface{}{
				"details": parseErr.Error(), // Provide underlying parse error detail.
			},
		},
	}
	return json.Marshal(response) // Returns ([]byte, error).
}

// createInvalidRequestErrorResponse creates JSON bytes for a JSON-RPC invalid request error (-32600).
func createInvalidRequestErrorResponse(id interface{}, requestErr error) ([]byte, error) {
	if id == nil {
		id = json.RawMessage("null")
	}
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    transport.JSONRPCInvalidRequest, // -32600.
			"message": "Invalid Request.",
			"data": map[string]interface{}{
				"details": requestErr.Error(),
			},
		},
	}
	return json.Marshal(response) // Returns ([]byte, error).
}

// createValidationErrorResponse creates JSON bytes for a JSON-RPC validation error.
// Maps schema validation errors to Invalid Request (-32600) or Invalid Params (-32602).
func createValidationErrorResponse(id interface{}, validationErr error) ([]byte, error) {
	if id == nil {
		id = json.RawMessage("null")
	}
	code := transport.JSONRPCInvalidRequest // -32600 Default to Invalid Request.
	message := "Invalid Request."
	var errorData interface{} // Keep nil unless details are useful/safe.

	// Check if it's a structured schema validation error.
	var schemaValErr *schema.ValidationError
	if errors.As(validationErr, &schemaValErr) {
		// Check if the error path indicates it's a parameter issue.
		if schemaValErr.InstancePath != "" && (strings.Contains(schemaValErr.InstancePath, "/params") || strings.Contains(schemaValErr.InstancePath, "params")) {
			code = transport.JSONRPCInvalidParams // -32602.
			message = "Invalid params."
		}
		// Optionally include sanitized validation context.
		// errorData = schemaValErr.Context // Example, ensure sanitization.
		errorData = map[string]interface{}{"details": schemaValErr.Error()} // Include formatted error.
	} else {
		// For generic validation errors.
		errorData = map[string]interface{}{"details": validationErr.Error()}
	}

	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
			"data":    errorData, // Include structured details if available.
		},
	}
	return json.Marshal(response) // Returns ([]byte, error).
}

// performToolNameValidation extracts and validates tool names from a tools response.
func (m *ValidationMiddleware) performToolNameValidation(responseBytes []byte) {
	var toolsResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}

	if err := json.Unmarshal(responseBytes, &toolsResp); err != nil {
		m.logger.Debug("Could not parse tools response for validation.", "error", err)
		return
	}

	for i, tool := range toolsResp.Result.Tools {
		if err := schema.ValidateName(schema.EntityTypeTool, tool.Name); err != nil {
			m.logger.Error("Invalid tool name in response.",
				"toolIndex", i,
				"toolName", tool.Name,
				"error", err,
				"rules", schema.GetNamePatternDescription(schema.EntityTypeTool))
		}
	}
}

func (m *ValidationMiddleware) determineResponseSchemaType(requestMethod string, responseBytes []byte) string {
	// If we have a request method, use it to derive the response schema type
	if requestMethod != "" && !strings.HasSuffix(requestMethod, "_notification") {
		responseSchema := fmt.Sprintf(SchemaRelationshipPattern, requestMethod)
		// Check if this schema exists
		if m.validator.HasSchema(responseSchema) {
			return responseSchema
		}
	}

	// Fallback: determine schema type from the response itself
	outMsgType, _, err := m.identifyMessage(responseBytes)
	if err == nil {
		if m.validator.HasSchema(outMsgType) {
			return outMsgType
		}
	}

	// Last resort: generic schema types
	if isErrorResponse(responseBytes) {
		return "error_response"
	}
	return "success_response"
}
