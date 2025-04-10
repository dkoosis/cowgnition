// file: internal/transport/in_memory_transport.go
package transport

import (
	"context"
	"sync"

	"github.com/cockroachdb/errors"
)

// InMemoryTransport implements the Transport interface using in-memory channels.
// It's designed specifically for testing purposes, allowing two transport instances
// to communicate with each other without actual I/O.
type InMemoryTransport struct {
	// incomingMessages is a channel of messages to be read by ReadMessage
	incomingMessages chan []byte

	// outgoingMessages is a channel to send messages to the paired transport
	outgoingMessages chan []byte

	// closed indicates whether the transport has been closed
	closed bool

	// closeLock protects the closed flag from concurrent access
	closeLock sync.RWMutex

	// readLock ensures only one ReadMessage call is active at a time
	readLock sync.Mutex

	// writeLock ensures only one WriteMessage call is active at a time
	writeLock sync.Mutex
}

// InMemoryTransportPair contains a pair of linked InMemoryTransport instances
// that communicate with each other.
type InMemoryTransportPair struct {
	ClientTransport *InMemoryTransport
	ServerTransport *InMemoryTransport
}

// NewInMemoryTransportPair creates a pair of InMemoryTransport instances
// that are connected to each other. Messages written to one can be read from the other.
// This is particularly useful for testing MCP server-client interactions.
func NewInMemoryTransportPair() *InMemoryTransportPair {
	// Create channels with buffer size 100 to avoid immediate blocking
	clientToServer := make(chan []byte, 100)
	serverToClient := make(chan []byte, 100)

	clientTransport := &InMemoryTransport{
		incomingMessages: serverToClient,
		outgoingMessages: clientToServer,
	}

	serverTransport := &InMemoryTransport{
		incomingMessages: clientToServer,
		outgoingMessages: serverToClient,
	}

	return &InMemoryTransportPair{
		ClientTransport: clientTransport,
		ServerTransport: serverTransport,
	}
}

// ReadMessage implements Transport.ReadMessage.
// It reads a message from the incomingMessages channel.
func (t *InMemoryTransport) ReadMessage(ctx context.Context) ([]byte, error) {
	// Get read lock to ensure only one read operation at a time
	t.readLock.Lock()
	defer t.readLock.Unlock()

	// Check if transport is closed
	t.closeLock.RLock()
	if t.closed {
		t.closeLock.RUnlock()
		return nil, NewClosedError("read")
	}
	t.closeLock.RUnlock()

	// Wait for a message or context cancellation
	select {
	case <-ctx.Done():
		return nil, errors.Wrap(ctx.Err(), "context cancelled during read")
	case msg, ok := <-t.incomingMessages:
		if !ok {
			// Channel closed
			return nil, NewClosedError("read from closed channel")
		}

		// Validate the message
		if err := ValidateMessage(msg); err != nil {
			return nil, err
		}

		return msg, nil
	}
}

// WriteMessage implements Transport.WriteMessage.
// It sends a message to the outgoingMessages channel.
func (t *InMemoryTransport) WriteMessage(ctx context.Context, message []byte) error {
	// Get write lock to ensure only one write operation at a time
	t.writeLock.Lock()
	defer t.writeLock.Unlock()

	// Check if transport is closed
	t.closeLock.RLock()
	if t.closed {
		t.closeLock.RUnlock()
		return NewClosedError("write")
	}
	t.closeLock.RUnlock()

	// Validate the message
	if err := ValidateMessage(message); err != nil {
		return err
	}

	// Check message size
	if len(message) > MaxMessageSize {
		return NewMessageSizeError(len(message), MaxMessageSize, message[:min(len(message), 100)])
	}

	// Send message with context awareness
	select {
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "context cancelled during write")
	case t.outgoingMessages <- message:
		return nil
	}
}

// Close implements Transport.Close.
// It marks the transport as closed and closes the channels.
func (t *InMemoryTransport) Close() error {
	t.closeLock.Lock()
	defer t.closeLock.Unlock()

	if t.closed {
		return nil // Already closed
	}

	t.closed = true

	// We don't actually close the channels here because:
	// 1. Closing a send-only channel from the receiver can cause panics
	// 2. The paired transport still needs to drain messages
	//
	// Instead, future read/write operations will check the closed flag
	// and return appropriate errors.

	return nil
}

// CloseChannels explicitly closes both channels in the transport pair.
// This should only be called when both transports are done using the channels,
// typically after both transports have been closed with Close().
// This is primarily used in tests during cleanup.
func (p *InMemoryTransportPair) CloseChannels() {
	// Close the channels to release resources
	// Be careful to only close each channel once
	p.ServerTransport.closeLock.Lock()
	p.ClientTransport.closeLock.Lock()

	// Only close if not already done
	// Note: This requires some coordination between the pair
	close(p.ServerTransport.outgoingMessages)
	close(p.ClientTransport.outgoingMessages)

	p.ClientTransport.closeLock.Unlock()
	p.ServerTransport.closeLock.Unlock()
}
