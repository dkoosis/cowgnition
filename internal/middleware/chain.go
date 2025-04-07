// file: internal/middleware/chain.go
package middleware

import (
	"context"

	"github.com/dkoosis/cowgnition/internal/transport"
)

// MiddlewareWithNext is an interface for middleware components that need to maintain
// their own state and be linked to the next handler.
type MiddlewareWithNext interface {
	// SetNext sets the next handler in the chain.
	SetNext(next transport.MessageHandler)

	// HandleMessage processes a message and passes it to the next handler if needed.
	// This should be implemented by middleware components to satisfy transport.MessageHandler.
	HandleMessage(ctx context.Context, message []byte) ([]byte, error)
}

// Chain represents a middleware chain that processes messages sequentially.
// The chain pattern provides flexibility and separation of concerns by allowing
// multiple processing steps to be composed into a single handler.
//
// Key benefits of the middleware chain approach:
// - Modularity: Each middleware focuses on a single responsibility.
// - Configurability: Middlewares can be added or removed based on requirements.
// - Reusability: Middleware components can be used in different combinations.
// - Testability: Each middleware can be tested in isolation.
// - Performance: Unused middleware can be omitted from the chain.
type Chain struct {
	// middlewares is the ordered list of middleware handlers.
	middlewares []interface{}

	// final is the handler that processes the message after all middleware.
	final transport.MessageHandler
}

// NewChain creates a new middleware chain with the given final handler.
// The final handler is the core message processor that runs after all middleware.
func NewChain(final transport.MessageHandler) *Chain {
	return &Chain{
		middlewares: make([]interface{}, 0),
		final:       final,
	}
}

// Use adds a middleware to the chain.
// Middleware will be executed in the order they are added.
// It accepts either a MiddlewareWithNext implementation or a function with
// the transport.MessageHandler signature.
func (c *Chain) Use(middleware interface{}) {
	c.middlewares = append(c.middlewares, middleware)
}

// Handler builds the middleware chain and returns a handler function.
// The returned handler encapsulates the entire processing pipeline.
func (c *Chain) Handler() transport.MessageHandler {
	handler := c.final

	// Build the chain from the end to the beginning.
	// This creates a nested structure where each middleware wraps the next one.
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		middleware := c.middlewares[i]

		// Check if middleware implements MiddlewareWithNext.
		if mw, ok := middleware.(MiddlewareWithNext); ok {
			mw.SetNext(handler)
			handler = mw.HandleMessage
		} else if mwFunc, ok := middleware.(transport.MessageHandler); ok {
			// For function middlewares.
			nextHandler := handler
			handler = func(ctx context.Context, message []byte) ([]byte, error) {
				// Call the middleware.
				result, err := mwFunc(ctx, message)
				if err != nil {
					// If middleware returns an error, propagate it.
					return nil, err
				}

				// If middleware returned a result, it means it handled the request completely.
				if result != nil {
					return result, nil
				}

				// Otherwise, continue the chain by calling the next handler.
				return nextHandler(ctx, message)
			}
		} else {
			// This would be a developer error - middleware must be either a MiddlewareWithNext or a MessageHandler.
			panic("Invalid middleware type. Must be either a MiddlewareWithNext or a transport.MessageHandler function.")
		}
	}

	return handler
}
