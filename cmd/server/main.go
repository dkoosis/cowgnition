// Package main provides the entry point for the CowGnition MCP server.
// file: cmd/server/main.go
package main

import (
	"log"       // Standard logging for output.
	"os"        // OS-level interactions like environment variables.
	"os/signal" // Signal handling for graceful shutdown.
	"syscall"   // System call interface for signals.

	"github.com/dkoosis/cowgnition/internal/config" // Configuration loading.
	"github.com/dkoosis/cowgnition/internal/mcp"    // MCP server logic.
	"github.com/dkoosis/cowgnition/internal/rtm"    // RTM authentication.
)

func main() {
	// Load configuration.
	// This is done early to ensure settings are available.
	cfg := config.New()

	// Create and start MCP server.
	// The server orchestrates MCP communication.
	server, err := mcp.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err) // Terminate on fatal error.
	}

	// Set version.
	// Provides server identification to clients.
	server.SetVersion("1.0.0")

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
		log.Fatalf("RTM API key and shared secret must be provided in config or environment variables") // Terminate if missing.
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
		log.Fatalf("Failed to expand token path: %v", err) // Terminate on error.
	}
	tokenPath = expandedPath

	// Create and register RTM auth provider.
	// This handles RTM authentication within the server.
	authProvider, err := rtm.NewAuthProvider(apiKey, sharedSecret, tokenPath)
	if err != nil {
		log.Fatalf("Failed to create RTM auth provider: %v", err) // Terminate on error.
	}
	server.RegisterResourceProvider(authProvider) // Register with the server.

	// Handle graceful shutdown.
	// This ensures the server can stop cleanly.
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM) // Listen for interrupt or terminate.
		<-signals                                               // Block until a signal is received.
		log.Println("Shutting down server...")
		if err := server.Stop(); err != nil {
			log.Printf("Error stopping server: %v", err) // Log any shutdown errors.
		}
	}()

	// Start server.
	// This begins the main execution loop.
	log.Printf("Starting CowGnition MCP server on %s", cfg.GetServerAddress())
	if err := server.Start(); err != nil {
		log.Fatalf("Server failed: %v", err) // Terminate if server fails to start.
	}
}

// DocEnhanced: 2025-03-25
