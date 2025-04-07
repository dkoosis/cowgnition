// file: internal/middleware/chain.go
package middleware

import (
	"context"

	"github.com/dkoosis/cowgnition/internal/transport"
)

// Middleware represents a component in the middleware chain.
// It can be either a function or a struct implementing the Middleware interface.
type Middleware interface {
	Handle(ctx context.Context, message []byte, next transport.MessageHandler) ([]byte, error)
}

// FuncMiddleware wraps a function to implement the Middleware interface.
type FuncMiddleware func(ctx context.Context, message []byte, next transport.MessageHandler) ([]byte, error)

// Handle implements the Middleware interface for FuncMiddleware.
func (f FuncMiddleware) Handle(ctx context.Context, message []byte, next transport.MessageHandler) ([]byte, error) {
	return f(ctx, message, next)
}

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
	middlewares []Middleware

	// final is the handler that processes the message after all middleware.
	final transport.MessageHandler
}

// NewChain creates a new middleware chain with the given final handler.
// The final handler is the core message processor that runs after all middleware.
func NewChain(final transport.MessageHandler) *Chain {
	return &Chain{
		middlewares: make([]Middleware, 0),
		final:       final,
	}
}

// Use adds a middleware to the chain.
// It accepts either a Middleware interface or a function that can be converted to FuncMiddleware.
func (c *Chain) Use(middleware interface{}) {
	switch m := middleware.(type) {
	case Middleware:
		c.middlewares = append(c.middlewares, m)
	case func(ctx context.Context, message []byte, next transport.MessageHandler) ([]byte, error):
		c.middlewares = append(c.middlewares, FuncMiddleware(m))
	case transport.MessageHandler:
		// Convert MessageHandler to Middleware
		c.UseFunc(func(ctx context.Context, message []byte, next transport.MessageHandler) ([]byte, error) {
			result, err := m(ctx, message)
			if err != nil || result != nil {
				return result, err
			}
			return next(ctx, message)
		})
	default:
		panic("middleware must be a Middleware interface or compatible function")
	}
}

// UseFunc adds a function middleware to the chain.
func (c *Chain) UseFunc(fn func(ctx context.Context, message []byte, next transport.MessageHandler) ([]byte, error)) {
	c.middlewares = append(c.middlewares, FuncMiddleware(fn))
}

// Handler builds the middleware chain and returns a handler function.
// The returned handler encapsulates the entire processing pipeline.
func (c *Chain) Handler() transport.MessageHandler {
	// Start with the final handler
	var handler transport.MessageHandler = c.final

	// Build the chain from the end to the beginning
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		middleware := c.middlewares[i]
		next := handler // Capture the current handler to use as "next" in the closure

		// Create a new handler that uses this middleware and the captured "next" handler
		handler = func(ctx context.Context, message []byte) ([]byte, error) {
			return middleware.Handle(ctx, message, next)
		}
	}

	return handler
}
