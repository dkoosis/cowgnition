// file: internal/middleware/chain.go
package middleware

import (
	"context"

	"github.com/dkoosis/cowgnition/internal/transport"
)

// Chain represents a middleware chain that processes messages sequentially.
// The chain pattern provides flexibility and separation of concerns by allowing
// multiple processing steps to be composed into a single handler.
//
// Key benefits of the middleware chain approach:
// - Modularity: Each middleware focuses on a single responsibility
// - Configurability: Middlewares can be added or removed based on requirements
// - Reusability: Middleware components can be used in different combinations
// - Testability: Each middleware can be tested in isolation
// - Performance: Unused middleware can be omitted from the chain
type Chain struct {
	// middlewares is the ordered list of middleware handlers.
	middlewares []transport.MessageHandler

	// final is the handler that processes the message after all middleware.
	final transport.MessageHandler
}

// NewChain creates a new middleware chain with the given final handler.
// The final handler is the core message processor that runs after all middleware.
func NewChain(final transport.MessageHandler) *Chain {
	return &Chain{
		middlewares: make([]transport.MessageHandler, 0),
		final:       final,
	}
}

// Use adds a middleware to the chain.
// Middleware will be executed in the order they are added.
func (c *Chain) Use(middleware transport.MessageHandler) {
	c.middlewares = append(c.middlewares, middleware)
}

// Handler builds the middleware chain and returns a handler function.
// The returned handler encapsulates the entire processing pipeline.
//
// The chain is built from the end to the beginning, so that the first middleware
// in the list is the first to process the message, and the final handler is the last.
func (c *Chain) Handler() transport.MessageHandler {
	handler := c.final

	// Build the chain from the end to the beginning
	// This creates a nested structure where each middleware wraps the next one
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		middleware := c.middlewares[i]

		// For middleware that implement the SetNext interface
		// This approach allows middleware to maintain state between calls
		if nextSetter, ok := middleware.(interface {
			SetNext(transport.MessageHandler)
		}); ok {
			nextSetter.SetNext(handler)
			handler = middleware
		} else {
			// For simple middleware functions
			// This approach works for stateless middleware functions
			next := handler // Capture the current handler in the closure
			handler = func(ctx context.Context, message []byte) ([]byte, error) {
				return middleware(ctx, message)
			}
		}
	}

	return handler
}
