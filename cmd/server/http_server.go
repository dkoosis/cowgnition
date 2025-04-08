// file: cmd/server/http_server.go
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
)

// RunServer starts the MCP server with the specified transport type.
// It handles setup, startup, and graceful shutdown of the server.
func RunServer(transportType, configPath string, requestTimeout, shutdownTimeout time.Duration, debug bool) error {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Set up logging
	logLevel := "info"
	if debug {
		logLevel = "debug"
	}
	logging.SetupDefaultLogger(logLevel)
	logger := logging.GetLogger("server")

	logger.Info("Starting CowGnition server",
		"transportType", transportType,
		"debug", debug)

	// Load configuration
	var cfg *config.Config
	var err error

	if configPath != "" {
		// Load from specified path
		logger.Info("Loading configuration from file", "path", configPath)
		cfg, err = config.LoadFromFile(configPath)
		if err != nil {
			return errors.Wrap(err, "failed to load configuration from file")
		}
	} else {
		// Use default configuration
		logger.Info("Using default configuration")
		cfg = config.DefaultConfig()
	}

	if debug {
		logger.Debug("Server configuration",
			"serverName", cfg.Server.Name,
			"port", cfg.Server.Port)
	}

	// Create MCP server options
	opts := mcp.ServerOptions{
		RequestTimeout:  requestTimeout,
		ShutdownTimeout: shutdownTimeout,
		Debug:           debug,
	}

	// Create MCP server instance
	server, err := mcp.NewServer(cfg, opts, logger)
	if err != nil {
		return errors.Wrap(err, "failed to create MCP server")
	}

	// Start the server based on transport type
	switch transportType {
	case "stdio":
		logger.Info("Starting server with stdio transport")
		go func() {
			if err := server.ServeSTDIO(ctx); err != nil {
				logger.Error("Server error", "error", err)
				cancel() // Cancel context to trigger shutdown
			}
		}()

	case "http":
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		logger.Info("Starting server with HTTP transport", "address", addr)
		go func() {
			if err := server.ServeHTTP(ctx, addr); err != nil {
				logger.Error("Server error", "error", err)
				cancel() // Cancel context to trigger shutdown
			}
		}()

	default:
		return errors.Newf("unsupported transport type: %s", transportType)
	}

	// Wait for signal or context cancellation
	select {
	case sig := <-sigChan:
		logger.Info("Received signal", "signal", sig)
	case <-ctx.Done():
		logger.Info("Context cancelled")
	}

	// Initiate graceful shutdown
	logger.Info("Shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return errors.Wrap(err, "server shutdown error")
	}

	logger.Info("Server shutdown complete")
	return nil
}
