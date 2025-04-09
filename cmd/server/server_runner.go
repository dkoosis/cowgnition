// file: cmd/server/server_runner.go
package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath" // Needed for schema path
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/mcp"
	"github.com/dkoosis/cowgnition/internal/schema" // Import schema package
)

// RunServer starts the MCP server with the specified transport type.
// It handles setup, startup, and graceful shutdown of the server.
// nolint:gocyclo
func RunServer(transportType, configPath string, requestTimeout, shutdownTimeout time.Duration, debug bool) error {
	// --- Context and Signal Handling (unchanged) ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// --- Logging Setup (unchanged) ---
	logLevel := "info"
	if debug {
		logLevel = "debug"
	}
	logging.SetupDefaultLogger(logLevel)
	logger := logging.GetLogger("server_logic") // Changed logger name for clarity

	logger.Info("Starting server run sequence",
		"transport", transportType,
		"config_path", configPath,
		"request_timeout", requestTimeout,
		"shutdown_timeout", shutdownTimeout)

	// --- Configuration Loading (unchanged) ---
	var cfg *config.Config
	var err error
	if configPath != "" {
		logger.Info("Loading configuration", "config_path", configPath)
		cfg, err = config.LoadFromFile(configPath)
		if err != nil {
			return errors.Wrap(err, "failed to load configuration from file")
		}
		logger.Info("Configuration loaded successfully", "config_path", configPath)
	} else {
		logger.Info("Using default configuration")
		cfg = config.DefaultConfig()
	}
	logger.Info("Configuration loaded successfully")

	// --- *** UPDATED: Initialize Schema Validator with improved path finding *** ---
	logger.Info("Initializing schema validator...")

	// Try multiple paths to find the schema file
	var schemaFilePath string
	possiblePaths := []string{
		filepath.Join("internal", "schema", "schema.json"),             // Relative from working dir
		filepath.Join("..", "internal", "schema", "schema.json"),       // One level up
		filepath.Join("..", "..", "internal", "schema", "schema.json"), // Two levels up
		filepath.Join(".", "schema.json"),                              // Local dir
		filepath.Join("internal", "schema", "min_schema.json"),         // Try min_schema as fallback
	}

	// Check each path and use the first one that exists
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			schemaFilePath = path
			logger.Info("Found schema file", "path", schemaFilePath)
			break
		}
	}

	// Configure schema source with file path (if found) and URL fallback
	schemaSource := schema.SchemaSource{
		FilePath: schemaFilePath, // May be empty if no file found
		// Use the MCP repository URL as fallback
		URL: "https://raw.githubusercontent.com/modelcontextprotocol/specification/main/schema/2025-03-26/schema.json",
	}

	validator := schema.NewSchemaValidator(schemaSource, logging.GetLogger("schema_validator"))
	if err := validator.Initialize(ctx); err != nil {
		return errors.Wrap(err, "failed to initialize schema validator")
	}
	logger.Info("Schema validator initialized successfully.")
	// --- *** End Schema Validator Init *** ---

	// --- Create MCP Server Options (unchanged) ---
	opts := mcp.ServerOptions{
		RequestTimeout:  requestTimeout,
		ShutdownTimeout: shutdownTimeout,
		Debug:           debug,
	}

	// --- Create MCP Server Instance ---
	// *** MODIFIED: Pass the initialized validator to NewServer ***
	server, err := mcp.NewServer(cfg, opts, validator, logger) // Pass validator
	if err != nil {
		return errors.Wrap(err, "failed to create MCP server")
	}

	// --- Start Server (unchanged logic for selecting transport) ---
	switch transportType {
	case "stdio":
		logger.Info("Starting server with stdio transport")
		go func() {
			// *** MODIFIED: Call ServeSTDIO which now uses middleware ***
			if err := server.ServeSTDIO(ctx); err != nil {
				// Check if the error is due to context cancellation (expected during shutdown)
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					// Log only unexpected errors
					logger.Error("Server error (stdio)", "error", fmt.Sprintf("%+v", err))
				} else {
					logger.Info("Server stopped gracefully (stdio).", "reason", err)
				}
				cancel() // Ensure context is canceled on any server error/stop
			} else {
				logger.Info("Server stopped normally (stdio).")
				cancel() // Cancel context if server stops without error
			}
		}()

	case "http":
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		logger.Info("Starting server with HTTP transport", "address", addr)
		go func() {
			// *** MODIFIED: Call ServeHTTP which should also use middleware ***
			if err := server.ServeHTTP(ctx, addr); err != nil {
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, errors.New("HTTP transport not implemented")) /* Temp check */ {
					logger.Error("Server error (http)", "error", fmt.Sprintf("%+v", err))
				} else {
					logger.Info("Server stopped gracefully (http).", "reason", err)
				}
				cancel() // Ensure context is canceled on any server error/stop
			} else {
				logger.Info("Server stopped normally (http).")
				cancel() // Cancel context if server stops without error
			}
		}()

	default:
		// Ensure validator is shut down on this error path too.
		if shutdownErr := validator.Shutdown(); shutdownErr != nil {
			// Log the shutdown error, but proceed with returning the main error.
			logger.Error("Error shutting down schema validator during transport type error", "error", shutdownErr)
		}
		return errors.Newf("unsupported transport type: %s", transportType)
	}

	// --- Wait for signal or context cancellation (unchanged) ---
	select {
	case sig := <-sigChan:
		logger.Info("Received signal", "signal", sig)
	case <-ctx.Done():
		logger.Info("Context cancelled, initiating shutdown.")
	}

	// --- Graceful Shutdown ---
	logger.Info("Shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	// *** ADDED: Shutdown validator ***
	if err := validator.Shutdown(); err != nil {
		logger.Error("Error shutting down schema validator", "error", err)
		// Decide if this is fatal or just a warning
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		return errors.Wrap(err, "server shutdown error")
	}

	logger.Info("Server shutdown complete")
	return nil
}
