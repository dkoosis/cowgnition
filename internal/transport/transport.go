// Package transport defines interfaces and implementations for sending and receiving MCP messages.
// It handles the low-level communication details, abstracting away specific mechanisms like
// stdio or network sockets, and ensures messages adhere to basic framing and size constraints.
// file: internal/transport/transport.go.
package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging" // Added missing import.
)

// MaxMessageSize defines the maximum allowed size for a single JSON-RPC message in bytes (1MB).
// This helps prevent memory exhaustion from excessively large messages.
const MaxMessageSize = 1024 * 1024 // 1MB.

// Transport defines the interface for sending and receiving JSON-RPC messages.
// Implementations must handle the underlying communication channel (e.g., stdio, network)
// and ensure message integrity and framing. Implementations must be concurrency-safe.
type Transport interface {
	// ReadMessage reads a single, complete JSON-RPC message from the transport.
	// It returns the raw message bytes or an error if reading fails (e.g., connection closed, timeout).
	// The context allows for cancellation of potentially long-running reads.
	ReadMessage(ctx context.Context) ([]byte, error)

	// WriteMessage sends a single JSON-RPC message over the transport.
	// It takes the raw message bytes and returns an error if writing fails.
	// The context allows for cancellation of potentially long-running writes.
	WriteMessage(ctx context.Context, message []byte) error

	// Close shuts down the transport, releasing any underlying resources (e.g., closing connections).
	// Any blocked Read or Write operations should be unblocked and return errors.
	Close() error
}

// MessageHandler defines the signature for a function that processes a received MCP message.
// It takes the message bytes and returns a response message or an error.
type MessageHandler func(ctx context.Context, message []byte) ([]byte, error)

// ErrorHandler defines the signature for functions that handle asynchronous transport errors.
// This allows for custom error logging or recovery strategies outside the main read/write flow.
type ErrorHandler func(ctx context.Context, err error)

// DefaultErrorHandler provides a basic no-op error handling implementation.
// Used when no specific error handler is configured.
func DefaultErrorHandler(_ context.Context, _ error) {
	// Default implementation does nothing; implementations should replace with
	// appropriate logging, metrics, etc.
}

// <<< FIX: Replaced minInt with calculatePreview for gosec/linting compatibility >>>.
// calculatePreview generates a short, safe preview of byte data for logging.
func calculatePreview(data []byte) string {
	const maxPreviewLen = 100 // Using 100 as the length for preview.
	if len(data) > maxPreviewLen {
		// Consider replacing control characters for cleaner previews.
		previewBytes := bytes.Map(func(r rune) rune {
			if r < 32 || r == 127 {
				return '.'
			}
			return r
		}, data[:maxPreviewLen])
		return string(previewBytes) + "..."
	}
	// Consider replacing control characters here too.
	previewBytes := bytes.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return '.'
		}
		return r
	}, data)
	return string(previewBytes)
}

// ValidateMessage performs basic validation on a JSON-RPC message bytes.
// It checks for valid JSON syntax and the presence and correctness of core
// JSON-RPC 2.0 fields (`jsonrpc`, `id`, `method`, `params`, `result`, `error`),
// enforcing structural rules like mutual exclusivity of `result` and `error`.
// file: internal/transport/transport.go.
// nolint:gocyclo.
func ValidateMessage(message []byte) error {
	// First check if it's valid JSON.
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		return NewParseError(message, err)
	}

	// Check for required "jsonrpc" field with value "2.0".
	version, ok := msg["jsonrpc"]
	if !ok {
		return NewError(
			ErrInvalidMessage,
			"missing 'jsonrpc' field",
			nil,
		).WithContext("messagePreview", calculatePreview(message))
	}

	if version != "2.0" {
		return NewError(
			ErrInvalidMessage,
			"unsupported JSON-RPC version",
			nil,
		).WithContext("version", version).
			WithContext("messagePreview", calculatePreview(message))
	}

	// Check if it's a batch request/response (array of messages).
	// Basic validation assumes single message; batch validation would require iterating.
	// This check relies on a hypothetical internal marker; proper batch detection needs JSON array check.
	if _, isArray := msg["_isBatch"]; isArray { // Note: `_isBatch` is not standard JSON-RPC.
		// Proper batch validation would parse as []interface{} and validate each element.
		// For now, we assume single messages for this basic validation.
		return nil // Skipping detailed batch validation here.
	}

	// Determine message type (Request, Notification, Response) and validate accordingly.
	hasMethod := false
	if method, exists := msg["method"]; exists {
		hasMethod = true
		// Method must be a non-empty string and not start with "rpc.".
		methodStr, ok := method.(string)
		if !ok || methodStr == "" {
			return NewError(ErrInvalidMessage, "method must be a non-empty string", nil).
				WithContext("messagePreview", calculatePreview(message))
		}
		if len(methodStr) >= 4 && methodStr[:4] == "rpc." {
			return NewError(ErrInvalidMessage, "method names starting with 'rpc.' are reserved", nil).
				WithContext("method", methodStr).
				WithContext("messagePreview", calculatePreview(message))
		}
	}

	hasID := false
	if id, exists := msg["id"]; exists {
		hasID = true
		// ID must be a string, number, or null. Objects/arrays are invalid.
		switch id.(type) {
		case string, float64, nil, json.Number:
			// Valid ID types.
		default:
			return NewError(ErrInvalidMessage, "invalid JSON-RPC ID type", nil).
				WithContext("idType", fmt.Sprintf("%T", id)).
				WithContext("messagePreview", calculatePreview(message))
		}
	}

	hasResult := false
	if _, exists := msg["result"]; exists {
		hasResult = true
	}

	hasError := false
	if errorObj, exists := msg["error"]; exists {
		hasError = true
		// If error is present, it must be an object with code (number) and message (string).
		errorMap, ok := errorObj.(map[string]interface{})
		if !ok {
			return NewError(ErrInvalidMessage, "JSON-RPC error field must be an object", nil).
				WithContext("messagePreview", calculatePreview(message))
		}
		code, codeExists := errorMap["code"]
		messageText, messageExists := errorMap["message"]
		if !codeExists || !messageExists {
			return NewError(ErrInvalidMessage, "JSON-RPC error object must contain 'code' and 'message'", nil).
				WithContext("messagePreview", calculatePreview(message))
		}
		switch code.(type) {
		case float64, json.Number: // Valid numeric types.
		default:
			return NewError(ErrInvalidMessage, "JSON-RPC error code must be a number", nil).
				WithContext("codeType", fmt.Sprintf("%T", code)).
				WithContext("messagePreview", calculatePreview(message))
		}
		if _, ok := messageText.(string); !ok {
			return NewError(ErrInvalidMessage, "JSON-RPC error message must be a string", nil).
				WithContext("messageType", fmt.Sprintf("%T", messageText)).
				WithContext("messagePreview", calculatePreview(message))
		}
	}

	// --- Structural Rules ---.
	if hasMethod { // Request or Notification.
		if hasResult || hasError {
			return NewError(ErrInvalidMessage, "request/notification cannot contain 'result' or 'error'", nil).
				WithContext("messagePreview", calculatePreview(message))
		}
		// Params validation (must be object or array if present).
		if params, exists := msg["params"]; exists {
			switch params.(type) {
			case map[string]interface{}, []interface{}, nil: // Allow null params too.
			default:
				return NewError(ErrInvalidMessage, "params must be object, array, or null", nil).
					WithContext("paramsType", fmt.Sprintf("%T", params)).
					WithContext("messagePreview", calculatePreview(message))
			}
		}
		// Notification specific: must NOT have ID (or ID must be null, handled by hasID logic).
		// Request specific: must have ID (checked by hasID logic).
	} else { // Response (Success or Error).
		if !hasID { // Note: RTM seems to allow error responses without ID, but spec says MUST have ID. We allow missing ID *only* if it's an error response.
			if !hasError { // Success responses MUST have an ID.
				return NewError(ErrInvalidMessage, "response message must contain 'id'", nil).
					WithContext("messagePreview", calculatePreview(message))
			}
		}
		if !hasResult && !hasError {
			return NewError(ErrInvalidMessage, "response message must contain 'result' or 'error'", nil).
				WithContext("messagePreview", calculatePreview(message))
		}
		if hasResult && hasError {
			return NewError(ErrInvalidMessage, "response message cannot contain both 'result' and 'error'", nil).
				WithContext("messagePreview", calculatePreview(message))
		}
	}

	return nil
}

// NDJSONTransport implements the Transport interface for newline-delimited JSON streams.
// It reads and writes complete JSON objects separated by newline characters, typically
// used for communication over standard input/output (stdio).
type NDJSONTransport struct {
	reader    *bufio.Reader
	writer    io.Writer
	closer    io.Closer
	logger    logging.Logger // For internal logging.
	writeLock sync.Mutex     // Ensures atomic writes of complete messages.
	closed    bool
	closeLock sync.RWMutex
}

// NewNDJSONTransport creates a new transport layer that reads/writes NDJSON messages
// from the provided reader and writer, using the closer to shut down the underlying stream.
// It requires a logger for internal operations.
func NewNDJSONTransport(reader io.Reader, writer io.Writer, closer io.Closer, logger logging.Logger) Transport {
	if logger == nil {
		logger = logging.GetNoopLogger() // Use no-op logger if none provided.
	}
	return &NDJSONTransport{
		reader: bufio.NewReader(reader),
		writer: writer,
		closer: closer,
		logger: logger.WithField("component", "ndjson_transport"),
	}
}

// ReadMessage implements Transport.ReadMessage for NDJSON.
// It reads bytes until a newline character is encountered, validating the resulting
// line as a single, complete JSON message according to JSON-RPC 2.0 structure.
func (t *NDJSONTransport) ReadMessage(ctx context.Context) ([]byte, error) {
	// Check if the transport is closed.
	t.closeLock.RLock()
	if t.closed {
		t.closeLock.RUnlock()
		return nil, NewClosedError("read")
	}
	t.closeLock.RUnlock()

	// Create a channel for the result to enable cancellation.
	type readResult struct {
		data []byte
		err  error
	}
	resultCh := make(chan readResult, 1)

	// Perform the blocking read in a separate goroutine.
	go func() {
		var lineBytes []byte
		var readErr error
		// Use ReadBytes for simplicity and potential memory efficiency over ReadLine loop.
		// Handles messages larger than the buffer size.
		lineBytes, readErr = t.reader.ReadBytes('\n')

		if readErr != nil {
			// Handle common read errors.
			if readErr == io.EOF {
				resultCh <- readResult{nil, NewError(ErrTransportClosed, "connection closed by peer", io.EOF)}
			} else {
				resultCh <- readResult{nil, NewError(ErrGeneric, "failed to read message line", readErr)}
			}
			return
		}

		// Trim trailing newline characters (\n or \r\n).
		message := bytes.TrimRight(lineBytes, "\r\n")

		// Check for empty lines after trimming.
		if len(message) == 0 {
			resultCh <- readResult{nil, NewError(ErrInvalidMessage, "received empty message line", nil)}
			return
		}

		// Check message size limit.
		if len(message) > MaxMessageSize {
			fragment := message[:minInt(len(message), 100)] // Use minInt directly.
			resultCh <- readResult{nil, NewMessageSizeError(len(message), MaxMessageSize, fragment)}
			return
		}

		t.logger.Debug("Received raw message line.", "size", len(message), "contentPreview", calculatePreview(message))

		// Validate the basic JSON-RPC structure of the message.
		if err := ValidateMessage(message); err != nil {
			t.logger.Warn("Invalid message received.", "validationError", err, "rawMessage", string(message))
			resultCh <- readResult{nil, err} // Return the specific validation error.
			return
		}

		resultCh <- readResult{message, nil}
	}()

	// Wait for the read result or context cancellation.
	select {
	case <-ctx.Done():
		t.logger.Warn("Context cancelled while reading message.", "error", ctx.Err())
		// Return a specific timeout/cancellation error.
		return nil, NewTimeoutError("read", ctx.Err())
	case result := <-resultCh:
		if result.err != nil {
			// Log errors (excluding expected EOF/closed errors).
			var transportErr *Error
			if !errors.As(result.err, &transportErr) || (transportErr.Code != ErrTransportClosed) {
				t.logger.Error("Error processing read message.", "error", fmt.Sprintf("%+v", result.err))
			}
		}
		return result.data, result.err
	}
}

// WriteMessage implements Transport.WriteMessage for NDJSON.
// It validates the message, appends a newline character, and writes it atomically.
func (t *NDJSONTransport) WriteMessage(ctx context.Context, message []byte) error {
	// Check if the transport is closed.
	t.closeLock.RLock()
	if t.closed {
		t.closeLock.RUnlock()
		return NewClosedError("write")
	}
	t.closeLock.RUnlock()

	// Validate the message conforms to basic JSON-RPC structure first.
	// Although messages *should* be valid by this point, this is a safety check.
	if err := ValidateMessage(message); err != nil {
		t.logger.Error("Attempted to write invalid message.", "validationError", err, "messagePreview", calculatePreview(message))
		return err // Return validation error.
	}

	// Check message size limit.
	if len(message) > MaxMessageSize {
		fragment := message[:minInt(len(message), 100)] // Use minInt directly.
		return NewMessageSizeError(len(message), MaxMessageSize, fragment)
	}

	// Create a channel for the result to enable cancellation.
	resultCh := make(chan error, 1)

	// Ensure atomic write using mutex.
	t.writeLock.Lock()
	defer t.writeLock.Unlock()

	// Perform the write in a goroutine.
	go func() {
		// Prepare buffer with message and trailing newline.
		buf := make([]byte, len(message)+1)
		copy(buf, message)
		buf[len(message)] = '\n'

		t.logger.Debug("Writing NDJSON message.", "size", len(buf), "contentPreview", calculatePreview(message))

		// Write the entire buffer.
		n, err := t.writer.Write(buf)
		if err == nil && n < len(buf) {
			err = io.ErrShortWrite // Treat partial writes as errors.
		}
		resultCh <- err // Send error or nil.
	}()

	// Wait for write completion or context cancellation.
	select {
	case <-ctx.Done():
		t.logger.Warn("Context cancelled while writing message.", "error", ctx.Err())
		return NewTimeoutError("write", ctx.Err()) // Return timeout/cancellation error.
	case err := <-resultCh:
		if err != nil {
			t.logger.Error("Failed to write message.", "error", fmt.Sprintf("%+v", err))
			// Wrap the underlying write error.
			return NewError(ErrGeneric, "failed to write message", err)
		}
		return nil // Success.
	}
}

// Close implements Transport.Close for NDJSONTransport.
// It marks the transport as closed and closes the underlying closer if available.
func (t *NDJSONTransport) Close() error {
	t.closeLock.Lock()
	defer t.closeLock.Unlock()

	if t.closed {
		return nil // Already closed.
	}

	t.logger.Info("Closing NDJSON transport.")
	t.closed = true // Mark as closed.

	// Close the underlying stream (e.g., stdin/stdout, file, network connection).
	if t.closer != nil {
		if err := t.closer.Close(); err != nil {
			t.logger.Error("Error closing underlying transport stream.", "error", err)
			// Wrap the underlying close error.
			return NewError(ErrTransportClosed, "failed to close underlying transport stream", err)
		}
		t.logger.Debug("Underlying closer closed successfully.")
	}

	return nil
}

// minInt returns the smaller of x or y. Used for safe slicing.
// <<< NOTE: This function is now unused because calculatePreview is used instead, but kept for reference or potential future use >>>.
// nolint:unused.
func minInt(x, y int) int {
	if x < y {
		return x
	}
	return y
}
