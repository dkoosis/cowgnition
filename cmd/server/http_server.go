// file: cmd/server/http_server.go
package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/mcp"
)

// RunServer starts the MCP server with the specified transport type.
// It handles setup, startup, and graceful shutdown of the server.
//
// transportType string: The type of transport to use ("http" or "stdio").
// configPath string: Path to the configuration file.
// requestTimeout time.Duration: Timeout for request processing.
// shutdownTimeout time.Duration: Timeout for graceful shutdown.
// debug bool: Enable debug mode for verbose logging.
//
// Returns:
//
//	error: An error if the server fails to start or encounters a fatal error.
func RunServer(transportType, configPath string, requestTimeout, shutdownTimeout time.Duration, debug bool) error {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Load configuration
	var cfg *config.Config
	var err error

	if configPath != "" {
		// Load from specified path
		cfg, err = config.LoadFromFile(configPath)
		if err != nil {
			return errors.Wrap(err, "failed to load configuration from file")
		}
	} else {
		// Use default configuration
		cfg = config.DefaultConfig()
	}

	if debug {
		log.Printf("Server configuration: %+v", cfg)
	}

	// Create MCP server options
	opts := mcp.ServerOptions{
		RequestTimeout:  requestTimeout,
		ShutdownTimeout: shutdownTimeout,
		Debug:           debug,
	}

	// Create MCP server instance
	server, err := mcp.NewServer(cfg, opts)
	if err != nil {
		return errors.Wrap(err, "failed to create MCP server")
	}

	// Start the server based on transport type
	switch transportType {
	case "stdio":
		if debug {
			log.Println("Starting server with stdio transport")
		}
		go func() {
			if err := server.ServeSTDIO(ctx); err != nil {
				log.Printf("Server error: %v", err)
				cancel() // Cancel context to trigger shutdown
			}
		}()

	case "http":
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		if debug {
			log.Printf("Starting server with HTTP transport on %s", addr)
		}
		go func() {
			if err := server.ServeHTTP(ctx, addr); err != nil {
				log.Printf("Server error: %v", err)
				cancel() // Cancel context to trigger shutdown
			}
		}()

	default:
		return errors.Newf("unsupported transport type: %s", transportType)
	}

	// Wait for signal or context cancellation
	select {
	case sig := <-sigChan:
		log.Printf("Received signal: %v", sig)
	case <-ctx.Done():
		log.Println("Context cancelled")
	}

	// Initiate graceful shutdown
	log.Println("Shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return errors.Wrap(err, "server shutdown error")
	}

	log.Println("Server shutdown complete")
	return nil
}
