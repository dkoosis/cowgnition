// Package server provides the runner and setup logic for the main CowGnition MCP server process.
// It handles server lifecycle management including initialization, runtime operation,
// and graceful shutdown.
// file: cmd/server/server_runner.go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/mcp"
	"github.com/dkoosis/cowgnition/internal/mcp/router"
	"github.com/dkoosis/cowgnition/internal/mcp/state"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types"
	"github.com/dkoosis/cowgnition/internal/rtm"
	"github.com/dkoosis/cowgnition/internal/schema"
	"github.com/dkoosis/cowgnition/internal/transport"
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
	validator, rtmService, err := initializeServices(ctx, cfg, logger)
	if err != nil {
		return err // Errors already logged within initializeServices.
	}
	defer func() {
		if shutdownErr := validator.Shutdown(); shutdownErr != nil {
			logger.Error("⚠️ Error shutting down schema validator during potential early exit.", "error", shutdownErr)
		}
	}()

	// Create FSM and Router.
	mcpFSM, err := state.NewMCPStateMachine(logger.WithField("component", "mcp_fsm"))
	if err != nil {
		logger.Error("❌ Failed to create MCP state machine.", "error", err)
		return errors.Wrap(err, "failed to create MCP state machine")
	}
	mcpRouter := router.NewRouter(logger.WithField("component", "mcp_router"))

	// MCP Server Creation.
	opts := mcp.ServerOptions{
		RequestTimeout:  requestTimeout,
		ShutdownTimeout: shutdownTimeout,
		Debug:           debug,
	}
	server, err := mcp.NewServer(cfg, opts, validator, mcpFSM, mcpRouter, startTime, logger)
	if err != nil {
		logger.Error("❌ Failed to create MCP server.", "error", err.Error())
		return errors.Wrap(err, "failed to create MCP server")
	}

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

	// Register Core MCP Routes.
	logger.Debug(">>> Runner: About to register core routes...") // <<< ADDED LOG
	if err := registerCoreRoutes(server); err != nil {
		logger.Error("❌ Failed to register core MCP routes.", "error", err)
		return err
	}
	logger.Debug(">>> Runner: Finished registering core routes.") // <<< ADDED LOG

	// Start Server Transport.
	logger.Debug(">>> Runner: About to start server transport...") // <<< ADDED LOG
	if err := startServerTransport(ctx, transportType, cfg, server, cancel, logger); err != nil {
		// <<< ADDED LOGGING FOR ERROR CASE >>>
		logger.Error(">>> Runner: startServerTransport returned an error.", "error", err)
		return err // Error already logged within startServerTransport potentially.
	}
	logger.Debug(">>> Runner: startServerTransport finished.") // <<< ADDED LOG

	logger.Info("✅ Server startup complete and ready to process requests.",
		"startup_time_ms", time.Since(startTime).Milliseconds())

	// Wait for Shutdown Signal.
	waitForShutdownSignal(ctx, sigChan, logger)

	// Graceful Shutdown.
	return performGracefulShutdown(shutdownTimeout, server, validator, startTime, logger)
}

// registerCoreRoutes encapsulates the registration of core MCP methods with the router.
func registerCoreRoutes(server *mcp.Server) error {
	coreRouter := server.GetRouter()
	if coreRouter == nil {
		return errors.New("cannot register core routes: server router is nil")
	}
	logger := server.GetLogger().WithField("subcomponent", "core_routes")
	logger.Debug(">>> registerCoreRoutes: Starting...") // <<< ADDED LOG

	// --- Ping ---.
	logger.Debug(">>> registerCoreRoutes: Registering 'ping'...") // <<< ADDED LOG
	err := coreRouter.AddRoute(router.Route{
		Method: "ping",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			logger.Debug("Handling ping request.")
			return json.RawMessage(`{}`), nil
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to register ping route")
	}

	// --- Initialize ---.
	logger.Debug(">>> registerCoreRoutes: Registering 'initialize'...") // <<< ADDED LOG
	err = coreRouter.AddRoute(router.Route{
		Method: "initialize",
		Handler: func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
			logger.Info("Handling initialize request via router.")
			var req mcptypes.InitializeRequest
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, errors.Wrap(err, "failed to unmarshal initialize request parameters")
			}

			// Log Client Info
			server.LogClientInfo(&req.ClientInfo, &req.Capabilities)

			serverProtocolVersion := "2024-11-05" // Forced version.
			logger.Warn("Forcing server protocol version.", "serverVersion", serverProtocolVersion)

			// Aggregate capabilities
			caps := server.AggregateServerCapabilities()

			// Get server info
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
	logger.Debug(">>> registerCoreRoutes: Registering 'shutdown'...") // <<< ADDED LOG
	err = coreRouter.AddRoute(router.Route{
		Method: "shutdown",
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
	logger.Debug(">>> registerCoreRoutes: Registering 'exit'...") // <<< ADDED LOG
	err = coreRouter.AddRoute(router.Route{
		Method: "exit",
		NotificationHandler: func(_ context.Context, _ json.RawMessage) error {
			logger.Info("Handling exit notification via router.")
			logger.Warn("Exit notification received. Server should terminate process.")
			return nil
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to register exit route")
	}

	// --- notifications/initialized ---.
	logger.Debug(">>> registerCoreRoutes: Registering 'notifications/initialized'...") // <<< ADDED LOG
	err = coreRouter.AddRoute(router.Route{
		Method: "notifications/initialized",
		NotificationHandler: func(_ context.Context, params json.RawMessage) error {
			logger.Info("Handling notifications/initialized notification via router.")
			var notifParams map[string]interface{}
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
	logger.Debug(">>> registerCoreRoutes: Registering '$/cancelRequest'...") // <<< ADDED LOG
	err = coreRouter.AddRoute(router.Route{
		Method: "$/cancelRequest",
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

	// --- tools/list ---.
	logger.Debug(">>> registerCoreRoutes: Registering 'tools/list'...") // <<< ADDED LOG
	err = coreRouter.AddRoute(router.Route{
		Method: "tools/list",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) { // FIX: Renamed params to _
			logger.Info("Handling tools/list request via router.")
			// Get all services
			allServices := server.GetAllServices()
			allTools := []mcptypes.Tool{}

			// Collect tools from all services
			for _, svc := range allServices {
				tools := svc.GetTools()
				allTools = append(allTools, tools...)
			}

			// Build response
			result := mcptypes.ListToolsResult{
				Tools: allTools,
				// NextCursor is only needed for pagination
			}

			resBytes, err := json.Marshal(result)
			if err != nil {
				return nil, errors.Wrap(err, "failed to marshal tools/list result")
			}

			logger.Info("Successfully listed tools", "count", len(allTools))
			return resBytes, nil
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to register tools/list route")
	}

	// --- resources/list ---.
	logger.Debug(">>> registerCoreRoutes: Registering 'resources/list'...") // <<< ADDED LOG
	err = coreRouter.AddRoute(router.Route{
		Method: "resources/list",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) { // FIX: Renamed params to _
			logger.Info("Handling resources/list request via router.")
			// Get all services
			allServices := server.GetAllServices()
			allResources := []mcptypes.Resource{}

			// Collect resources from all services
			for _, svc := range allServices {
				resources := svc.GetResources()
				allResources = append(allResources, resources...)
			}

			// Add system resources
			systemResources := []mcptypes.Resource{
				{
					Name:        "Server Health",
					URI:         "cowgnition://health",
					Description: "Server health and diagnostic information",
					MimeType:    "application/json",
				},
			}
			allResources = append(allResources, systemResources...)

			// Build response
			result := mcptypes.ListResourcesResult{
				Resources: allResources,
				// NextCursor is only needed for pagination
			}

			resBytes, err := json.Marshal(result)
			if err != nil {
				return nil, errors.Wrap(err, "failed to marshal resources/list result")
			}

			logger.Info("Successfully listed resources", "count", len(allResources))
			return resBytes, nil
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to register resources/list route")
	}

	// --- resources/read ---.
	logger.Debug(">>> registerCoreRoutes: Registering 'resources/read'...") // <<< ADDED LOG
	err = coreRouter.AddRoute(router.Route{
		Method: "resources/read",
		Handler: func(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
			logger.Info("Handling resources/read request via router.")

			var req mcptypes.ReadResourceRequest
			if err := json.Unmarshal(params, &req); err != nil {
				return nil, errors.Wrap(err, "failed to unmarshal resources/read parameters")
			}

			uri := req.URI
			logger.Debug("Resource read request", "uri", uri)

			// Special handling for system resources
			if uri == "cowgnition://health" {
				contents, err := server.ReadServerHealthMetrics(ctx)
				if err != nil {
					return nil, errors.Wrap(err, "failed to read server health metrics")
				}

				result := mcptypes.ReadResourceResult{
					Contents: contents,
				}

				resBytes, err := json.Marshal(result)
				if err != nil {
					return nil, errors.Wrap(err, "failed to marshal resources/read result")
				}

				return resBytes, nil
			}

			// Delegate to appropriate service based on URI scheme
			var serviceResources []interface{}
			var serviceErr error

			for _, svc := range server.GetAllServices() {
				resources := svc.GetResources()
				for _, res := range resources {
					if res.URI == uri {
						serviceResources, serviceErr = svc.ReadResource(ctx, uri)
						if serviceErr != nil {
							return nil, errors.Wrapf(serviceErr, "service %s failed to read resource %s",
								svc.GetName(), uri)
						}
						break
					}
				}
				if serviceResources != nil {
					break
				}
			}

			if serviceResources == nil {
				return nil, errors.Newf("resource not found: %s", uri)
			}

			result := mcptypes.ReadResourceResult{
				Contents: serviceResources,
			}

			resBytes, err := json.Marshal(result)
			if err != nil {
				return nil, errors.Wrap(err, "failed to marshal resources/read result")
			}

			return resBytes, nil
		},
	})
	if err != nil {
		return errors.Wrap(err, "failed to register resources/read route")
	}

	logger.Info("✅ Core MCP routes registered.")
	logger.Debug(">>> registerCoreRoutes: Finished.") // <<< ADDED LOG
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
	logger.Debug(">>> startServerTransport: Entering function.", "transportType", transportType) // <<< ADDED LOG
	switch transportType {
	case "stdio":
		logger.Info("📡 Starting server with stdio transport.",
			"description", "Communication via standard input/output.")
		logger.Debug(">>> startServerTransport: Launching runTransportLoop goroutine for stdio.") // <<< ADDED LOG
		go runTransportLoop(ctx, cancel, logger, "stdio", func(innerCtx context.Context) error {
			logger.Debug(">>> startServerTransport (goroutine): Calling server.ServeSTDIO...") // <<< ADDED LOG
			err := server.ServeSTDIO(innerCtx)
			logger.Debug(">>> startServerTransport (goroutine): server.ServeSTDIO returned.", "error", err) // <<< ADDED LOG
			return err
		})
		logger.Debug(">>> startServerTransport: Returning nil (stdio goroutine launched).") // <<< ADDED LOG
		return nil
	case "http":
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		logger.Info("📡 Starting server with HTTP transport.",
			"address", addr,
			"description", "Communication via HTTP protocol.")
		logger.Debug(">>> startServerTransport: Launching runTransportLoop goroutine for http.") // <<< ADDED LOG
		go runTransportLoop(ctx, cancel, logger, "http", func(innerCtx context.Context) error {
			logger.Debug(">>> startServerTransport (goroutine): Calling server.ServeHTTP...") // <<< ADDED LOG
			err := server.ServeHTTP(innerCtx, addr)
			// <<< ADJUSTED LOGIC TO CHECK FOR SPECIFIC ERROR >>>
			if err != nil && !errors.Is(err, errors.New("HTTP transport not implemented")) {
				logger.Debug(">>> startServerTransport (goroutine): server.ServeHTTP returned error.", "error", err) // <<< ADDED LOG
				return err
			}
			logger.Debug(">>> startServerTransport (goroutine): server.ServeHTTP finished (or not implemented).") // <<< ADDED LOG
			return nil                                                                                            // Return nil if it's the "not implemented" error
		})
		logger.Debug(">>> startServerTransport: Returning nil (http goroutine launched).") // <<< ADDED LOG
		return nil
	default:
		logger.Error("❌ Unsupported transport type.",
			"transport", transportType,
			"supported", "stdio, http",
			"advice", "Use --transport stdio or --transport http.")
		err := errors.Newf("unsupported transport type: %s", transportType)
		logger.Debug(">>> startServerTransport: Returning error for unsupported type.", "error", err) // <<< ADDED LOG
		return err
	}
}

// runTransportLoop runs the server's serve function in a goroutine and handles its exit.
func runTransportLoop(ctx context.Context, cancel context.CancelFunc, logger logging.Logger, transportName string, serveFunc func(context.Context) error) {
	logger.Debug(">>> runTransportLoop: Goroutine started.", "transport", transportName) // <<< ADDED LOG
	if err := serveFunc(ctx); err != nil {
		// Check if the error is context cancellation or deadline exceeded
		isContextError := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
		// Check if the error indicates the transport was closed (using transport helper)
		isClosedError := transport.IsClosedError(err)

		if isContextError || isClosedError {
			logger.Info(fmt.Sprintf("🛑 Server stopped gracefully (%s).", transportName), "reason", err)
		} else {
			// Log other errors more prominently
			logger.Error(fmt.Sprintf("❌ Server error (%s).", transportName), "error", fmt.Sprintf("%+v", err))
		}
	} else {
		logger.Info(fmt.Sprintf("🛑 Server stopped normally (%s).", transportName))
	}
	logger.Debug(">>> runTransportLoop: Calling cancel().", "transport", transportName)   // <<< ADDED LOG
	cancel()                                                                              // Ensure context is canceled to potentially unblock main thread
	logger.Debug(">>> runTransportLoop: Goroutine finished.", "transport", transportName) // <<< ADDED LOG
}

// waitForShutdownSignal blocks until a shutdown signal is received or the context is cancelled.
func waitForShutdownSignal(ctx context.Context, sigChan <-chan os.Signal, logger logging.Logger) {
	logger.Debug(">>> waitForShutdownSignal: Waiting for signal or context cancellation...") // <<< ADDED LOG
	select {
	case sig := <-sigChan:
		logger.Info("⏹️ Received termination signal.", "signal", sig)
	case <-ctx.Done():
		logger.Info("⏹️ Context cancelled, initiating shutdown.", "reason", ctx.Err())
	}
	logger.Debug(">>> waitForShutdownSignal: Finished waiting.") // <<< ADDED LOG
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
