// Package logging provides a common interface and setup for application-wide logging.
package logging

// file: internal/logging/slog.go

import (
	"context"
	"log/slog"
	"os"
)

// SlogLogger wraps slog.Logger to implement our Logger interface.
type SlogLogger struct {
	logger *slog.Logger
}

// NewSlogLogger creates a new structured logger based on slog.
func NewSlogLogger(level slog.Level) *SlogLogger {
	// Create a new handler with the specified level
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})

	// Create a new logger with the handler
	logger := slog.New(handler)

	return &SlogLogger{
		logger: logger,
	}
}

// Debug logs a debug-level message.
func (l *SlogLogger) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

// Info logs an info-level message.
func (l *SlogLogger) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

// Warn logs a warning-level message.
func (l *SlogLogger) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

// Error logs an error-level message.
func (l *SlogLogger) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

// WithContext returns a logger with the specified context.
func (l *SlogLogger) WithContext(_ context.Context) Logger { // Renamed ctx to _
	// slog.Logger doesn't have a WithContext method
	// So we'll create a new logger and add the context as a field
	// Note: This doesn't actually attach the context to slog for cancellation,
	// but it satisfies our interface
	return &SlogLogger{
		logger: l.logger.With("ctx", "attached"),
	}
}

// WithField returns a logger with an additional field.
func (l *SlogLogger) WithField(key string, value any) Logger {
	// Create a new logger with the field
	newLogger := l.logger.With(key, value)

	return &SlogLogger{
		logger: newLogger,
	}
}

// SetupDefaultLogger initializes the default logger for the application.
// Call this early in main() to set up logging for the entire application.
func SetupDefaultLogger(level string) {
	// Convert string level to slog.Level
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		// Default to info level
		logLevel = slog.LevelInfo
	}

	// Create and set the default logger
	logger := NewSlogLogger(logLevel)
	SetDefaultLogger(logger)
}
