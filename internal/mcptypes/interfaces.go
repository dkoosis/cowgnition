// Package mcptypes defines shared types and interfaces for the MCP (Model Context Protocol)
// server and middleware components. It acts as a neutral package that can be imported by
// both mcp and middleware packages, preventing circular dependencies.
package mcptypes

// file: internal/mcptypes/interfaces.go

import (
	"context"
)

// MessageHandler is a function type for handling MCP messages.
// It processes a message (as JSON bytes) and returns a response (as JSON bytes)
// or an error if processing fails.
type MessageHandler func(ctx context.Context, message []byte) ([]byte, error)

// MiddlewareFunc is a function that wraps a MessageHandler with additional functionality
// such as validation, logging, or metrics collection.
type MiddlewareFunc func(handler MessageHandler) MessageHandler

// Chain represents a middleware chain that can be built and executed.
// It allows for composing multiple middleware functions to process a message.
type Chain interface {
	// Use adds a middleware function to the chain.
	Use(middleware MiddlewareFunc) Chain

	// Handler returns the final composed handler function.
	Handler() MessageHandler
}

// ValidationOptions contains configuration options for validation middleware.
type ValidationOptions struct {
	// Enabled controls whether validation is performed at all. Defaults to true.
	Enabled bool

	// StrictMode enables strict validation of message structures. If false,
	// validation errors are logged but processing continues. If true, validation
	// errors result in immediate JSON-RPC error responses.
	StrictMode bool

	// ValidateOutgoing determines whether to validate outgoing messages (responses).
	ValidateOutgoing bool

	// StrictOutgoing enables strict validation specifically for outgoing messages.
	// If true, invalid outgoing messages are replaced with an error response.
	// If false, errors are logged but the original (invalid) message is sent.
	StrictOutgoing bool

	// MeasurePerformance enables performance measurements for validation operations.
	MeasurePerformance bool

	// SkipTypes defines a set of message types (e.g., method names like "ping")
	// for which incoming validation should be skipped.
	SkipTypes map[string]bool
}

// ValidatorInterface defines common operations for a schema validator.
type ValidatorInterface interface {
	// Validate validates data against a schema definition.
	Validate(ctx context.Context, messageType string, data []byte) error

	// HasSchema checks if a schema exists for the given name.
	HasSchema(name string) bool

	// IsInitialized returns whether the validator has been initialized.
	IsInitialized() bool
}
