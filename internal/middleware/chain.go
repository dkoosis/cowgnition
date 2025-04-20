// Package middleware provides chainable handlers for processing MCP messages, like validation.
// It implements the Chain interface defined in the mcptypes package for creating middleware
// pipelines that can transform, validate, or enhance message processing.
package middleware

// file: internal/middleware/chain.go

import (
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
)

// middlewareChain implements the Chain interface for building middleware stacks.
type middlewareChain struct {
	handler     mcptypes.MessageHandler   // Use type from renamed package.
	middlewares []mcptypes.MiddlewareFunc // Use type from renamed package.
	finalized   bool
}

// NewChain creates a new middleware chain with the given final handler.
func NewChain(finalHandler mcptypes.MessageHandler) mcptypes.Chain { // Use type from renamed package.
	return &middlewareChain{
		handler:     finalHandler,
		middlewares: make([]mcptypes.MiddlewareFunc, 0), // Use type from renamed package.
		finalized:   false,
	}
}

// Use adds a middleware function to the chain.
func (c *middlewareChain) Use(middleware mcptypes.MiddlewareFunc) mcptypes.Chain { // Use type from renamed package.
	if c.finalized {
		// If already finalized, create a new chain.
		return NewChain(c.handler).Use(middleware)
	}

	c.middlewares = append(c.middlewares, middleware)
	return c
}

// Handler returns the final composed handler function.
func (c *middlewareChain) Handler() mcptypes.MessageHandler { // Use type from renamed package.
	if c.finalized {
		return c.handler
	}

	// Apply middleware in reverse order (last added is first executed).
	handler := c.handler
	for i := len(c.middlewares) - 1; i >= 0; i-- {
		handler = c.middlewares[i](handler)
	}

	// Mark as finalized and store the composed handler.
	c.finalized = true
	c.handler = handler

	return handler
}
