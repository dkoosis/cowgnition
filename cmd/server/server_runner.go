// Package server provides the runner and setup logic for the main CowGnition MCP server process.
// It handles server lifecycle management including initialization, runtime operation,
// and graceful shutdown. The package integrates various components such as configuration
// loading, schema validation, RTM service connectivity diagnostics, and transport-specific
// server implementations (stdio, http). It also manages signal handling for proper
// server termination and resource cleanup.
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
	"github.com/dkoosis/cowgnition/internal/schema"
)

// RunServer starts the MCP server with the specified transport type.
func RunServer(transportType, configPath string, requestTimeout, shutdownTimeout time.Duration, debug bool) error {
	startTime := time.Now()

	// Setup Context and Signal Handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Logging and Configuration Setup.
	logger, cfg, err := setupLoggingAndConfig(configPath, debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed during logging/config setup: %+v\n", err)
		return err
	}

	logger.Info("🚀 Starting CowGnition server.",
		"transport", transportType,
		"config_path", configPath,
		"request_timeout", requestTimeout,
		"shutdown_timeout", shutdownTimeout,
		"debug_mode", debug)

	// Service Initialization.
	// Use blank identifier "_" for rtmService to fix unused variable error.
	// Corrected: Use ValidatorInterface.
	validator, rtmService, err := initializeServices(ctx, cfg, logger) // <-- rtmService is returned here
	if err != nil {
		return err // Errors already logged within initializeServices.
	}
	defer func() {
		// Ensure validator shutdown runs even if server start fails.
		if shutdownErr := validator.Shutdown(); shutdownErr != nil {
			logger.Error("⚠️ Error shutting down schema validator during potential early exit.", "error", shutdownErr)
		}
	}()

	// MCP Server Creation.
	// Corrected: Pass ValidatorInterface.
	server, err := createMCPServer(cfg, requestTimeout, shutdownTimeout, debug, validator, startTime, logger) // <-- server is created here
	if err != nil {
		return err // Error already logged.
	}

	// ---> ADD THIS BLOCK HERE <---
	if rtmService != nil { // Check if RTM service initialization succeeded
		logger.Info("🔧 Registering RTM service with MCP server.") // Add logging
		if err := server.RegisterService(rtmService); err != nil {
			// Handle registration error appropriately
			logger.Error("❌ Failed to register RTM service.", "error", err)
			// Decide if this should be fatal, returning the error might be best:
			return errors.Wrap(err, "failed to register RTM service")
		}
	} else {
		// This case shouldn't happen if initializeServices succeeded without error, but good practice.
		logger.Warn("⚠️ RTM Service instance was nil after initialization, cannot register.")
	}
	// ---> END BLOCK TO ADD <---

	// Start Server Transport.
	if err := startServerTransport(ctx, transportType, cfg, server, cancel, logger); err != nil {
		return err // Error already logged.
	}

	logger.Info("✅ Server startup complete and ready to process requests.",
		"startup_time_ms", time.Since(startTime).Milliseconds())

	// Wait for Shutdown Signal.
	waitForShutdownSignal(ctx, sigChan, logger)

	// Graceful Shutdown.
	// Corrected: Use ValidatorInterface.
	return performGracefulShutdown(shutdownTimeout, server, validator, startTime, logger)
}

// setupLoggingAndConfig initializes the logger and loads configuration.
func setupLoggingAndConfig(configPath string, debug bool) (logging.Logger, *config.Config, error) {
	logLevel := "info"
	if debug {
		logLevel = "debug"
	}
	logging.SetupDefaultLogger(logLevel)
	logger := logging.GetLogger("server_runner")

	var cfg *config.Config
	var err error
	if configPath != "" {
		logger.Info("📂 Loading configuration from file.", "config_path", configPath)
		cfg, err = config.LoadFromFile(configPath)
		if err != nil {
			logger.Error("❌ Failed to load configuration.", "config_path", configPath, "error", err.Error())
			return logger, nil, errors.Wrap(err, "failed to load configuration from file")
		}
		logger.Info("✅ Configuration loaded successfully.", "config_path", configPath)
	} else {
		logger.Info("📝 Using default configuration (no config file specified).")
		cfg = config.DefaultConfig()
	}
	logger.Info("⚙️ Configuration ready.")
	return logger, cfg, nil
}

// initializeServices sets up the schema validator and RTM service.
// Corrected: Returns schema.ValidatorInterface.
func initializeServices(ctx context.Context, cfg *config.Config, logger logging.Logger) (schema.ValidatorInterface, *rtm.Service, error) {
	validator, err := initializeSchemaValidator(ctx, cfg.Schema, logger)
	if err != nil {
		return nil, nil, err // Error logged within initializeSchemaValidator.
	}

	rtmService, err := initializeRTMService(ctx, cfg, logger)
	if err != nil {
		if shutdownErr := validator.Shutdown(); shutdownErr != nil {
			logger.Error("⚠️ Error shutting down schema validator during RTM init failure.", "error", shutdownErr)
		}
		return nil, nil, err // Error logged within initializeRTMService.
	}

	return validator, rtmService, nil
}

// initializeSchemaValidator sets up the schema validator.
// Corrected: Returns schema.ValidatorInterface.
func initializeSchemaValidator(ctx context.Context, schemaCfg config.SchemaConfig, logger logging.Logger) (schema.ValidatorInterface, error) {
	logger.Info("📋 Initializing schema validator.")
	// Corrected: Use NewValidator.
	schemaLogger := logger.WithField("component", "schema_validator")
	validator := schema.NewValidator(schemaCfg, schemaLogger)

	logger.Info("🔄 Initializing schema validator - this might take a moment.")
	if err := validator.Initialize(ctx); err != nil {
		logger.Error("❌ Schema validator initialization failed.",
			"error", err.Error(),
			"advice", "Check schema override URI or embedded schema content.")
		if os.IsNotExist(errors.Cause(err)) {
			logger.Error("💡 Override schema file not found.", "uri", schemaCfg.SchemaOverrideURI)
		}
		return nil, errors.Wrap(err, "failed to initialize schema validator")
	}
	logger.Info("✅ Schema validator initialized successfully.",
		"schema_version", validator.GetSchemaVersion())
	return validator, nil
}

// initializeRTMService creates and initializes the RTM service, performing diagnostics.
// (No changes needed in this function's logic related to validator rename).
func initializeRTMService(ctx context.Context, cfg *config.Config, logger logging.Logger) (*rtm.Service, error) {
	logger.Info("🔄 Creating and initializing RTM service.")
	rtmFactory := rtm.NewServiceFactory(cfg, logging.GetLogger("rtm_factory"))
	rtmService, err := rtmFactory.CreateService(ctx)
	if err != nil {
		logger.Error("❌ RTM service initialization failed.", "error", err.Error())
		return nil, errors.Wrap(err, "failed to initialize RTM service")
	}
	logger.Info("🔍 Performing RTM connectivity diagnostics.")
	options := rtm.DefaultConnectivityCheckOptions()
	diagResults, diagErr := rtmService.PerformConnectivityCheck(ctx, options)
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
	if diagErr != nil {
		logger.Error("❌ RTM connectivity diagnostics failed.", "error", diagErr.Error())
		logger.Error("💡 RTM connectivity troubleshooting tips:",
			"tip1", "Verify your RTM API key and shared secret are correct (check env vars RTM_API_KEY, RTM_SHARED_SECRET or config file).",
			"tip2", "Check your internet connection.",
			"tip3", "Ensure the RTM API is accessible from your network.")
		return nil, errors.Wrap(diagErr, "failed to verify RTM connectivity")
	}
	if rtmService.IsAuthenticated() {
		logger.Info("🔑 RTM authentication successful.",
			"username", rtmService.GetUsername(),
			"status", "Ready to access tasks and lists.")
	} else {
		logger.Warn("⚠️ Not authenticated with RTM.",
			"status", "Limited functionality available.",
			"advice", "Use authentication tools (e.g., rtm_connection_test or Claude Desktop) to connect.")
	}
	return rtmService, nil
}

// createMCPServer creates the MCP server instance.
// Corrected: Accepts schema.ValidatorInterface.
func createMCPServer(cfg *config.Config, requestTimeout, shutdownTimeout time.Duration, debug bool, validator schema.ValidatorInterface, startTime time.Time, logger logging.Logger) (*mcp.Server, error) {
	logger.Info("🔄 Creating MCP server instance.")
	opts := mcp.ServerOptions{
		RequestTimeout:  requestTimeout,
		ShutdownTimeout: shutdownTimeout,
		Debug:           debug,
	}
	server, err := mcp.NewServer(cfg, opts, validator, startTime, logger)
	if err != nil {
		logger.Error("❌ Failed to create MCP server.", "error", err.Error())
		return nil, errors.Wrap(err, "failed to create MCP server")
	}
	return server, nil
}

// startServerTransport selects and starts the appropriate server transport.
// (No changes needed in this function's logic related to validator rename).
func startServerTransport(ctx context.Context, transportType string, cfg *config.Config, server *mcp.Server, cancel context.CancelFunc, logger logging.Logger) error {
	switch transportType {
	case "stdio":
		logger.Info("📡 Starting server with stdio transport.",
			"description", "Communication via standard input/output.")
		go runTransportLoop(ctx, cancel, logger, "stdio", func(innerCtx context.Context) error {
			return server.ServeSTDIO(innerCtx)
		})
		return nil // Return nil as the loop runs in a goroutine.
	case "http":
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		logger.Info("📡 Starting server with HTTP transport.",
			"address", addr,
			"description", "Communication via HTTP protocol.")
		go runTransportLoop(ctx, cancel, logger, "http", func(innerCtx context.Context) error {
			// Note: ServeHTTP is not implemented, so this path currently does nothing.
			if err := server.ServeHTTP(innerCtx, addr); !errors.Is(err, errors.New("HTTP transport not implemented")) {
				return err
			}
			return nil
		})
		return nil // Return nil as the loop runs in a goroutine.
	default:
		logger.Error("❌ Unsupported transport type.",
			"transport", transportType,
			"supported", "stdio, http",
			"advice", "Use --transport stdio or --transport http.")
		return errors.Newf("unsupported transport type: %s", transportType)
	}
}

// runTransportLoop runs the server's serve function in a goroutine and handles its exit.
// (No changes needed in this function's logic).
func runTransportLoop(ctx context.Context, cancel context.CancelFunc, logger logging.Logger, transportName string, serveFunc func(context.Context) error) {
	if err := serveFunc(ctx); err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.Error(fmt.Sprintf("❌ Server error (%s).", transportName), "error", fmt.Sprintf("%+v", err))
		} else {
			logger.Info(fmt.Sprintf("🛑 Server stopped gracefully (%s).", transportName), "reason", err)
		}
	} else {
		logger.Info(fmt.Sprintf("🛑 Server stopped normally (%s).", transportName))
	}
	cancel() // Ensure context is canceled on any server error or normal stop.
}

// waitForShutdownSignal blocks until a shutdown signal is received or the context is cancelled.
// (No changes needed in this function's logic).
func waitForShutdownSignal(ctx context.Context, sigChan <-chan os.Signal, logger logging.Logger) {
	select {
	case sig := <-sigChan:
		logger.Info("⏹️ Received termination signal.", "signal", sig)
	case <-ctx.Done():
		logger.Info("⏹️ Context cancelled, initiating shutdown.", "reason", ctx.Err())
	}
}

// performGracefulShutdown handles the shutdown sequence for the server and validator.
// Corrected: Accepts schema.ValidatorInterface.
func performGracefulShutdown(shutdownTimeout time.Duration, server *mcp.Server, validator schema.ValidatorInterface, startTime time.Time, logger logging.Logger) error {
	logger.Info("🔄 Shutting down server gracefully.",
		"timeout", shutdownTimeout.String())
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	logger.Debug("Shutting down schema validator.")
	if err := validator.Shutdown(); err != nil {
		logger.Error("⚠️ Error shutting down schema validator.", "error", err)
	} else {
		logger.Debug("Schema validator shut down successfully.")
	}

	logger.Debug("Shutting down MCP server.")
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("❌ Server shutdown error.", "error", err.Error())
	} else {
		logger.Debug("MCP server shut down successfully.")
	}

	logger.Info("👋 Server shutdown complete - goodbye.",
		"run_duration", time.Since(startTime).Round(time.Millisecond).String())
	return nil
}
