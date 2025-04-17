// Package server provides the runner and setup logic for the main CowGnition MCP server process.
// It handles server lifecycle management including initialization, runtime operation,
// and graceful shutdown. The package integrates various components such as configuration
// loading, schema validation, RTM service connectivity diagnostics, and transport-specific
// server implementations (stdio, http). It also manages signal handling for proper
// server termination and resource cleanup.
// file: cmd/server/server_runner.go
package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/mcp"
	"github.com/dkoosis/cowgnition/internal/rtm"
	"github.com/dkoosis/cowgnition/internal/schema" // Add import for schema validator
)

// RunServer starts the MCP server with the specified transport type.
// It handles setup, startup, and graceful shutdown of the server.
// nolint:gocyclo
func RunServer(transportType, configPath string, requestTimeout, shutdownTimeout time.Duration, debug bool) error {
	startTime := time.Now()

	// --- Context and Signal Handling ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// --- Logging Setup ---
	logLevel := "info"
	if debug {
		logLevel = "debug"
	}
	logging.SetupDefaultLogger(logLevel)
	logger := logging.GetLogger("server_runner")

	logger.Info("Starting server run sequence",
		"transport", transportType,
		"config_path", configPath,
		"request_timeout", requestTimeout,
		"shutdown_timeout", shutdownTimeout)

	// --- Configuration Loading ---
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

	// --- Schema Validator Initialization ---
	logger.Info("Initializing schema validator...")
	schemaSource := schema.SchemaSource{
		// Configure appropriate schema source
		URL: "https://raw.githubusercontent.com/anthropics/ModelContextProtocol/main/schema/mcp-schema.json",
	}
	validator := schema.NewSchemaValidator(schemaSource, logger.WithField("component", "schema_validator"))
	if err = validator.Initialize(ctx); err != nil {
		return errors.Wrap(err, "failed to initialize schema validator")
	}
	logger.Info("Schema validator initialized")

	// --- RTM Service Initialization and Diagnostics ---
	logger.Info("Creating and initializing RTM service...")
	rtmFactory := rtm.NewServiceFactory(cfg, logging.GetLogger("rtm_factory"))
	rtmService, err := rtmFactory.CreateService(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize RTM service")
	}

	// Perform RTM connectivity diagnostics
	logger.Info("Performing RTM connectivity diagnostics...")
	options := rtm.DefaultConnectivityCheckOptions()
	diagResults, diagErr := rtmService.PerformConnectivityCheck(ctx, options)

	// Log diagnostic results
	for _, result := range diagResults {
		if result.Success {
			logger.Info(fmt.Sprintf("RTM Diagnostic: %s - Success", result.Name),
				"duration_ms", result.Duration.Milliseconds(),
				"description", result.Description)
		} else {
			logger.Warn(fmt.Sprintf("RTM Diagnostic: %s - Failed", result.Name),
				"duration_ms", result.Duration.Milliseconds(),
				"description", result.Description,
				"error", result.Error)
		}
	}

	// Handle diagnostic failures
	if diagErr != nil {
		logger.Error("RTM connectivity diagnostics failed", "error", diagErr)
		// Critical diagnostic failures should be treated as server startup failures
		return errors.Wrap(diagErr, "failed to verify RTM connectivity")
	}

	logger.Info("RTM service initialized successfully",
		"authenticated", rtmService.IsAuthenticated(),
		"username", rtmService.GetUsername())

	// --- Create MCP Server Options ---
	opts := mcp.ServerOptions{
		RequestTimeout:  requestTimeout,
		ShutdownTimeout: shutdownTimeout,
		Debug:           debug,
	}

	// --- Create MCP Server Instance ---
	server, err := mcp.NewServer(cfg, opts, validator, startTime, logger)
	if err != nil {
		return errors.Wrap(err, "failed to create MCP server")
	}

	// --- Start Server (logic for selecting transport) ---
	switch transportType {
	case "stdio":
		logger.Info("Starting server with stdio transport")
		go func() {
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

	// --- Wait for signal or context cancellation ---
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

	// Shutdown validator
	if err := validator.Shutdown(); err != nil {
		logger.Error("Error shutting down schema validator", "error", err)
		// We continue with server shutdown even if schema validator shutdown failed
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		return errors.Wrap(err, "server shutdown error")
	}

	logger.Info("Server shutdown complete")
	return nil
}
