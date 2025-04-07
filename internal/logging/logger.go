// file: internal/logging/logger.go
package logging

import (
	"context"
)

// Logger defines the interface for logging within the application.
// This abstraction allows for different logger implementations while
// maintaining consistent logging conventions throughout the codebase.
type Logger interface {
	// Debug logs a debug-level message
	Debug(msg string, args ...any)

	// Info logs an info-level message
	Info(msg string, args ...any)

	// Warn logs a warning-level message
	Warn(msg string, args ...any)

	// Error logs an error-level message
	Error(msg string, args ...any)

	// WithContext returns a logger with context values
	WithContext(ctx context.Context) Logger

	// WithField returns a logger with an additional field
	WithField(key string, value any) Logger
}

// NoopLogger implements Logger but does nothing.
// Used as a fallback when no logger is provided.
type NoopLogger struct{}

func (l *NoopLogger) Debug(msg string, args ...any)          {}
func (l *NoopLogger) Info(msg string, args ...any)           {}
func (l *NoopLogger) Warn(msg string, args ...any)           {}
func (l *NoopLogger) Error(msg string, args ...any)          {}
func (l *NoopLogger) WithContext(ctx context.Context) Logger { return l }
func (l *NoopLogger) WithField(key string, value any) Logger { return l }

// Global singleton instance of NoopLogger
var noop = &NoopLogger{}

// GetNoopLogger returns the no-op logger instance
func GetNoopLogger() Logger {
	return noop
}

// defaultLogger is the application's default logger instance
var defaultLogger Logger = GetNoopLogger()

// SetDefaultLogger sets the default logger for the application
func SetDefaultLogger(logger Logger) {
	if logger != nil {
		defaultLogger = logger
	}
}

// GetLogger returns a logger, typically used by packages to get their own logger
func GetLogger(name string) Logger {
	return defaultLogger.WithField("component", name)
}
