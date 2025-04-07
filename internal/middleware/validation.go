// file: internal/middleware/validation.go
package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/schema"
)

// MessageHandler defines the function signature for message processors.
type MessageHandler func(ctx context.Context, message []byte) ([]byte, error)

// JSONRPCError represents a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// JSONRPCErrorResponse represents a JSON-RPC 2.0 error response.
type JSONRPCErrorResponse struct {
	JSONRPC string       `json:"jsonrpc"`
	ID      interface{}  `json:"id"`
	Error   JSONRPCError `json:"error"`
}

// Standard JSON-RPC 2.0 error codes.
const (
	ParseError     = -32700 // Invalid JSON
	InvalidRequest = -32600 // Invalid Request object
	MethodNotFound = -32601 // Method not found
	InvalidParams  = -32602 // Invalid method parameters
	InternalError  = -32603 // Internal error
)

// ValidationMiddleware implements middleware for validating MCP messages.
// It validates incoming messages against the MCP schema before passing
// them to the next handler.
type ValidationMiddleware struct {
	// validator is used to validate messages against the MCP schema.
	validator *schema.SchemaValidator
	// next is the next handler in the middleware chain.
	next MessageHandler
	// logger is the structured logger to use for validation errors.
	logger *log.Logger
}

// NewValidationMiddleware creates a new validation middleware.
func NewValidationMiddleware(validator *schema.SchemaValidator, next MessageHandler, logger *log.Logger) *ValidationMiddleware {
	if logger == nil {
		// Create a default logger if none provided
		logger = log.New(log.Writer(), "validation: ", log.LstdFlags|log.Lshortfile)
	}

	return &ValidationMiddleware{
		validator: validator,
		next:      next,
		logger:    logger,
	}
}

// HandleMessage processes incoming messages, validating them against the MCP schema.
func (m *ValidationMiddleware) HandleMessage(ctx context.Context, message []byte) ([]byte, error) {
	// 1. Basic check for valid JSON
	if !json.Valid(message) {
		m.logger.Printf("Invalid JSON received: %s", limitString(string(message), 100))
		return createErrorResponse(nil, ParseError, "Parse error", nil), nil
	}

	// 2. Identify message type and request ID
	msgType, requestID, err := identifyMessageType(message)
	if err != nil {
		m.logger.Printf("Failed to identify message type: %v", err)
		return createErrorResponse(requestID, InvalidRequest, "Invalid Request", map[string]interface{}{
			"detail": "Could not determine message type",
		}), nil
	}

	// 3. Validate against schema
	validationErr := m.validator.Validate(ctx, msgType, message)
	if validationErr != nil {
		// Log detailed error information
		var valErr *schema.ValidationError
		if errors.As(validationErr, &valErr) {
			m.logger.Printf("Schema validation failed: %s (schema path: %s, instance path: %s)",
				valErr.Message, valErr.SchemaPath, valErr.InstancePath)

			// Map validation error to appropriate JSON-RPC error
			return m.mapValidationErrorToResponse(requestID, valErr)
		}

		// Handle other validation errors
		m.logger.Printf("Validation error: %v", validationErr)
		return createErrorResponse(requestID, InvalidRequest, "Invalid Request", map[string]interface{}{
			"detail": validationErr.Error(),
		}), nil
	}

	// 4. Validation passed, proceed to next handler
	return m.next(ctx, message)
}

// identifyMessageType determines the message type and extracts the request ID.
func identifyMessageType(message []byte) (string, interface{}, error) {
	// Unmarshal just enough to determine the type
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		return "", nil, errors.Wrap(err, "failed to unmarshal message for type identification")
	}

	// Get request ID if present
	var requestID interface{}
	if idBytes, ok := msg["id"]; ok {
		if err := json.Unmarshal(idBytes, &requestID); err != nil {
			// If ID can't be unmarshaled, continue with nil ID
			requestID = nil
		}
	}

	// Check if it's a batch request
	if len(msg) == 0 && json.Valid(message) {
		// Message appears to be a valid JSON array (batch)
		var arr []interface{}
		if err := json.Unmarshal(message, &arr); err == nil && len(arr) > 0 {
			return "JSONRPCBatch", requestID, nil
		}
	}

	// Determine message type based on fields
	_, hasMethod := msg["method"]
	_, hasResult := msg["result"]
	_, hasError := msg["error"]

	switch {
	case hasMethod && hasError:
		// Invalid: method and error should not coexist
		return "", requestID, errors.New("invalid message: contains both method and error")
	case hasMethod && hasResult:
		// Invalid: method and result should not coexist
		return "", requestID, errors.New("invalid message: contains both method and result")
	case hasResult && hasError:
		// Invalid: result and error should not coexist
		return "", requestID, errors.New("invalid message: contains both result and error")
	case hasMethod:
		// It's a request or notification
		if requestID != nil {
			// Look for specific method to further refine the type
			var methodStr string
			if err := json.Unmarshal(msg["method"], &methodStr); err == nil {
				// Map method name to specific request type if known
				switch methodStr {
				case "initialize":
					return "InitializeRequest", requestID, nil
				case "ping":
					return "PingRequest", requestID, nil
				case "resources/list":
					return "ListResourcesRequest", requestID, nil
				case "resources/read":
					return "ReadResourceRequest", requestID, nil
				case "tools/list":
					return "ListToolsRequest", requestID, nil
				case "tools/call":
					return "CallToolRequest", requestID, nil
				case "prompts/list":
					return "ListPromptsRequest", requestID, nil
				case "prompts/get":
					return "GetPromptRequest", requestID, nil
				default:
					// Generic request if method not specifically recognized
					return "ClientRequest", requestID, nil
				}
			}
			return "JSONRPCRequest", requestID, nil
		}
		// Notification (no ID)
		var methodStr string
		if err := json.Unmarshal(msg["method"], &methodStr); err == nil {
			// Map method name to specific notification type if known
			switch methodStr {
			case "notifications/initialized":
				return "InitializedNotification", requestID, nil
			case "notifications/cancelled":
				return "CancelledNotification", requestID, nil
			case "notifications/roots/list_changed":
				return "RootsListChangedNotification", requestID, nil
			default:
				// Generic notification if method not specifically recognized
				return "JSONRPCNotification", requestID, nil
			}
		}
		return "JSONRPCNotification", requestID, nil
	case hasResult:
		// It's a success response
		return "JSONRPCResponse", requestID, nil
	case hasError:
		// It's an error response
		return "JSONRPCError", requestID, nil
	default:
		// Can't determine type
		return "", requestID, errors.New("unable to determine message type")
	}
}

// mapValidationErrorToResponse maps a validation error to an appropriate JSON-RPC error response.
func (m *ValidationMiddleware) mapValidationErrorToResponse(requestID interface{}, err *schema.ValidationError) ([]byte, error) {
	// Default is InvalidRequest
	code := InvalidRequest
	message := "Invalid Request"

	// Create detailed error data
	data := map[string]interface{}{
		"detail":       err.Message,
		"schemaPath":   err.SchemaPath,
		"instancePath": err.InstancePath,
	}

	// Add any additional context that's safe to expose
	for k, v := range err.Context {
		// Only include non-sensitive context data
		if k != "dataPreview" && k != "timestamp" {
			data[k] = v
		}
	}

	// More specific error mapping based on error details could be added here
	// For example, parameter validation failures could map to InvalidParams
	if err.InstancePath != "" && (err.InstancePath == "/params" ||
		len(err.InstancePath) > 7 && err.InstancePath[:7] == "/params/") {
		code = InvalidParams
		message = "Invalid Params"
	}

	return createErrorResponse(requestID, code, message, data), nil
}

// createErrorResponse creates a JSON-RPC 2.0 error response.
func createErrorResponse(id interface{}, code int, message string, data interface{}) []byte {
	response := JSONRPCErrorResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	respBytes, err := json.Marshal(response)
	if err != nil {
		// If marshaling fails, return a simpler error response
		fallback := fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"error":{"code":%d,"message":"Internal error marshaling response"}}`, id, InternalError)
		return []byte(fallback)
	}

	return respBytes
}

// limitString limits a string to a maximum length for logging.
func limitString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
