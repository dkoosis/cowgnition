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
	lfsm "github.com/looplab/fsm"
)

// createErrorResponse creates the byte representation of a JSON-RPC error response.
func (s *Server) createErrorResponse(_ []byte, originalErr error, responseID json.RawMessage) ([]byte, error) {
	// Input validation for responseID
	if responseID == nil {
		s.logger.Error("CRITICAL: createErrorResponse called with nil responseID. Defaulting to '0'.")
		responseID = json.RawMessage("0")
		originalErr = errors.Wrap(originalErr, "programmer error: createErrorResponse called with nil ID")
	}

	// <<<--- DIAGNOSTIC LOGGING --- >>>
	errorTypeString := "nil"
	if originalErr != nil {
		errorTypeString = reflect.TypeOf(originalErr).String()
	}
	s.logger.Debug(">>> CREATE ERROR RESPONSE: Received error to map",
		"errorType", errorTypeString,
		"errorValue", fmt.Sprintf("%#v", originalErr),
		"errorString", fmt.Sprintf("%v", originalErr))
	// <<<--- END DIAGNOSTIC LOGGING --- >>>

	var code int
	var message string
	var data map[string]interface{}
	errorMapped := false // Flag to track if we successfully mapped the error

	// --- START Explicit FSM Error Handling (Using errors.As) ---
	var noTransitionErr lfsm.NoTransitionError
	var canceledErr lfsm.CanceledError
	var unknownEventErr lfsm.UnknownEventError
	var inTransitionErr lfsm.InTransitionError
	// ... other lfsm error vars ...

	s.logger.Debug("Checking if error matches specific FSM types using errors.As...")
	if errors.As(originalErr, &noTransitionErr) {
		s.logger.Debug("MATCHED lfsm.NoTransitionError via errors.As")
		code = int(mcperrors.ErrRequestSequence) // -32001
		message = "Invalid message sequence."
		data = map[string]interface{}{"detail": fmt.Sprintf("Operation not allowed in current state: %s", noTransitionErr.Error()), "fsmCode": "NoTransitionError"}
		errorMapped = true
	} else if errors.As(originalErr, &canceledErr) {
		s.logger.Debug("MATCHED lfsm.CanceledError via errors.As")
		code = int(mcperrors.ErrRequestSequence) // Or perhaps a different custom code?
		message = "Operation rejected."
		data = map[string]interface{}{"detail": fmt.Sprintf("Transition rejected by guard: %s", canceledErr.Error()), "fsmCode": "CanceledError"}
		errorMapped = true
	} else if errors.As(originalErr, &unknownEventErr) {
		s.logger.Debug("MATCHED lfsm.UnknownEventError via errors.As")
		code = transport.JSONRPCInvalidRequest // -32600
		message = "Invalid Request."
		data = map[string]interface{}{"detail": fmt.Sprintf("Unknown FSM event triggered: %s", unknownEventErr.Error()), "fsmCode": "UnknownEventError"}
		errorMapped = true
	} else if errors.As(originalErr, &inTransitionErr) {
		s.logger.Debug("MATCHED lfsm.InTransitionError via errors.As")
		code = transport.JSONRPCInternalError // -32603
		message = "Internal Server Error."
		data = map[string]interface{}{"detail": fmt.Sprintf("Server busy processing previous state change: %s", inTransitionErr.Error()), "fsmCode": "InTransitionError"}
		s.logger.Error("FSM InTransitionError occurred", "error", inTransitionErr)
		errorMapped = true
		// <<<--- ADDED CLOSING BRACE HERE --- >>>
	} else { // <<<--- This else now correctly follows the 'if/else if' chain
		s.logger.Debug("Error did NOT match specific FSM types via errors.As")
	}
	// --- END Explicit FSM Error Handling (Using errors.As) ---

	// If not handled by specific FSM error mapping, use the general MCP error mapping
	if !errorMapped {
		s.logger.Debug("Error not identified as specific FSM type, proceeding to MapMCPErrorToJSONRPC.", "originalErrorType", fmt.Sprintf("%T", originalErr))
		code, message, data = mcperrors.MapMCPErrorToJSONRPC(originalErr)
		// errorMapped = true // No need to set here, this is the final fallback
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
		return nil, errors.Wrapf(marshalErr, "failed to marshal error response object for original error: %v", originalErr)
	}

	return responseBytes, nil
}

// extractRequestID attempts to get the ID from raw message bytes.
// Returns json.RawMessage("null") if ID is missing, null, or invalid JSON type.
func extractRequestID(logger logging.Logger, msgBytes []byte) json.RawMessage {
	var request struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(msgBytes, &request); err != nil {
		logger.Debug("Could not extract request ID due to JSON parsing error", "error", err)
		return json.RawMessage("null")
	}
	if request.ID != nil {
		idStr := strings.TrimSpace(string(request.ID))
		if idStr == "[]" || idStr == "{}" || idStr == "true" || idStr == "false" {
			logger.Warn("Invalid JSON-RPC ID type (array/object/boolean) found, treating as null for extraction.", "rawId", idStr)
			return json.RawMessage("null")
		}
		if idStr == "null" {
			return json.RawMessage("null")
		}
		return request.ID
	}
	return json.RawMessage("null")
}

// logErrorDetails logs detailed error information server-side.
func (s *Server) logErrorDetails(code int, message string, responseID json.RawMessage, data interface{}, err error) {
	args := []interface{}{
		"jsonrpcErrorCode", code,
		"jsonrpcErrorMessage", message,
		"originalError", fmt.Sprintf("%+v", err),
		"responseIDUsed", string(responseID),
	}
	dataMap, isMap := data.(map[string]interface{})
	if isMap {
		if internalCodeVal, exists := dataMap["internalCode"]; exists {
			if errCode, ok := internalCodeVal.(mcperrors.ErrorCode); ok {
				args = append(args, "internalCode", int(errCode))
			} else {
				args = append(args, "internalCode", internalCodeVal)
			}
		}
		if fsmCodeVal, exists := dataMap["fsmCode"]; exists {
			args = append(args, "fsmCode", fsmCodeVal)
		}
		if detailVal, exists := dataMap["detail"]; exists {
			args = append(args, "errorDetail", detailVal)
		}
	} else if data != nil {
		args = append(args, "errorData", data)
	}
	s.logger.Error("Generating JSON-RPC error response.", args...)
}
