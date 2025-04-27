// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/mcp_server_error_handling.go

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
	"github.com/dkoosis/cowgnition/internal/transport" // Required for JSONRPCInternalError constant
	lfsm "github.com/looplab/fsm"                      // Import for FSM error handling
)

// createErrorResponse creates the byte representation of a JSON-RPC error response.
func (s *Server) createErrorResponse(_ []byte, originalErr error, responseID json.RawMessage) ([]byte, error) {
	// Input validation: Ensure responseID is never nil here (should be "0" if original was null)
	if responseID == nil {
		s.logger.Error("CRITICAL: createErrorResponse called with nil responseID. Defaulting to '0'.")
		responseID = json.RawMessage("0")
		originalErr = errors.Wrap(originalErr, "programmer error: createErrorResponse called with nil ID")
	}

	var code int
	var message string
	var data map[string]interface{}
	var fsmCode string // To store the identified FSM error type string

	// --- START FSM Error Handling ---
	// NOTE: errors.As for looplab/fsm errors requires checking against VALUE types, not pointers.
	var noTransitionErr lfsm.NoTransitionError // Check VALUE type
	var canceledErr lfsm.CanceledError         // Check VALUE type
	var unknownEventErr lfsm.UnknownEventError // Check VALUE type
	var inTransitionErr lfsm.InTransitionError // Check VALUE type

	handledByFSM := true // Assume we can handle it as an FSM error initially

	switch {
	case errors.As(originalErr, &noTransitionErr): // Check for value type
		code = int(mcperrors.ErrRequestSequence) // -32001
		message = "Invalid message sequence."
		fsmCode = "NoTransitionError"
		data = map[string]interface{}{"detail": fmt.Sprintf("Operation not allowed in current state: %s", noTransitionErr.Error())}
		s.logger.Debug("Mapping FSM NoTransitionError", "detail", data["detail"])

	case errors.As(originalErr, &canceledErr): // Check for value type
		code = int(mcperrors.ErrRequestSequence) // -32001
		message = "Operation rejected."
		fsmCode = "CanceledError"
		data = map[string]interface{}{"detail": fmt.Sprintf("Transition rejected by guard: %s", canceledErr.Error())}
		s.logger.Debug("Mapping FSM CanceledError", "detail", data["detail"])

	case errors.As(originalErr, &unknownEventErr): // Check for value type
		code = transport.JSONRPCInvalidRequest // -32600
		message = "Invalid Request."
		fsmCode = "UnknownEventError"
		data = map[string]interface{}{"detail": fmt.Sprintf("Unknown FSM event triggered: %s", unknownEventErr.Error())}
		s.logger.Debug("Mapping FSM UnknownEventError", "detail", data["detail"])

	case errors.As(originalErr, &inTransitionErr): // Check for value type
		code = transport.JSONRPCInternalError // -32603
		message = "Internal Server Error."
		fsmCode = "InTransitionError"
		data = map[string]interface{}{"detail": fmt.Sprintf("Server busy processing previous state change: %s", inTransitionErr.Error())}
		s.logger.Error("FSM InTransitionError occurred", "error", inTransitionErr) // Log internal errors

	// REMOVED fallback string checks, rely on corrected errors.As

	default:
		handledByFSM = false // It wasn't one of the specific FSM errors we checked
	}

	// Add fsmCode to data if identified
	if handledByFSM && fsmCode != "" {
		if data == nil {
			data = make(map[string]interface{})
		}
		data["fsmCode"] = fsmCode
	}
	// --- END FSM Error Handling ---

	// If not handled by specific FSM error mapping, use the general MCP error mapping
	if !handledByFSM {
		s.logger.Debug("Error not identified as specific FSM type, using MapMCPErrorToJSONRPC.", "originalErrorType", fmt.Sprintf("%T", originalErr))
		code, message, data = mcperrors.MapMCPErrorToJSONRPC(originalErr)
	}

	// Log details before marshalling. Use the definitive responseID.
	s.logErrorDetails(code, message, responseID, data, originalErr) // Pass original err here.

	// Construct the payload part of the error
	errorPayload := mcptypes.JSONRPCErrorPayload{ // Use type from mcptypes
		Code:    code,
		Message: message,
		Data:    data, // Assign the potentially enriched data map.
	}

	// Construct the full error response container
	errorResponse := mcptypes.JSONRPCErrorContainer{ // Use type from mcptypes
		JSONRPC: "2.0",
		ID:      responseID, // Use the passed-in responseID
		Error:   errorPayload,
	}

	// Marshal the final response object
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
