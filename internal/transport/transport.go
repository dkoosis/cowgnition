// Package transport defines interfaces and implementations for sending and receiving MCP messages.
package transport

// file: internal/transport/transport.go

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/cockroachdb/errors"
)

// MaxMessageSize defines the maximum allowed size for a single JSON-RPC message in bytes.
// This helps prevent memory exhaustion attacks.
const MaxMessageSize = 1024 * 1024 // 1MB

// Transport defines the interface for sending and receiving JSON-RPC messages.
// Implementations must be concurrency-safe.
type Transport interface {
	// ReadMessage reads a single JSON-RPC message from the transport.
	// It returns the raw message bytes, or an error if reading fails.
	// The context allows for cancellation of long-running reads.
	ReadMessage(ctx context.Context) ([]byte, error)

	// WriteMessage sends a single JSON-RPC message over the transport.
	// It takes raw message bytes and returns an error if writing fails.
	// The context allows for cancellation of long-running writes.
	WriteMessage(ctx context.Context, message []byte) error

	// Close shuts down the transport, closing any underlying connections.
	// Any blocked Read or Write operations will be unblocked and return errors.
	Close() error
}

// MessageHandler defines the signature for a function that processes JSON-RPC messages.
// It receives the raw message bytes and returns a response message or error.
type MessageHandler func(ctx context.Context, message []byte) ([]byte, error)

// ErrorHandler defines the signature for functions that handle transport errors.
// It allows customized error handling strategies.
type ErrorHandler func(ctx context.Context, err error)

// DefaultErrorHandler provides a basic error handling implementation.
func DefaultErrorHandler(_ context.Context, err error) {
	// Default implementation does nothing; implementations should replace with
	// appropriate logging, metrics, etc.
}

// ValidateMessage performs basic validation on a JSON-RPC message.
// It ensures the message has the required fields for a JSON-RPC 2.0 message.
// ValidateMessage performs thorough validation on a JSON-RPC message according to
// the JSON-RPC 2.0 specification (https://www.jsonrpc.org/specification).
// It ensures the message has all required fields and follows the correct format.
// nolint:gocyclo
func ValidateMessage(message []byte) error {
	// First check if it's valid JSON
	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		return NewParseError(message, err)
	}

	// Check for required "jsonrpc" field with value "2.0"
	version, ok := msg["jsonrpc"]
	if !ok {
		return NewError(
			ErrInvalidMessage,
			"missing 'jsonrpc' field",
			nil,
		).WithContext("messagePreview", string(message[:min(len(message), 100)]))
	}

	if version != "2.0" {
		return NewError(
			ErrInvalidMessage,
			"unsupported JSON-RPC version",
			nil,
		).WithContext("version", version).
			WithContext("messagePreview", string(message[:min(len(message), 100)]))
	}

	// Check if it's a batch request/response (array of messages)
	if _, isArray := msg["_isBatch"]; isArray {
		// For batch requests/responses, each individual message
		// should be validated separately
		return nil
	}

	// Determine message type and validate accordingly
	hasMethod := false
	if method, exists := msg["method"]; exists {
		hasMethod = true

		// Method must be a string
		methodStr, ok := method.(string)
		if !ok {
			return NewError(
				ErrInvalidMessage,
				"method must be a string",
				nil,
			).WithContext("method", method).
				WithContext("messagePreview", string(message[:min(len(message), 100)]))
		}

		// Method cannot be empty
		if methodStr == "" {
			return NewError(
				ErrInvalidMessage,
				"method cannot be empty",
				nil,
			).WithContext("messagePreview", string(message[:min(len(message), 100)]))
		}

		// Reserved method names starting with "rpc." are for internal use
		if len(methodStr) >= 4 && methodStr[:4] == "rpc." {
			return NewError(
				ErrInvalidMessage,
				"method names starting with 'rpc.' are reserved for internal use",
				nil,
			).WithContext("method", methodStr).
				WithContext("messagePreview", string(message[:min(len(message), 100)]))
		}
	}

	hasID := false
	if id, exists := msg["id"]; exists {
		hasID = true

		// ID must be a string, number, or null
		switch id.(type) {
		case string, float64, nil, json.Number:
			// Valid ID types
		default:
			return NewError(
				ErrInvalidMessage,
				"invalid request ID type",
				nil,
			).WithContext("idType", fmt.Sprintf("%T", id)).
				WithContext("messagePreview", string(message[:min(len(message), 100)]))
		}
	}

	// Based on the combination of method and id, determine message type
	if hasMethod {
		if hasID {
			// Request: check for params
			if params, exists := msg["params"]; exists {
				// Params must be an object or array
				switch params.(type) {
				case map[string]interface{}, []interface{}:
					// Valid params types
				default:
					return NewError(
						ErrInvalidMessage,
						"params must be an object or array",
						nil,
					).WithContext("paramsType", fmt.Sprintf("%T", params)).
						WithContext("messagePreview", string(message[:min(len(message), 100)]))
				}
			}

			// Requests shouldn't have result or error fields
			if _, hasResult := msg["result"]; hasResult {
				return NewError(
					ErrInvalidMessage,
					"request message cannot contain 'result' field",
					nil,
				).WithContext("messagePreview", string(message[:min(len(message), 100)]))
			}

			if _, hasError := msg["error"]; hasError {
				return NewError(
					ErrInvalidMessage,
					"request message cannot contain 'error' field",
					nil,
				).WithContext("messagePreview", string(message[:min(len(message), 100)]))
			}
		} else {
			// Notification: similar to request but no id field
			if params, exists := msg["params"]; exists {
				// Params must be an object or array
				switch params.(type) {
				case map[string]interface{}, []interface{}:
					// Valid params types
				default:
					return NewError(
						ErrInvalidMessage,
						"params must be an object or array",
						nil,
					).WithContext("paramsType", fmt.Sprintf("%T", params)).
						WithContext("messagePreview", string(message[:min(len(message), 100)]))
				}
			}

			// Notifications shouldn't have result or error fields
			if _, hasResult := msg["result"]; hasResult {
				return NewError(
					ErrInvalidMessage,
					"notification message cannot contain 'result' field",
					nil,
				).WithContext("messagePreview", string(message[:min(len(message), 100)]))
			}

			if _, hasError := msg["error"]; hasError {
				return NewError(
					ErrInvalidMessage,
					"notification message cannot contain 'error' field",
					nil,
				).WithContext("messagePreview", string(message[:min(len(message), 100)]))
			}
		}
	} else {
		// Response: must have id and either result or error
		if !hasID {
			return NewError(
				ErrInvalidMessage,
				"response message must contain 'id' field",
				nil,
			).WithContext("messagePreview", string(message[:min(len(message), 100)]))
		}

		hasResult := false
		if _, exists := msg["result"]; exists {
			hasResult = true
		}

		hasError := false
		if errorObj, exists := msg["error"]; exists {
			hasError = true

			// If error is present, it must be an object with code and message
			errorMap, ok := errorObj.(map[string]interface{})
			if !ok {
				return NewError(
					ErrInvalidMessage,
					"error must be an object",
					nil,
				).WithContext("errorType", fmt.Sprintf("%T", errorObj)).
					WithContext("messagePreview", string(message[:min(len(message), 100)]))
			}

			// Error must have code (number) and message (string)
			if code, exists := errorMap["code"]; !exists {
				return NewError(
					ErrInvalidMessage,
					"error object must contain 'code' field",
					nil,
				).WithContext("messagePreview", string(message[:min(len(message), 100)]))
			} else {
				// Code must be a number
				switch code.(type) {
				case float64, json.Number:
					// Valid code types
				default:
					return NewError(
						ErrInvalidMessage,
						"error code must be a number",
						nil,
					).WithContext("codeType", fmt.Sprintf("%T", code)).
						WithContext("messagePreview", string(message[:min(len(message), 100)]))
				}
			}

			if messageText, exists := errorMap["message"]; !exists {
				return NewError(
					ErrInvalidMessage,
					"error object must contain 'message' field",
					nil,
				).WithContext("messagePreview", string(message[:min(len(message), 100)]))
			} else {
				// Message must be a string
				if _, ok := messageText.(string); !ok {
					return NewError(
						ErrInvalidMessage,
						"error message must be a string",
						nil,
					).WithContext("messageType", fmt.Sprintf("%T", messageText)).
						WithContext("messagePreview", string(message[:min(len(message), 100)]))
				}
			}
		}

		// Response must have either result or error, but not both
		if !hasResult && !hasError {
			return NewError(
				ErrInvalidMessage,
				"response message must contain either 'result' or 'error' field",
				nil,
			).WithContext("messagePreview", string(message[:min(len(message), 100)]))
		}

		if hasResult && hasError {
			return NewError(
				ErrInvalidMessage,
				"response message cannot contain both 'result' and 'error' fields",
				nil,
			).WithContext("messagePreview", string(message[:min(len(message), 100)]))
		}

		// Response shouldn't have method or params
		if _, hasMethod := msg["method"]; hasMethod {
			return NewError(
				ErrInvalidMessage,
				"response message cannot contain 'method' field",
				nil,
			).WithContext("messagePreview", string(message[:min(len(message), 100)]))
		}

		if _, hasParams := msg["params"]; hasParams {
			return NewError(
				ErrInvalidMessage,
				"response message cannot contain 'params' field",
				nil,
			).WithContext("messagePreview", string(message[:min(len(message), 100)]))
		}
	}

	return nil
}

// min returns the smaller of x or y.
// nolint:unparam
func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// NDJSONTransport implements the Transport interface for newline-delimited JSON.
// It supports both stdio and socket-based communications.
type NDJSONTransport struct {
	reader    *bufio.Reader
	writer    io.Writer
	closer    io.Closer
	writeLock sync.Mutex // Ensures atomic writes
	closed    bool
	closeLock sync.RWMutex
}

// NewNDJSONTransport creates a new Transport that reads and writes newline-delimited JSON.
// It works with any paired Reader/Writer, including stdio, TCP connections, etc.
func NewNDJSONTransport(reader io.Reader, writer io.Writer, closer io.Closer) Transport {
	return &NDJSONTransport{
		reader: bufio.NewReader(reader),
		writer: writer,
		closer: closer,
	}
}

// ReadMessage implements Transport.ReadMessage for NDJSON.
// It reads a single line of JSON data delimited by a newline character.
func (t *NDJSONTransport) ReadMessage(ctx context.Context) ([]byte, error) {
	// Check if the transport is closed
	t.closeLock.RLock()
	if t.closed {
		t.closeLock.RUnlock()
		return nil, NewClosedError("read")
	}
	t.closeLock.RUnlock()

	// Create a channel for the result
	type readResult struct {
		data []byte
		err  error
	}
	resultCh := make(chan readResult, 1)

	// Read in a separate goroutine to allow for context cancellation
	go func() {
		// Start reading the line
		var line []byte
		var prefix bool
		var err error
		var totalSize int

		// Buffer to store message parts if they exceed a single read
		var buffer bytes.Buffer

		// Read until we hit a newline or an error
		for {
			line, prefix, err = t.reader.ReadLine()
			if err != nil {
				resultCh <- readResult{nil, errors.Wrap(err, "failed to read message")}
				return
			}

			// Append the line to our buffer
			buffer.Write(line)
			totalSize += len(line)

			// Check if we've hit the size limit
			if totalSize > MaxMessageSize {
				fragment := buffer.Bytes()
				if len(fragment) > 100 {
					fragment = fragment[:100]
				}
				resultCh <- readResult{nil, NewMessageSizeError(totalSize, MaxMessageSize, fragment)}
				return
			}

			// If there's no more to read, we're done
			if !prefix {
				break
			}
		}

		// Get the full message
		message := buffer.Bytes()

		// Validate the message
		if err := ValidateMessage(message); err != nil {
			resultCh <- readResult{nil, err}
			return
		}

		resultCh <- readResult{message, nil}
	}()

	// Wait for either the read to complete or the context to be canceled
	select {
	case <-ctx.Done():
		return nil, NewTimeoutError("read", ctx.Err())
	case result := <-resultCh:
		return result.data, result.err
	}
}

// WriteMessage implements Transport.WriteMessage for NDJSON.
// It writes a single line of JSON data with a trailing newline character.
func (t *NDJSONTransport) WriteMessage(ctx context.Context, message []byte) error {
	// Check if the transport is closed
	t.closeLock.RLock()
	if t.closed {
		t.closeLock.RUnlock()
		return NewClosedError("write")
	}
	t.closeLock.RUnlock()

	// Validate the message first
	if err := ValidateMessage(message); err != nil {
		return err
	}

	// Check message size
	if len(message) > MaxMessageSize {
		fragment := message
		if len(fragment) > 100 {
			fragment = fragment[:100]
		}
		return NewMessageSizeError(len(message), MaxMessageSize, fragment)
	}

	// Create a channel for the result
	resultCh := make(chan error, 1)

	// Lock to ensure no concurrent writes
	t.writeLock.Lock()
	defer t.writeLock.Unlock()

	// Write in a separate goroutine to allow for context cancellation
	go func() {
		// Create a buffer that ends with a newline
		buf := make([]byte, len(message)+1)
		copy(buf, message)
		buf[len(message)] = '\n'

		// Try to write the full message at once
		_, err := t.writer.Write(buf)
		resultCh <- err
	}()

	// Wait for either the write to complete or the context to be canceled
	select {
	case <-ctx.Done():
		return NewTimeoutError("write", ctx.Err())
	case err := <-resultCh:
		if err != nil {
			return errors.Wrap(err, "failed to write message")
		}
		return nil
	}
}

// Close implements Transport.Close.
func (t *NDJSONTransport) Close() error {
	t.closeLock.Lock()
	defer t.closeLock.Unlock()

	// If already closed, just return
	if t.closed {
		return nil
	}

	// Mark as closed
	t.closed = true

	// Close the underlying closer if available
	if t.closer != nil {
		if err := t.closer.Close(); err != nil {
			return errors.Wrap(err, "failed to close transport")
		}
	}

	return nil
}
