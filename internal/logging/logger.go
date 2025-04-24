// Package logging provides a common interface and setup for application-wide logging.
// It defines a standard Logger interface and allows for configuring different
// underlying logging implementations (like slog) through a default logger instance.
// This promotes consistent logging practices across the CowGnition codebase.
package logging

// file: internal/logging/logger.go
import (
	"context"
)

// Logger defines a standard interface for logging within the application.
// This abstraction allows for different underlying logger implementations (e.g., slog, zap)
// while maintaining consistent logging call sites throughout the codebase.
type Logger interface {
	// Debug logs a message at the Debug level, typically for detailed tracing or diagnostics.
	// Arguments are handled as key-value pairs.
	Debug(msg string, args ...any)

	// Info logs a message at the Info level, used for informational messages about application state.
	// Arguments are handled as key-value pairs.
	Info(msg string, args ...any)

	// Warn logs a message at the Warn level, indicating potential issues or non-critical problems.
	// Arguments are handled as key-value pairs.
	Warn(msg string, args ...any)

	// Error logs a message at the Error level, signifying errors that require attention.
	// Arguments are handled as key-value pairs. It's recommended to include an 'error' key with the actual error object.
	Error(msg string, args ...any)

	// WithContext returns a logger instance potentially enriched with values from the provided context.
	// The exact behavior depends on the underlying logger implementation.
	WithContext(ctx context.Context) Logger

	// WithField returns a new logger instance with the specified key-value pair added to its context.
	// This is useful for adding persistent context (like component name or request ID) to log messages.
	WithField(key string, value any) Logger
}

// NoopLogger is an implementation of the Logger interface that performs no operations.
// It serves as a safe default when no specific logger is configured, preventing nil pointer errors.
type NoopLogger struct{}

// Debug implements Logger but performs no action for the NoopLogger.
func (l *NoopLogger) Debug(_ string, _ ...any) {}

// Info implements Logger but performs no action for the NoopLogger.
func (l *NoopLogger) Info(_ string, _ ...any) {}

// Warn implements Logger but performs no action for the NoopLogger.
func (l *NoopLogger) Warn(_ string, _ ...any) {}

// Error implements Logger but performs no action for the NoopLogger.
func (l *NoopLogger) Error(_ string, _ ...any) {}

// WithContext implements Logger for NoopLogger, returning the same no-op instance.
func (l *NoopLogger) WithContext(_ context.Context) Logger { return l }

// WithField implements Logger for NoopLogger, returning the same no-op instance.
func (l *NoopLogger) WithField(_ string, _ any) Logger { return l }

// noop holds the singleton instance of NoopLogger.
var noop = &NoopLogger{}

// GetNoopLogger returns the singleton no-op logger instance.
// Useful as a default or in testing environments where logging is not desired.
func GetNoopLogger() Logger {
	return noop
}

// defaultLogger holds the application's globally configured logger instance.
// It defaults to a NoopLogger until explicitly set by SetupDefaultLogger.
var defaultLogger = GetNoopLogger() // Initialize with noop logger.

// SetDefaultLogger assigns the global logger instance used by the application.
// This should be called early during application startup (e.g., in main()).
// If a nil logger is provided, it resets the default to a NoopLogger.
func SetDefaultLogger(logger Logger) {
	if logger != nil {
		defaultLogger = logger
	} else {
		// Fallback to noop logger if nil is provided, ensuring defaultLogger is never nil.
		defaultLogger = GetNoopLogger()
	}
}

// GetLogger returns a logger instance intended for use by a specific component or package.
// It retrieves the globally configured default logger and adds a "component" field
// with the provided name for contextual logging.
func GetLogger(name string) Logger {
	// Ensure defaultLogger is never nil before calling methods on it.
	if defaultLogger == nil {
		defaultLogger = GetNoopLogger()
	}
	return defaultLogger.WithField("component", name)
}
