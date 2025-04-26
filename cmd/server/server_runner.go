// Package server provides the runner and setup logic for the main CowGnition MCP server process.
// It handles server lifecycle management including initialization, runtime operation,.
// and graceful shutdown. The package integrates various components such as configuration.
// loading, schema validation, RTM service connectivity diagnostics, and transport-specific.
// server implementations (stdio, http). It also manages signal handling for proper.
// server termination and resource cleanup.
// file: cmd/server/server_runner.go
package server

import (
	"context"
	"encoding/json" // Added for core handler response marshaling.
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"     // Added.
	"github.com/dkoosis/cowgnition/internal/logging"    // Added.
	"github.com/dkoosis/cowgnition/internal/mcp"        // Added for core handlers.
	"github.com/dkoosis/cowgnition/internal/mcp/router" // Added.
	"github.com/dkoosis/cowgnition/internal/mcp/state"  // Added.
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
	"github.com/dkoosis/cowgnition/internal/rtm"
	"github.com/dkoosis/cowgnition/internal/schema"
	// Added for GetName().
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
	validator, rtmService, err := initializeServices(ctx, cfg, logger) // <-- rtmService is returned here.
	if err != nil {
		return err // Errors already logged within initializeServices.
	}
	defer func() {
		if shutdownErr := validator.Shutdown(); shutdownErr != nil {
			logger.Error("⚠️ Error shutting down schema validator during potential early exit.", "error", shutdownErr)
		}
	}()

	// --- Create FSM and Router ---.
	mcpFSM, err := state.NewMCPStateMachine(logger.WithField("component", "mcp_fsm"))
	if err != nil {
		logger.Error("❌ Failed to create MCP state machine.", "error", err)
		return errors.Wrap(err, "failed to create MCP state machine")
	}
	mcpRouter := router.NewRouter(logger.WithField("component", "mcp_router"))
	// --- End Create FSM and Router ---.

	// MCP Server Creation.
	// --- MODIFIED: Call mcp.NewServer directly with FSM and Router ---.
	opts := mcp.ServerOptions{
		RequestTimeout:  requestTimeout,
		ShutdownTimeout: shutdownTimeout,
		Debug:           debug,
	}
	server, err := mcp.NewServer(cfg, opts, validator, mcpFSM, mcpRouter, startTime, logger) // Pass FSM and Router.
	if err != nil {
		logger.Error("❌ Failed to create MCP server.", "error", err.Error())
		return errors.Wrap(err, "failed to create MCP server")
	}
	// --- END MODIFICATION ---.

	// Register RTM Service (if available).
	if rtmService != nil {
		logger.Info("🔧 Registering RTM service with MCP server.")
		if err := server.RegisterService(rtmService); err != nil {
			logger.Error("❌ Failed to register RTM service.", "error", err)
			return errors.Wrap(err, "failed to register RTM service")
		}
	} else {
		logger.Warn("⚠️ RTM Service instance was nil after initialization, cannot register.")
	}

	// --- Register Core MCP Routes AFTER server creation ---.
	if err := registerCoreRoutes(server); err != nil { // Pass the server instance.
		logger.Error("❌ Failed to register core MCP routes.", "error", err)
		return err
	}
	// --- End Register Core MCP Routes ---.

	// Start Server Transport.
	if err := startServerTransport(ctx, transportType, cfg, server, cancel, logger); err != nil {
		return err // Error already logged.
	}

	logger.Info("✅ Server startup complete and ready to process requests.",
		"startup_time_ms", time.Since(startTime).Milliseconds())

	// Wait for Shutdown Signal.
	waitForShutdownSignal(ctx, sigChan, logger)

	// Graceful Shutdown.
	return performGracefulShutdown(shutdownTimeout, server, validator, startTime, logger)
}

// --- Helper Function to Register Core Routes ---.
// This function encapsulates the registration of core MCP methods with the router.
func registerCoreRoutes(server *mcp.Server) error {
	coreRouter := server.GetRouter() // Use the new getter method.
	if coreRouter == nil {
		return errors.New("cannot register core routes: server router is nil")
	}
	logger := server.GetLogger().WithField("subcomponent", "core_routes") // Use the new getter method.

	// --- Ping ---.
	err := coreRouter.AddRoute(router.Route{
		Method: "ping",
		// --- MODIFIED: Rename unused ctx to _ ---.
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			logger.Debug("Handling ping request.")
			return json.RawMessage(`{}`), nil
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to register ping route")
	}

	// --- Initialize ---.
	err = coreRouter.AddRoute(router.Route{
		Method: "initialize",
		// --- MODIFIED: Rename unused ctx to _ ---.
		Handler: func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
			logger.Info("Handling initialize request via router.")
			var req mcptypes.InitializeRequest
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, errors.Wrap(err, "failed to unmarshal initialize request parameters")
			}

			// Log Client Info using the new method on server.
			server.LogClientInfo(&req.ClientInfo, &req.Capabilities)

			serverProtocolVersion := "2024-11-05" // Forced version.
			logger.Warn("Forcing server protocol version.", "serverVersion", serverProtocolVersion)

			// Aggregate capabilities using the new method on server.
			caps := server.AggregateServerCapabilities()

			// Get server info using the new method on server.
			appVersion := "0.1.0-dev" // TODO: Get from build flags.
			serverInfo := mcptypes.Implementation{Name: server.GetConfig().Server.Name, Version: appVersion}

			res := mcptypes.InitializeResult{
				ProtocolVersion: serverProtocolVersion,
				ServerInfo:      &serverInfo,
				Capabilities:    caps,
			}

			resBytes, err := json.Marshal(res)
			if err != nil {
				return nil, errors.Wrap(err, "failed to marshal initialize result")
			}

			logger.Info("Initialize successful, returning server capabilities.")
			return resBytes, nil
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to register initialize route")
	}

	// --- Shutdown ---.
	err = coreRouter.AddRoute(router.Route{
		Method: "shutdown",
		// --- MODIFIED: Rename unused ctx to _ ---.
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			logger.Info("Handling shutdown request via router.")
			logger.Info("Server state transition to ShuttingDown acknowledged.")
			return json.RawMessage(`null`), nil
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to register shutdown route")
	}

	// --- Exit ---.
	err = coreRouter.AddRoute(router.Route{
		Method: "exit",
		// --- MODIFIED: Rename unused ctx to _ ---.
		NotificationHandler: func(_ context.Context, _ json.RawMessage) error {
			logger.Info("Handling exit notification via router.")
			logger.Warn("Exit notification received. Server should terminate process.")
			// Actual cancellation handled by server loop exit.
			return nil
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to register exit route")
	}

	// --- notifications/initialized ---.
	err = coreRouter.AddRoute(router.Route{
		Method: "notifications/initialized",
		// --- MODIFIED: Rename unused ctx to _ ---.
		NotificationHandler: func(_ context.Context, params json.RawMessage) error {
			logger.Info("Handling notifications/initialized notification via router.")
			var notifParams map[string]interface{} // Generic map.
			if err := json.Unmarshal(params, &notifParams); err != nil {
				logger.Debug("Could not parse notifications/initialized params.", "error", err)
			} else {
				logger.Debug("Parsed notifications/initialized params.", "params", fmt.Sprintf("%+v", notifParams))
			}
			logger.Info("Client initialization acknowledged. State is now Initialized.")
			return nil
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to register notifications/initialized route")
	}

	// --- $/cancelRequest ---.
	err = coreRouter.AddRoute(router.Route{
		Method: "$/cancelRequest",
		// --- MODIFIED: Rename unused ctx to _ ---.
		NotificationHandler: func(_ context.Context, params json.RawMessage) error {
			var reqParams struct {
				ID json.RawMessage `json:"id"`
			}
			if err := json.Unmarshal(params, &reqParams); err != nil {
				logger.Warn("Failed to unmarshal $/cancelRequest params.", "error", err)
				return nil
			}
			logger.Info("Received cancellation request notification.", "requestId", string(reqParams.ID))
			// TODO: Implement actual request cancellation logic.
			return nil
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to register $/cancelRequest route")
	}

	logger.Info("✅ Core MCP routes registered.")
	return nil
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
func initializeServices(ctx context.Context, cfg *config.Config, logger logging.Logger) (schema.ValidatorInterface, *rtm.Service, error) {
	validator, err := initializeSchemaValidator(ctx, cfg.Schema, logger)
	if err != nil {
		return nil, nil, err
	}

	rtmService, err := initializeRTMService(ctx, cfg, logger)
	if err != nil {
		if shutdownErr := validator.Shutdown(); shutdownErr != nil {
			logger.Error("⚠️ Error shutting down schema validator during RTM init failure.", "error", shutdownErr)
		}
		return nil, nil, err
	}

	return validator, rtmService, nil
}

// initializeSchemaValidator sets up the schema validator.
func initializeSchemaValidator(ctx context.Context, schemaCfg config.SchemaConfig, logger logging.Logger) (schema.ValidatorInterface, error) {
	logger.Info("📋 Initializing schema validator.")
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

// startServerTransport selects and starts the appropriate server transport.
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
	cancel()
}

// waitForShutdownSignal blocks until a shutdown signal is received or the context is cancelled.
func waitForShutdownSignal(ctx context.Context, sigChan <-chan os.Signal, logger logging.Logger) {
	select {
	case sig := <-sigChan:
		logger.Info("⏹️ Received termination signal.", "signal", sig)
	case <-ctx.Done():
		logger.Info("⏹️ Context cancelled, initiating shutdown.", "reason", ctx.Err())
	}
}

// performGracefulShutdown handles the shutdown sequence for the server and validator.
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
