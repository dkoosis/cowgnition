// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/mcp_server_error_handling.go

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
	"github.com/dkoosis/cowgnition/internal/transport"
	lfsm "github.com/looplab/fsm" // Keep alias
)

// createErrorResponse creates the byte representation of a JSON-RPC error response.
func (s *Server) createErrorResponse(_ []byte, originalErr error, responseID json.RawMessage) ([]byte, error) {
	// Input validation for responseID
	if responseID == nil {
		s.logger.Error("CRITICAL: createErrorResponse called with nil responseID. Defaulting to '0'.")
		responseID = json.RawMessage("0")
		originalErr = errors.Wrap(originalErr, "programmer error: createErrorResponse called with nil ID")
	}

	errorTypeString := "nil"
	if originalErr != nil {
		errorTypeString = reflect.TypeOf(originalErr).String()
	}
	s.logger.Debug(">>> CREATE ERROR RESPONSE: Received error to map",
		"errorType", errorTypeString,
		"errorValue", fmt.Sprintf("%#v", originalErr), // Use %#v for more detail if needed
		"errorString", fmt.Sprintf("%v", originalErr))

	var code int
	var message string
	var data map[string]interface{}
	errorMapped := false // Flag to track if we successfully mapped the error

	// --- START Explicit FSM Error Handling (Using errors.As) ---
	var invalidEventErr lfsm.InvalidEventError // <<< Specific type for invalid event sequence
	var noTransitionErr lfsm.NoTransitionError
	var canceledErr lfsm.CanceledError
	var unknownEventErr lfsm.UnknownEventError
	var inTransitionErr lfsm.InTransitionError

	s.logger.Debug("Checking if error matches specific FSM types using errors.As...")
	// --- ADDED: Check for InvalidEventError first ---
	if errors.As(originalErr, &invalidEventErr) {
		s.logger.Debug("MATCHED lfsm.InvalidEventError via errors.As")
		code = int(mcperrors.ErrRequestSequence) // -32001
		message = "Invalid message sequence."
		// Include specific FSM error details in data
		data = map[string]interface{}{
			"fsmCode": "InvalidEventError",
			"detail":  invalidEventErr.Error(), // Use the error's message
			"event":   invalidEventErr.Event,
			"state":   invalidEventErr.State,
		}
		errorMapped = true
	} else if errors.As(originalErr, &noTransitionErr) {
		s.logger.Debug("MATCHED lfsm.NoTransitionError via errors.As")
		code = int(mcperrors.ErrRequestSequence) // Treat as sequence issue? Or internal? Sequence seems reasonable.
		message = "Invalid message sequence (no state change)."
		data = map[string]interface{}{
			"fsmCode": "NoTransitionError",
			"detail":  noTransitionErr.Error(),
		}
		if noTransitionErr.Err != nil { // Include underlying error if present
			data["cause"] = noTransitionErr.Err.Error()
		}
		errorMapped = true
	} else if errors.As(originalErr, &canceledErr) {
		s.logger.Debug("MATCHED lfsm.CanceledError via errors.As")
		code = int(mcperrors.ErrRequestSequence) // Or perhaps a different custom code like "OperationRejected"? Sequence for now.
		message = "Operation rejected by guard."
		data = map[string]interface{}{
			"fsmCode": "CanceledError",
			"detail":  canceledErr.Error(),
		}
		if canceledErr.Err != nil { // Include underlying error if present
			data["cause"] = canceledErr.Err.Error()
		}
		errorMapped = true
	} else if errors.As(originalErr, &unknownEventErr) {
		s.logger.Debug("MATCHED lfsm.UnknownEventError via errors.As")
		code = transport.JSONRPCMethodNotFound // Map unknown event to MethodNotFound (-32601)
		message = "Method not found."
		data = map[string]interface{}{
			"fsmCode": "UnknownEventError",
			"detail":  unknownEventErr.Error(),
			"event":   unknownEventErr.Event,
		}
		errorMapped = true
	} else if errors.As(originalErr, &inTransitionErr) {
		s.logger.Debug("MATCHED lfsm.InTransitionError via errors.As")
		code = transport.JSONRPCInternalError // Treat concurrent transition issues as Internal (-32603)
		message = "Internal Server Error (concurrent state change)."
		data = map[string]interface{}{
			"fsmCode": "InTransitionError",
			"detail":  inTransitionErr.Error(),
			"event":   inTransitionErr.Event,
		}
		s.logger.Error("FSM InTransitionError occurred - potential concurrency issue", "error", inTransitionErr)
		errorMapped = true
	} else {
		s.logger.Debug("Error did NOT match specific FSM types via errors.As")
	}
	// --- END Explicit FSM Error Handling ---

	// If not handled by specific FSM error mapping, use the general MCP error mapping
	if !errorMapped {
		s.logger.Debug("Error not identified as specific FSM type, proceeding to MapMCPErrorToJSONRPC.", "originalErrorType", fmt.Sprintf("%T", originalErr))
		code, message, data = mcperrors.MapMCPErrorToJSONRPC(originalErr)
	}

	// Log details before marshalling
	s.logErrorDetails(code, message, responseID, data, originalErr)

	// Construct and marshal the response
	errorPayload := mcptypes.JSONRPCErrorPayload{Code: code, Message: message, Data: data}
	errorResponse := mcptypes.JSONRPCErrorContainer{JSONRPC: "2.0", ID: responseID, Error: errorPayload}
	responseBytes, marshalErr := json.Marshal(errorResponse)
	if marshalErr != nil {
		s.logger.Error("CRITICAL: Failed to marshal final error response.",
			"targetID", string(responseID),
			"marshalError", fmt.Sprintf("%+v", marshalErr),
			"originalError", fmt.Sprintf("%+v", originalErr),
		)
		// Wrap the marshalling error, including context about the original error
		return nil, errors.Wrapf(marshalErr, "failed to marshal error response object for original error type %T: %v", originalErr, originalErr)
	}

	return responseBytes, nil
}

// extractRequestID attempts to get the ID from raw message bytes.
// Returns json.RawMessage("null") if ID is missing, null, or invalid JSON type.
func extractRequestID(logger logging.Logger, msgBytes []byte) json.RawMessage {
	var request struct {
		ID json.RawMessage `json:"id"`
	}
	// Use Unmarshal directly on the byte slice
	if err := json.Unmarshal(msgBytes, &request); err != nil {
		// Log if parsing fails, but still return null ID for error response consistency
		logger.Debug("Could not extract request ID due to JSON parsing error during error handling", "error", err)
		return json.RawMessage("null")
	}

	// Check if ID field was present in the JSON
	if request.ID != nil {
		idStr := strings.TrimSpace(string(request.ID))
		// Handle explicitly invalid ID types according to JSON-RPC spec (arrays, objects, booleans)
		if idStr == "[]" || idStr == "{}" || idStr == "true" || idStr == "false" {
			logger.Warn("Invalid JSON-RPC ID type (array/object/boolean) found in request, treating as null for error response.", "rawId", idStr)
			return json.RawMessage("null")
		}
		// Return the valid ID (including explicit null, string, or number)
		return request.ID
	}

	// ID field was missing entirely
	return json.RawMessage("null")
}

// logErrorDetails logs detailed error information server-side.
func (s *Server) logErrorDetails(code int, message string, responseID json.RawMessage, data interface{}, err error) {
	args := []interface{}{
		"jsonrpcErrorCode", code,
		"jsonrpcErrorMessage", message,
		"originalError", fmt.Sprintf("%+v", err), // Log with stack trace using %+v
		"responseIDUsed", string(responseID),
	}

	// Safely add details from the data map if it exists
	if dataMap, ok := data.(map[string]interface{}); ok && dataMap != nil {
		// Helper function to append if key exists
		appendIfExists := func(key string, logKey string) {
			if val, exists := dataMap[key]; exists {
				// Special handling for internalCode to cast if possible
				if key == "internalCode" {
					if errCode, isCode := val.(mcperrors.ErrorCode); isCode {
						args = append(args, logKey, int(errCode))
					} else {
						args = append(args, logKey, val) // Append original value if cast fails
					}
				} else {
					args = append(args, logKey, val)
				}
			}
		}
		appendIfExists("internalCode", "internalCode")
		appendIfExists("fsmCode", "fsmCode")
		appendIfExists("detail", "errorDetail")
		// Add other specific fields from 'data' if needed, e.g., "event", "state"
		appendIfExists("event", "fsmEvent")
		appendIfExists("state", "fsmState")
		appendIfExists("cause", "errorCause") // If cause is added to data map
	} else if data != nil {
		// If data is not a map but not nil, log its type and value
		args = append(args, "errorDataType", fmt.Sprintf("%T", data))
		args = append(args, "errorDataValue", fmt.Sprintf("%#v", data))
	}

	s.logger.Error("Generating JSON-RPC error response.", args...)
}
