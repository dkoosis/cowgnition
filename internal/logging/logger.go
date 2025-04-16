// Package logging provides a common interface and setup for application-wide logging.
package logging

// file: internal/logging/logger.go

import (
	"context"
)

// Logger defines the interface for logging within the application.
// This abstraction allows for different logger implementations while
// maintaining consistent logging conventions throughout the codebase.
type Logger interface {
	// Debug logs a debug-level message.
	Debug(msg string, args ...any)

	// Info logs an info-level message.
	Info(msg string, args ...any)

	// Warn logs a warning-level message.
	Warn(msg string, args ...any)

	// Error logs an error-level message.
	Error(msg string, args ...any)

	// WithContext returns a logger with context values.
	WithContext(ctx context.Context) Logger

	// WithField returns a logger with an additional field.
	WithField(key string, value any) Logger
}

// NoopLogger implements Logger but does nothing.
// Used as a fallback when no logger is provided.
// NoopLogger implements Logger but does nothing.
// Used as a fallback when no logger is provided.
type NoopLogger struct{}

// Debug implements Logger but performs no action.
func (l *NoopLogger) Debug(_ string, _ ...any) {}

// Info implements Logger but performs no action.
func (l *NoopLogger) Info(_ string, _ ...any) {}

// Warn implements Logger but performs no action.
func (l *NoopLogger) Warn(_ string, _ ...any) {}

// Error implements Logger but performs no action.
func (l *NoopLogger) Error(_ string, _ ...any) {} // NOTE: Error method was missing from lint output but likely needs comment too

// WithContext implements Logger, returning the NoopLogger itself.
func (l *NoopLogger) WithContext(_ context.Context) Logger { return l }

// WithField implements Logger, returning the NoopLogger itself.
func (l *NoopLogger) WithField(_ string, _ any) Logger { return l }

// Global singleton instance of NoopLogger.
var noop = &NoopLogger{}

// GetNoopLogger returns the no-op logger instance.
func GetNoopLogger() Logger {
	return noop
}

// defaultLogger is the application's default logger instance.
// Corrected: Removed explicit type Logger.
var defaultLogger = GetNoopLogger()

// SetDefaultLogger sets the default logger for the application.
func SetDefaultLogger(logger Logger) {
	if logger != nil {
		defaultLogger = logger
	}
}

// GetLogger returns a logger, used by packages to get their own logger.
func GetLogger(name string) Logger {
	return defaultLogger.WithField("component", name)
}
