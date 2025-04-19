// Package server provides the runner and setup logic for the main CowGnition MCP server process.
// It handles server lifecycle management including initialization, runtime operation,
// and graceful shutdown. The package integrates various components such as configuration
// loading, schema validation, RTM service connectivity diagnostics, and transport-specific
// server implementations (stdio, http). It also manages signal handling for proper
// server termination and resource cleanup.
package server

// file: cmd/server/server_runner.go

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

	logger.Info("🚀 Starting CowGnition server",
		"transport", transportType,
		"config_path", configPath,
		"request_timeout", requestTimeout,
		"shutdown_timeout", shutdownTimeout,
		"debug_mode", debug)

	// --- Configuration Loading ---
	var cfg *config.Config
	var err error
	if configPath != "" {
		logger.Info("📂 Loading configuration from file", "config_path", configPath)
		cfg, err = config.LoadFromFile(configPath)
		if err != nil {
			logger.Error("❌ Failed to load configuration", "config_path", configPath, "error", err.Error())
			return errors.Wrap(err, "failed to load configuration from file")
		}
		logger.Info("✅ Configuration loaded successfully", "config_path", configPath)
	} else {
		logger.Info("📝 Using default configuration (no config file specified)")
		cfg = config.DefaultConfig()
	}
	logger.Info("⚙️ Configuration ready")

	// --- Schema Validator Initialization ---
	logger.Info("📋 Initializing schema validator...")

	// Try multiple schema sources for better reliability
	// 1. Try bundled schema in the project
	// 2. Try specific GitHub URL used for authentication
	// 3. Try main branch as fallback
	schemaSource := schema.SchemaSource{
		// Try local file first (most reliable)
		FilePath: "internal/schema/schema.json", // Use bundled schema if available

		// Then try URL with exact version (most likely to work)
		URL: "https://raw.githubusercontent.com/modelcontextprotocol/specification/main/schema/schema.json",

		// Or embed schema directly as last resort
		Embedded: []byte(internalSchemaJSON), // Ensure the internal schema JSON is used as a fallback
	}

	logger.Info("🔍 Schema sources configured",
		"local_path", schemaSource.FilePath,
		"fallback_url", schemaSource.URL,
		"has_embedded", len(schemaSource.Embedded) > 0)

	// Use a more descriptive log field for the schema validator
	schemaLogger := logger.WithField("component", "schema_validator")

	validator := schema.NewSchemaValidator(schemaSource, schemaLogger)
	logger.Info("🔄 Initializing schema validator - this might take a moment...")
	if err = validator.Initialize(ctx); err != nil {
		logger.Error("❌ Schema validator initialization failed",
			"error", err.Error(),
			"advice", "Check schema source URLs and network connectivity")
		// Provide user-friendly error message
		if os.IsNotExist(errors.Cause(err)) {
			logger.Error("💡 Schema file not found. Suggestions:",
				"tip1", "Ensure schema file exists at the specified path",
				"tip2", "Consider using MCP development kit with bundled schemas",
				"tip3", "Check network connectivity for URL schema source")
		}
		return errors.Wrap(err, "failed to initialize schema validator")
	}
	logger.Info("✅ Schema validator initialized successfully",
		"schema_version", validator.GetSchemaVersion())

	// --- RTM Service Initialization and Diagnostics ---
	logger.Info("🔄 Creating and initializing RTM service...")
	rtmFactory := rtm.NewServiceFactory(cfg, logging.GetLogger("rtm_factory"))
	rtmService, err := rtmFactory.CreateService(ctx)
	if err != nil {
		logger.Error("❌ RTM service initialization failed", "error", err.Error())
		return errors.Wrap(err, "failed to initialize RTM service")
	}

	// Perform RTM connectivity diagnostics
	logger.Info("🔍 Performing RTM connectivity diagnostics...")
	options := rtm.DefaultConnectivityCheckOptions()
	diagResults, diagErr := rtmService.PerformConnectivityCheck(ctx, options)

	// Log diagnostic results
	for _, result := range diagResults {
		if result.Success {
			logger.Info(fmt.Sprintf("✅ RTM Diagnostic: %s", result.Name),
				"duration_ms", result.Duration.Milliseconds(),
				"description", result.Description)
		} else {
			logger.Warn(fmt.Sprintf("⚠️ RTM Diagnostic: %s", result.Name),
				"duration_ms", result.Duration.Milliseconds(),
				"description", result.Description,
				"error", result.Error)
		}
	}

	// Handle diagnostic failures
	if diagErr != nil {
		logger.Error("❌ RTM connectivity diagnostics failed", "error", diagErr.Error())
		logger.Error("💡 RTM connectivity troubleshooting tips:",
			"tip1", "Verify your RTM API key and shared secret are correct",
			"tip2", "Check your internet connection",
			"tip3", "Ensure the RTM API is accessible from your network")
		// Critical diagnostic failures should be treated as server startup failures
		return errors.Wrap(diagErr, "failed to verify RTM connectivity")
	}

	// User-friendly authentication status
	if rtmService.IsAuthenticated() {
		logger.Info("🔑 RTM authentication successful",
			"username", rtmService.GetUsername(),
			"status", "Ready to access tasks and lists")
	} else {
		logger.Warn("⚠️ Not authenticated with RTM",
			"status", "Limited functionality available",
			"advice", "Use authentication tools through Claude Desktop to connect")
	}

	// --- Create MCP Server Options ---
	opts := mcp.ServerOptions{
		RequestTimeout:  requestTimeout,
		ShutdownTimeout: shutdownTimeout,
		Debug:           debug,
	}

	// --- Create MCP Server Instance ---
	logger.Info("🔄 Creating MCP server instance")
	server, err := mcp.NewServer(cfg, opts, validator, startTime, logger)
	if err != nil {
		logger.Error("❌ Failed to create MCP server", "error", err.Error())
		return errors.Wrap(err, "failed to create MCP server")
	}

	// --- Start Server (logic for selecting transport) ---
	switch transportType {
	case "stdio":
		logger.Info("📡 Starting server with stdio transport",
			"description", "Communication via standard input/output")
		go func() {
			if err := server.ServeSTDIO(ctx); err != nil {
				// Check if the error is due to context cancellation (expected during shutdown)
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					// Log only unexpected errors
					logger.Error("❌ Server error (stdio)", "error", fmt.Sprintf("%+v", err))
				} else {
					logger.Info("🛑 Server stopped gracefully (stdio)", "reason", err)
				}
				cancel() // Ensure context is canceled on any server error/stop
			} else {
				logger.Info("🛑 Server stopped normally (stdio)")
				cancel() // Cancel context if server stops without error
			}
		}()

	case "http":
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		logger.Info("📡 Starting server with HTTP transport",
			"address", addr,
			"description", "Communication via HTTP protocol")
		go func() {
			if err := server.ServeHTTP(ctx, addr); err != nil {
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, errors.New("HTTP transport not implemented")) /* Temp check */ {
					logger.Error("❌ Server error (http)", "error", fmt.Sprintf("%+v", err))
				} else {
					logger.Info("🛑 Server stopped gracefully (http)", "reason", err)
				}
				cancel() // Ensure context is canceled on any server error/stop
			} else {
				logger.Info("🛑 Server stopped normally (http)")
				cancel() // Cancel context if server stops without error
			}
		}()

	default:
		// Ensure validator is shut down on this error path too.
		if shutdownErr := validator.Shutdown(); shutdownErr != nil {
			// Log the shutdown error, but proceed with returning the main error.
			logger.Error("⚠️ Error shutting down schema validator during transport type error", "error", shutdownErr)
		}
		logger.Error("❌ Unsupported transport type",
			"transport", transportType,
			"supported", "stdio, http",
			"advice", "Use --transport stdio or --transport http")
		return errors.Newf("unsupported transport type: %s", transportType)
	}

	logger.Info("✅ Server startup complete and ready to process requests",
		"startup_time_ms", time.Since(startTime).Milliseconds())

	// --- Wait for signal or context cancellation ---
	select {
	case sig := <-sigChan:
		logger.Info("⏹️ Received termination signal", "signal", sig)
	case <-ctx.Done():
		logger.Info("⏹️ Context cancelled, initiating shutdown")
	}

	// --- Graceful Shutdown ---
	logger.Info("🔄 Shutting down server gracefully...",
		"timeout", shutdownTimeout.String())
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	// Shutdown validator
	logger.Debug("Shutting down schema validator")
	if err := validator.Shutdown(); err != nil {
		logger.Error("⚠️ Error shutting down schema validator", "error", err)
		// We continue with server shutdown even if schema validator shutdown failed
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("❌ Server shutdown error", "error", err)
		return errors.Wrap(err, "server shutdown error")
	}

	logger.Info("👋 Server shutdown complete - goodbye!",
		"run_duration", time.Since(startTime).String())
	return nil
}

// Define the internal schema JSON to use as a fallback
// This is a skeleton version - would need to be replaced with the real schema.
const internalSchemaJSON = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "definitions": {
    "Request": {
      "properties": {
        "method": {
          "type": "string"
        },
        "params": {
          "additionalProperties": {},
          "properties": {
            "_meta": {
              "properties": {
                "progressToken": {
                  "description": "Progress token for out-of-band notifications",
                  "type": ["string", "integer"]
                }
              },
              "type": "object"
            }
          },
          "type": "object"
        }
      },
      "required": ["method"],
      "type": "object"
    }
  }
}`
