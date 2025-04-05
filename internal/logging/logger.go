// internal/logging/logger.go
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// LogLevel represents logging level.
type LogLevel string

const (
	LevelDebug LogLevel = "DEBUG"
	LevelInfo  LogLevel = "INFO"
	LevelWarn  LogLevel = "WARN"
	LevelError LogLevel = "ERROR"
)

var (
	// Default logger instance.
	defaultLogger *slog.Logger

	// Component-specific loggers cache.
	loggers      = make(map[string]*slog.Logger)
	loggersMutex sync.RWMutex

	// Current log level.
	currentLevel = new(slog.LevelVar)

	// Flag to track if logging has been initialized - removed unused variable.
	initMutex sync.Mutex
)

// init initializes the default logger with INFO level.
func init() {
	// Set default level to INFO.
	currentLevel.Set(slog.LevelInfo)

	// Create the default JSON handler.
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: currentLevel,
	})

	// Create the default logger.
	defaultLogger = slog.New(handler)

	// Set as the default logger for the slog package.
	slog.SetDefault(defaultLogger)
}

// InitLogging initializes the logging system with the specified configuration.
func InitLogging(level LogLevel, output io.Writer) {
	initMutex.Lock()
	defer initMutex.Unlock()

	// Convert string level to slog.Level.
	var slogLevel slog.Level
	switch strings.ToUpper(string(level)) {
	case string(LevelDebug):
		slogLevel = slog.LevelDebug
	case string(LevelInfo):
		slogLevel = slog.LevelInfo
	case string(LevelWarn):
		slogLevel = slog.LevelWarn
	case string(LevelError):
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	// Set the level.
	currentLevel.Set(slogLevel)

	// If output is nil, use stderr.
	if output == nil {
		output = os.Stderr
	}

	// Create a new handler
	handler := slog.NewJSONHandler(output, &slog.HandlerOptions{
		Level: currentLevel,
	})

	// Update the default logger.
	defaultLogger = slog.New(handler)

	// Set as the default logger for the slog package
	slog.SetDefault(defaultLogger)

	// Clear the loggers cache as the configuration has changed
	loggersMutex.Lock()
	loggers = make(map[string]*slog.Logger)
	loggersMutex.Unlock()
}

// GetLogger returns a logger for the specified component.
func GetLogger(component string) *slog.Logger {
	loggersMutex.RLock()
	logger, exists := loggers[component]
	loggersMutex.RUnlock()

	if exists {
		return logger
	}

	// Create a new logger with the component as a default attribute
	newLogger := defaultLogger.With("component", component)

	// Cache the logger
	loggersMutex.Lock()
	loggers[component] = newLogger
	loggersMutex.Unlock()

	return newLogger
}

// SetLevel sets the logging level.
func SetLevel(level LogLevel) {
	var slogLevel slog.Level
	switch strings.ToUpper(string(level)) {
	case string(LevelDebug):
		slogLevel = slog.LevelDebug
	case string(LevelInfo):
		slogLevel = slog.LevelInfo
	case string(LevelWarn):
		slogLevel = slog.LevelWarn
	case string(LevelError):
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	currentLevel.Set(slogLevel)
}

// IsDebugEnabled returns true if debug logging is enabled.
func IsDebugEnabled() bool {
	return currentLevel.Level() <= slog.LevelDebug
}

// IsInfoEnabled returns true if info logging is enabled.
func IsInfoEnabled() bool {
	return currentLevel.Level() <= slog.LevelInfo
}

// WithContext returns a logger with context values.
func WithContext(ctx context.Context, logger *slog.Logger) *slog.Logger {
	// This method can be expanded to extract relevant values from the context
	// and add them to the logger
	return logger
}
