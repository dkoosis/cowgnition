// file: internal/mcp_types/interfaces.go
package mcptypes

import (
	"context"
	"time"
)

// MessageHandler defines the function signature for processing a single MCP message.
// Implementations receive the message bytes and should return response bytes or an error.
// This type is used as the core processing unit in the server and middleware chain.
type MessageHandler func(ctx context.Context, message []byte) ([]byte, error)

// MiddlewareFunc defines the signature for middleware functions.
// A middleware function takes the next MessageHandler in the chain and returns.
// a new MessageHandler that typically performs some action before or after.
// calling the next handler. This allows for composing layers of functionality.
type MiddlewareFunc func(handler MessageHandler) MessageHandler

// Chain defines an interface for building and managing a sequence of middleware functions.
// that culminate in a final MessageHandler.
type Chain interface {
	// Use adds a MiddlewareFunc to the chain. Middlewares are typically executed.
	// in the reverse order they are added.
	Use(middleware MiddlewareFunc) Chain

	// Handler finalizes the chain and returns the composed MessageHandler.
	// Once called, the chain should generally not be modified further.
	Handler() MessageHandler
}

// ValidationOptions holds configuration settings for the validation middleware.
// These options control whether validation is enabled, how strict it is,
// and whether performance should be measured.
type ValidationOptions struct {
	// Enabled controls whether validation is performed at all. Defaults to true.
	Enabled bool
	// StrictMode, if true, causes validation failures to immediately return a.
	// JSON-RPC error response. If false, errors are logged, but processing may continue.
	StrictMode bool
	// ValidateOutgoing determines whether responses sent by the server should be validated.
	ValidateOutgoing bool
	// StrictOutgoing, if true, causes invalid outgoing messages to be replaced.
	// with an internal server error response. If false, errors are logged,
	// but the potentially invalid message is still sent.
	StrictOutgoing bool
	// MeasurePerformance enables logging of validation duration for performance analysis.
	MeasurePerformance bool
	// SkipTypes maps message method names (e.g., "ping") to true if incoming.
	// validation should be skipped for that specific method.
	SkipTypes map[string]bool
}

// ValidatorInterface defines the core methods required for validating messages.
// against a loaded schema. This allows different schema validation implementations.
// to be used interchangeably by the middleware.
type ValidatorInterface interface {
	// Validate checks if the provided data conforms to the schema definition.
	// associated with the given messageType (e.g., MCP method name).
	Validate(ctx context.Context, messageType string, data []byte) error
	// HasSchema checks if a compiled schema definition exists for the given name.
	HasSchema(name string) bool
	// IsInitialized returns true if the validator has successfully loaded and.
	// compiled the necessary schema definitions.
	IsInitialized() bool
	// Initialize loads and compiles the schema from the configured source (override URI or embedded).
	// This must be called successfully before Validate can be used.
	Initialize(ctx context.Context) error
	// GetLoadDuration returns the time taken during the last schema loading phase (reading from source).
	GetLoadDuration() time.Duration
	// GetCompileDuration returns the time taken during the last schema compilation phase.
	GetCompileDuration() time.Duration
	// GetSchemaVersion returns the detected version string of the loaded schema, if identifiable.
	GetSchemaVersion() string
	// Shutdown cleans up resources used by the validator, like closing idle HTTP connections.
	Shutdown() error
	// VerifyMappingsAgainstSchema checks the internal mappings against compiled schemas. <<< CORRECTED: Exported Method Name >>>
	// Note: This is primarily for testing/internal verification, not core validation path.
	VerifyMappingsAgainstSchema() []string // <<< CORRECTED: Exported Method Name >>>
}
