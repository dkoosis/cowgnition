// Package main provides the entry point for the CowGnition MCP server.
// file: cmd/server/main.go
package main

import (
	"flag"      // Command-line flag parsing.
	"log"       // Standard logging for output.
	"os"        // OS-level interactions like environment variables.
	"os/signal" // Signal handling for graceful shutdown.
	"syscall"   // System call interface for signals.
	"time"      // Time-related functionality.

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config" // Configuration loading.
	"github.com/dkoosis/cowgnition/internal/mcp"    // MCP server logic.
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/dkoosis/cowgnition/internal/rtm" // RTM authentication.
)

func main() {
	// Parse command-line flags
	transportType := flag.String("transport", "http", "Transport type (http or stdio)")
	configPath := flag.String("config", "", "Path to configuration file")
	requestTimeout := flag.Duration("request-timeout", 30*time.Second, "Timeout for JSON-RPC requests")
	shutdownTimeout := flag.Duration("shutdown-timeout", 5*time.Second, "Timeout for graceful shutdown")
	flag.Parse()

	// Load configuration.
	// This is done early to ensure settings are available.
	cfg := config.New()

	// TODO: Load configuration from file if specified
	if *configPath != "" {
		log.Printf("Configuration file loading not yet implemented. Using default configuration.")
	}

	// Create and start MCP server.
	// The server orchestrates MCP communication.
	server, err := mcp.NewServer(cfg)
	if err != nil {
		// Use better error logging with stack trace
		log.Fatalf("main: failed to create server: %+v", err)
	}

	// Set version.
	// Provides server identification to clients.
	server.SetVersion("1.0.0")

	// Set transport type.
	// Determines how the server communicates (HTTP or stdio).
	if err := server.SetTransport(*transportType); err != nil {
		err = cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to set transport"),
			cgerr.CategoryConfig,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"transport_type": *transportType,
				"valid_types":    []string{"http", "stdio"},
			},
		)
		// Use better error logging with stack trace
		log.Fatalf("main: %+v", err)
	}

	// Set timeout configurations
	server.SetRequestTimeout(*requestTimeout)
	server.SetShutdownTimeout(*shutdownTimeout)

	// Get RTM API credentials from environment or config.
	// Environment variables override config for flexibility.
	apiKey := os.Getenv("RTM_API_KEY")
	if apiKey == "" {
		apiKey = cfg.RTM.APIKey
	}

	sharedSecret := os.Getenv("RTM_SHARED_SECRET")
	if sharedSecret == "" {
		sharedSecret = cfg.RTM.SharedSecret
	}

	// Ensure API key and shared secret are available.
	// These are essential for RTM communication.
	if apiKey == "" || sharedSecret == "" {
		err := cgerr.ErrorWithDetails(
			errors.New("missing RTM API credentials"),
			cgerr.CategoryConfig,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"has_api_key":       apiKey != "",
				"has_shared_secret": sharedSecret != "",
				"rtm_api_key_env":   os.Getenv("RTM_API_KEY") != "",
				"rtm_secret_env":    os.Getenv("RTM_SHARED_SECRET") != "",
			},
		)
		// Use better error logging with stack trace
		log.Fatalf("main: %+v", err)
	}

	// Get token path from config or env.
	// Again, env vars override config for flexibility.
	tokenPath := os.Getenv("RTM_TOKEN_PATH")
	if tokenPath == "" {
		tokenPath = cfg.Auth.TokenPath
	}

	// Expand ~ in token path if present.
	// This allows using home directory shorthand.
	expandedPath, err := config.ExpandPath(tokenPath)
	if err != nil {
		// Use better error logging with stack trace
		log.Fatalf("main: failed to expand token path: %+v", err)
	}
	tokenPath = expandedPath

	// Create and register RTM auth provider.
	// This handles RTM authentication within the server.
	authProvider, err := rtm.NewAuthProvider(apiKey, sharedSecret, tokenPath)
	if err != nil {
		// Use better error logging with stack trace
		log.Fatalf("main: failed to create RTM auth provider: %+v", err)
	}
	server.RegisterResourceProvider(authProvider) // Register with the server.

	// Handle graceful shutdown.
	// This ensures the server can stop cleanly.
	if *transportType == "http" {
		go func() {
			signals := make(chan os.Signal, 1)
			signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM) // Listen for interrupt or terminate.
			<-signals                                               // Block until a signal is received.
			log.Println("Shutting down server...")
			if err := server.Stop(); err != nil {
				// Use better error logging with stack trace
				log.Printf("main: error stopping server: %+v", err)
			}
		}()
	}

	// Start server.
	// This begins the main execution loop.
	log.Printf("Starting CowGnition MCP server with %s transport", *transportType)
	if err := server.Start(); err != nil {
		// Use better error logging with stack trace
		log.Fatalf("main: server failed to start: %+v", err)
	}
}
