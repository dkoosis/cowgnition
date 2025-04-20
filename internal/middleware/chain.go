// Package middleware provides chainable handlers for processing MCP messages, like validation.
// It implements the Chain interface defined in the mcp_type package for creating middleware
// pipelines that can transform, validate, or enhance message processing.
package middleware

// middlewareChain implements the Chain interface for building middleware stacks.
type middlewareChain struct {
	handler     mcp_type.MessageHandler
	middlewares []mcp_type.MiddlewareFunc
	finalized   bool
}

// NewChain creates a new middleware chain with the given final handler.
func NewChain(finalHandler mcp_type.MessageHandler) mcp_type.Chain {
	return &middlewareChain{
		handler:     finalHandler,
		middlewares: make([]mcp_type.MiddlewareFunc, 0),
		finalized:   false,
	}
}

// Use adds a middleware function to the chain.
func (c *middlewareChain) Use(middleware mcp_type.MiddlewareFunc) mcp_type.Chain {
	if c.finalized {
		// If already finalized, create a new chain.
		return NewChain(c.handler).Use(middleware)
	}

	c.middlewares = append(c.middlewares, middleware)
	return c
}

// Handler returns the final composed handler function.
func (c *middlewareChain) Handler() mcp_type.MessageHandler {
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
