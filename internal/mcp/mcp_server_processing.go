// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// MODIFIED: handleProcessingError updated for MCP compliance (id: 0 instead of null).
package mcp

// file: internal/mcp/mcp_server_processing.go

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/cockroachdb/errors"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
	"github.com/dkoosis/cowgnition/internal/transport"
	// Ensure mcp_server_error_handling types/functions are accessible if needed.
	// mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors".
)

// serverProcessing handles the main server loop, reading messages and dispatching them.
func (s *Server) serverProcessing(ctx context.Context, handlerFunc mcptypes.MessageHandler) error {
	s.logger.Debug(">>> serverProcessing: Entering loop...") // <<< ADDED LOG
	s.logger.Info("Server processing loop started.")         // Keep this info log
	if handlerFunc == nil {
		s.logger.Error(">>> serverProcessing: FATAL - nil handlerFunc provided.") // <<< ADDED LOG
		return errors.New("serve called with nil handler function")
	}
	if s.transport == nil {
		s.logger.Error(">>> serverProcessing: FATAL - server transport is nil.") // <<< ADDED LOG
		return errors.New("serve called but server transport is nil")
	}

	for {
		s.logger.Debug(">>> serverProcessing: Top of loop, checking context.") // <<< ADDED LOG
		select {
		case <-ctx.Done():
			s.logger.Info("Context canceled, stopping server loop.")
			s.logger.Debug(">>> serverProcessing: Context canceled, returning.") // <<< ADDED LOG
			return ctx.Err()
		default:
			// processNextMessage reads, handles, and responds to a single message.
			// It returns terminal errors that should stop the loop.
			s.logger.Debug(">>> serverProcessing: Calling processNextMessage...") // <<< ADDED LOG
			if err := s.processNextMessage(ctx, handlerFunc); err != nil {
				s.logger.Debug(">>> serverProcessing: processNextMessage returned error.", "error", err) // <<< ADDED LOG
				// isTerminalError checks for EOF, closed transport, context cancellation etc.
				if s.isTerminalError(err) {
					s.logger.Info("Terminal error received, stopping server loop.", "reason", err)
					return err // Propagate terminal error to stop serve.
				}
				// Log non-terminal errors but continue the loop.
				s.logger.Error("Non-terminal error processing message.", "error", fmt.Sprintf("%+v", err))
				// Optionally, implement retry logic or specific error handling here.
			} else {
				s.logger.Debug(">>> serverProcessing: processNextMessage completed successfully.") // <<< ADDED LOG
			}
		}
	}
}

// processNextMessage handles reading, processing, and responding to a single message.
// It returns non-nil error only for terminal conditions. Other processing errors are handled internally.
// by sending a JSON-RPC error response.
func (s *Server) processNextMessage(ctx context.Context, handlerFunc mcptypes.MessageHandler) error {
	s.logger.Debug(">>> processNextMessage: Reading from transport...") // <<< ADDED LOG
	// 1. Read Message.
	msgBytes, readErr := s.transport.ReadMessage(ctx)
	if readErr != nil {
		s.logger.Debug(">>> processNextMessage: ReadMessage returned error.", "error", readErr) // <<< ADDED LOG
		// Let handleTransportReadError decide if it's terminal.
		return s.handleTransportReadError(readErr)
	}
	s.logger.Debug(">>> processNextMessage: ReadMessage successful.", "bytesRead", len(msgBytes)) // <<< ADDED LOG

	// 2. Extract Info for Logging/Context (Best Effort).
	method, idStr := s.extractMessageInfo(msgBytes) // Note: idStr here is just for logging
	// --- REMOVED ctxWithState line ---

	// 3. Handle Message via Middleware Chain / Final Handler (which should be s.handleMessage).
	// --- MODIFIED: Pass original ctx instead of ctxWithState ---
	s.logger.Debug(">>> processNextMessage: Calling final handler...", "method", method, "id", idStr) // <<< ADDED LOG
	respBytes, handleErr := handlerFunc(ctx, msgBytes)
	s.logger.Debug(">>> processNextMessage: Final handler returned.", "method", method, "id", idStr, "error", handleErr, "respBytesLen", len(respBytes)) // <<< ADDED LOG

	// 4. Handle Processing Error (if any).
	if handleErr != nil {
		s.logger.Debug(">>> processNextMessage: Handling processing error...", "method", method, "id", idStr) // <<< ADDED LOG
		// handleProcessingError logs the error and attempts to create/write a JSON-RPC error response.
		// It returns an error only if writing the error response fails.
		// Pass method and idStr purely for logging context within handleProcessingError.
		writeErr := s.handleProcessingError(ctx, msgBytes, method, idStr, handleErr)
		if writeErr != nil {
			// If we can't even write the error response, it might be a terminal transport issue.
			return errors.Wrap(writeErr, "failed to write error response after processing error")
		}
		// If handleProcessingError succeeded in sending an error response,
		// we don't return an error here, allowing the loop to continue.
		s.logger.Debug(">>> processNextMessage: Processing error handled (error response sent).", "method", method, "id", idStr) // <<< ADDED LOG
		return nil
	}

	// 5. State Update for Initialize Success is handled within handleMessage now.

	// 6. Write Successful Response (if one was generated).
	//    Notifications will have respBytes == nil here.
	if respBytes != nil {
		s.logger.Debug(">>> processNextMessage: Writing successful response...", "method", method, "id", idStr, "respBytesLen", len(respBytes)) // <<< ADDED LOG
		// Use the idStr extracted earlier for logging write operations.
		if writeErr := s.writeResponse(ctx, respBytes, method, idStr); writeErr != nil {
			// If writing the success response fails, return the error.
			return errors.Wrap(writeErr, "failed to write successful response")
		}
	} else {
		// Log if it was a request that resulted in no response bytes (shouldn't happen unless it was a notification).
		// Check if idStr indicates it was likely a request (not null or unknown).
		if idStr != "null" && idStr != "unknown" {
			s.logger.Warn("Handler returned nil response bytes for a non-notification request.", "method", method, "id", idStr)
		} else {
			s.logger.Debug(">>> processNextMessage: Notification processed, no response to write.", "method", method) // <<< ADDED LOG
		}
	}
	// Successfully processed the message.
	s.logger.Debug(">>> processNextMessage: Finished processing message successfully.", "method", method, "id", idStr) // <<< ADDED LOG
	return nil
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
		s.logger.Debug(">>> handleTransportReadError: Terminal read error detected.", "error", readErr) // <<< ADDED LOG
		return readErr                                                                                  // Return the original error.
	}

	// Other read errors (e.g., temporary network glitch, invalid NDJSON framing handled by transport).
	// are logged but might not be terminal. Return nil to allow the loop to continue.
	s.logger.Error("Non-terminal error reading message from transport.", "error", fmt.Sprintf("%+v", readErr))
	s.logger.Debug(">>> handleTransportReadError: Non-terminal read error, continuing loop.") // <<< ADDED LOG
	return nil                                                                                // Indicate loop should continue.
}

// handleProcessingError logs processing errors and attempts to send a JSON-RPC error response.
// Returns an error only if writing the error response fails.
// MODIFIED: Now enforces id: 0 for null/missing original IDs, per MCP spec.
func (s *Server) handleProcessingError(ctx context.Context, msgBytes []byte, method, idForLog string, handleErr error) error {
	// Log using the potentially unreliable idForLog extracted earlier.
	s.logger.Warn("Error processing message via handler.",
		"method", method,
		"requestID_log", idForLog, // Use distinct name for clarity
		"error", fmt.Sprintf("%+v", handleErr)) // Log original error with stack trace

	// ---> FIX: Determine the ID to use *in the response*, substituting 0 for null <---
	// Extract the original request ID reliably from the raw message bytes here.
	reqIDFromMsg := extractRequestID(s.logger, msgBytes) // extractRequestID is in mcp_server_error_handling.go

	// MCP Spec forbids null IDs. Substitute 0 if the ID couldn't be determined or was explicitly null.
	responseID := reqIDFromMsg
	if responseID == nil || string(responseID) == "null" {
		s.logger.Debug("Original request ID was null or undetectable, substituting ID 0 for MCP compliance in error response.")
		responseID = json.RawMessage("0") // Use "0" (number zero)
	}
	// ---> END FIX <---

	// Create the JSON-RPC error response bytes using the error mapping logic.
	// Ensure createErrorResponse uses the 'responseID' determined above.
	s.logger.Debug(">>> handleProcessingError: Creating error response.", "responseID", string(responseID)) // <<< ADDED LOG
	errRespBytes, creationErr := s.createErrorResponse(msgBytes, handleErr, responseID)                     // Pass the guaranteed non-null responseID
	if creationErr != nil {
		// This is critical - failed even to create the error response structure.
		s.logger.Error("CRITICAL: Failed to create error response.",
			"creationError", fmt.Sprintf("%+v", creationErr),
			"originalHandlingError", fmt.Sprintf("%+v", handleErr))
		// Return this marshalling error, as we can't send anything sensible back.
		return creationErr
	}
	s.logger.Debug(">>> handleProcessingError: Error response created.", "responseID", string(responseID), "bytesLen", len(errRespBytes)) // <<< ADDED LOG

	// Attempt to write the created error response.
	// Log using the ID that was actually *sent* in the response.
	s.logger.Debug(">>> handleProcessingError: Writing error response.", "responseID", string(responseID)) // <<< ADDED LOG
	writeErr := s.writeResponse(ctx, errRespBytes, method, string(responseID))
	if writeErr != nil {
		// Log the failure to write the error response.
		s.logger.Error("Failed to write error response.",
			"method", method,
			"responseIDUsed", string(responseID),
			"writeError", fmt.Sprintf("%+v", writeErr),
			"originalHandlingError", fmt.Sprintf("%+v", handleErr))
		// Return the write error, as it might indicate a terminal transport issue.
		return writeErr
	}

	// Successfully sent the error response, return nil so the server loop continues.
	s.logger.Debug(">>> handleProcessingError: Successfully wrote error response.", "responseID", string(responseID)) // <<< ADDED LOG
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
			"requestID", id, // Use the general 'id' passed for logging context
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
// Returns id as "unknown" if missing, "null" if json null, or the raw json string otherwise.
func (s *Server) extractMessageInfo(msgBytes []byte) (method string, id string) {
	method = ""
	id = "unknown" // Default ID if parsing fails or not present.

	// Use a simple struct to only parse needed fields.
	var parsedInfo struct {
		Method *string         `json:"method"` // Pointer to distinguish missing from empty string.
		ID     json.RawMessage `json:"id"`     // Keep ID raw.
	}

	// Unmarshal partially. Ignore error as this is best-effort for logging.
	_ = json.Unmarshal(msgBytes, &parsedInfo)

	if parsedInfo.Method != nil {
		method = *parsedInfo.Method
	}

	// Determine ID string representation for logging
	if parsedInfo.ID != nil {
		idStr := string(parsedInfo.ID)
		if idStr == "null" {
			id = "null" // Explicitly JSON null ID
		} else {
			// Use the raw JSON value (e.g., "123", "\"req-abc\"")
			id = idStr
		}
	}
	// If parsedInfo.ID was nil (field missing), id remains "unknown".

	return method, id
}

// Note: Ensure createErrorResponse function in mcp_server_error_handling.go
// now accepts the determined responseID (json.RawMessage) as an argument
// and uses that when constructing the JSONRPCErrorContainer.
