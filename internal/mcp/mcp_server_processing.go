// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// file: internal/mcp/mcp_server_processing.go.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cockroachdb/errors"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
	"github.com/dkoosis/cowgnition/internal/transport"
)

// serverProcessing handles the main server loop, reading messages and dispatching them.
func (s *Server) serverProcessing(ctx context.Context, handlerFunc mcptypes.MessageHandler) error {
	s.logger.Info("Server processing loop started.")
	if handlerFunc == nil {
		return errors.New("serve called with nil handler function")
	}
	if s.transport == nil {
		return errors.New("serve called but server transport is nil")
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Context canceled, stopping server loop.")
			return ctx.Err()
		default:
			// processNextMessage reads, handles, and responds to a single message.
			// It returns terminal errors that should stop the loop.
			if err := s.processNextMessage(ctx, handlerFunc); err != nil {
				// isTerminalError checks for EOF, closed transport, context cancellation etc.
				if s.isTerminalError(err) {
					s.logger.Info("Terminal error received, stopping server loop.", "reason", err)
					return err // Propagate terminal error to stop serve.
				}
				// Log non-terminal errors but continue the loop.
				s.logger.Error("Non-terminal error processing message", "error", fmt.Sprintf("%+v", err))
				// Optionally, implement retry logic or specific error handling here.
			}
		}
	}
}

// processNextMessage handles reading, processing, and responding to a single message.
// It returns non-nil error only for terminal conditions. Other processing errors are handled internally
// by sending a JSON-RPC error response.
func (s *Server) processNextMessage(ctx context.Context, handlerFunc mcptypes.MessageHandler) error {
	// 1. Read Message.
	msgBytes, readErr := s.transport.ReadMessage(ctx)
	if readErr != nil {
		// Let handleTransportReadError decide if it's terminal.
		return s.handleTransportReadError(readErr)
	}

	// 2. Extract Info for Logging/Context (Best Effort).
	method, idStr := s.extractMessageInfo(msgBytes)
	ctxWithState := context.WithValue(ctx, connectionStateKey, s.connectionState) // Add connection state.

	// 3. Handle Message via Middleware Chain / Final Handler.
	respBytes, handleErr := handlerFunc(ctxWithState, msgBytes)

	// 4. Handle Processing Error (if any).
	if handleErr != nil {
		// handleProcessingError logs the error and attempts to create/write a JSON-RPC error response.
		// It returns an error only if writing the error response fails.
		writeErr := s.handleProcessingError(ctx, msgBytes, method, idStr, handleErr)
		if writeErr != nil {
			// If we can't even write the error response, it might be a terminal transport issue.
			return errors.Wrap(writeErr, "failed to write error response after processing error")
		}
		// If handleProcessingError succeeded in sending an error response,
		// we don't return an error here, allowing the loop to continue.
		return nil
	}

	// 5. Handle State Update for Initialize Success.
	// Check if the successful response was for an "initialize" method.
	if method == "initialize" && respBytes != nil {
		var respObj struct {
			Error *json.RawMessage `json:"error"` // Only check if 'error' field exists.
		}
		// Check if the response indicates success (no 'error' field).
		if err := json.Unmarshal(respBytes, &respObj); err == nil && respObj.Error == nil {
			s.logger.Info("Initialize request successful, marking connection as initialized.")
			// Safely update connection state (assuming connectionState is thread-safe or accessed serially).
			if s.connectionState != nil {
				s.connectionState.SetInitialized()
			} else {
				s.logger.Warn("Connection state is nil, cannot mark as initialized.")
			}
		} else if err != nil {
			// Log if parsing the response fails, but don't prevent sending it.
			s.logger.Warn("Failed to parse successful response during initialize state check.", "error", err)
		}
		// No state change if the initialize response contained an error.
	}

	// 6. Write Successful Response (if one was generated).
	if respBytes != nil {
		if writeErr := s.writeResponse(ctx, respBytes, method, idStr); writeErr != nil {
			// If writing the success response fails, return the error.
			return errors.Wrap(writeErr, "failed to write successful response")
		}
	}

	// Successfully processed the message.
	return nil
}

// handleMessage is the final handler in the middleware chain.
// It routes validated messages to the appropriate method handler.
func (s *Server) handleMessage(ctx context.Context, msgBytes []byte) ([]byte, error) {
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id,omitempty"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"` // Keep as RawMessage for handler.
	}

	// We assume the message has already passed basic JSON validation and schema checks
	// by the time it reaches this handler via the middleware chain.
	if err := json.Unmarshal(msgBytes, &request); err != nil {
		// This indicates an internal issue, as middleware should have caught parse errors.
		return nil, errors.Wrap(err, "internal error: failed to parse validated message in handleMessage")
	}

	// Double-check method sequence against connection state (safety net).
	// Middleware should ideally handle this, but check again here.
	if s.connectionState != nil {
		if err := s.connectionState.ValidateMethodSequence(request.Method); err != nil {
			// Map this specific error type for JSON-RPC response creation.
			return nil, errors.Wrapf(err, "method sequence validation failed")
		}
	} else {
		s.logger.Error("Connection state is nil in handleMessage, cannot validate sequence.")
		// Potentially return an internal error here if state is critical.
		// For now, allow proceeding but log loudly.
	}

	// Find the registered handler for the method.
	handler, ok := s.methods[request.Method]
	if !ok {
		// Return a specific error that mapErrorToJSONRPCComponents can recognize.
		return nil, errors.Newf("Method not found: %s", request.Method)
	}

	// Execute the specific method handler.
	resultBytes, handlerErr := handler(ctx, request.Params)
	if handlerErr != nil {
		// Wrap the error from the handler for context.
		return nil, errors.Wrapf(handlerErr, "error executing method '%s'", request.Method)
	}

	// If it was a notification (no ID), we don't send a response.
	if request.ID == nil || string(request.ID) == "null" {
		s.logger.Debug("Processed notification, no response needed.", "method", request.Method)
		return nil, nil // Success, but no response bytes.
	}

	// Construct the success response object for requests with IDs.
	responseObj := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"` // Result is expected to be already marshalled JSON.
	}{
		JSONRPC: "2.0",
		ID:      request.ID,
		Result:  resultBytes, // Assign the raw JSON bytes from the handler.
	}

	// Marshal the final success response.
	respBytes, marshalErr := json.Marshal(responseObj)
	if marshalErr != nil {
		// This is an internal server error.
		return nil, errors.Wrap(marshalErr, "internal error: failed to marshal success response")
	}

	return respBytes, nil // Return marshalled success response bytes.
}

// handleTransportReadError decides if a read error is terminal.
func (s *Server) handleTransportReadError(readErr error) error {
	var transportErr *transport.Error
	isEOF := errors.Is(readErr, io.EOF)
	// Check for specific closed error code.
	isClosedCode := errors.As(readErr, &transportErr) && transportErr.Code == transport.ErrTransportClosed
	// Also check for the closed error type for robustness.
	isClosedType := transport.IsClosedError(readErr)

	isContextDone := errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded)

	if isEOF || isClosedCode || isClosedType || isContextDone {
		// These are expected ways the connection can terminate, return the error to stop the loop.
		return readErr // Return the original error.
	}

	// Other read errors (e.g., temporary network glitch, invalid NDJSON framing handled by transport).
	// are logged but might not be terminal. Return nil to allow the loop to continue.
	s.logger.Error("Non-terminal error reading message from transport", "error", fmt.Sprintf("%+v", readErr))
	return nil // Indicate loop should continue.
}

// handleProcessingError logs processing errors and attempts to send a JSON-RPC error response.
// Returns an error only if writing the error response fails.
func (s *Server) handleProcessingError(ctx context.Context, msgBytes []byte, method, id string, handleErr error) error {
	s.logger.Warn("Error processing message via handler.",
		"method", method,
		"requestID", id,
		"error", fmt.Sprintf("%+v", handleErr)) // Log original error with stack trace.

	// Create the JSON-RPC error response bytes using the error mapping logic.
	errRespBytes, creationErr := s.createErrorResponse(msgBytes, handleErr) // createErrorResponse is in mcp_server_error_handling.go.
	if creationErr != nil {
		// This is critical - failed even to create the error response structure.
		s.logger.Error("CRITICAL: Failed to create error response.",
			"creationError", fmt.Sprintf("%+v", creationErr),
			"originalHandlingError", fmt.Sprintf("%+v", handleErr))
		// Return this marshalling error, as we can't send anything sensible back.
		return creationErr
	}

	// Attempt to write the created error response.
	writeErr := s.writeResponse(ctx, errRespBytes, method, id)
	if writeErr != nil {
		// Log the failure to write the error response.
		s.logger.Error("Failed to write error response.",
			"method", method,
			"requestID", id,
			"writeError", fmt.Sprintf("%+v", writeErr),
			"originalHandlingError", fmt.Sprintf("%+v", handleErr))
		// Return the write error, as it might indicate a terminal transport issue.
		return writeErr
	}

	// Successfully sent the error response, return nil so the server loop continues.
	return nil
}

// writeResponse sends response bytes through the transport.
func (s *Server) writeResponse(ctx context.Context, respBytes []byte, method, id string) error {
	if s.transport == nil {
		s.logger.Error("Attempted to write response but transport is nil.", "method", method, "requestID", id)
		return errors.New("cannot write response: transport is nil")
	}
	if writeErr := s.transport.WriteMessage(ctx, respBytes); writeErr != nil {
		// Don't log the full response bytes here for brevity/security.
		s.logger.Error("Failed to write response.",
			"method", method,
			"requestID", id,
			"responseSize", len(respBytes),
			"error", fmt.Sprintf("%+v", writeErr))
		return writeErr // Propagate the error.
	}
	s.logger.Debug("Successfully wrote response.", "method", method, "requestID", id, "responseSize", len(respBytes))
	return nil
}

// isTerminalError checks if an error signifies the end of the connection.
func (s *Server) isTerminalError(err error) bool {
	if err == nil {
		return false
	}
	// Check standard context errors.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Check for EOF.
	if errors.Is(err, io.EOF) {
		return true
	}
	// Check for specific transport closed/timeout errors.
	var transportErr *transport.Error
	if errors.As(err, &transportErr) {
		return transportErr.Code == transport.ErrTransportClosed ||
			transportErr.Code == transport.ErrWriteTimeout ||
			transportErr.Code == transport.ErrReadTimeout // Read timeout might also be terminal.
	}
	// Check using the transport helper function as well.
	if transport.IsClosedError(err) {
		return true
	}

	// Add other conditions specific to your application if needed.

	return false // Assume other errors are potentially recoverable.
}

// extractMessageInfo attempts to get method name and ID from raw message bytes for logging/context.
func (s *Server) extractMessageInfo(msgBytes []byte) (method string, id string) {
	method = ""
	id = "unknown" // Default ID if parsing fails or not present.

	// Use a simple struct to only parse needed fields.
	var parsedInfo struct {
		Method *string         `json:"method"` // Pointer to distinguish missing from empty string.
		ID     json.RawMessage `json:"id"`     // Keep ID raw.
	}

	// Unmarshal partially. Ignore error as this is best-effort for logging.
	// If JSON is invalid, method remains "" and id remains "unknown".
	_ = json.Unmarshal(msgBytes, &parsedInfo)

	if parsedInfo.Method != nil {
		method = *parsedInfo.Method
	}
	if parsedInfo.ID != nil && string(parsedInfo.ID) != "null" {
		// Represent ID as its raw JSON string representation (e.g., "123", "\"req-abc\"").
		id = string(parsedInfo.ID)
	} else if parsedInfo.ID != nil && string(parsedInfo.ID) == "null" {
		id = "null" // Explicitly null ID.
	}
	// If ID is missing, it remains "unknown".

	return method, id
}
