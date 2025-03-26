// cmd/server/main.go
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/mcp"
	"github.com/dkoosis/cowgnition/internal/rtm"
)

func main() {
	// Load configuration
	cfg := config.New()

	// Create and start MCP server
	server, err := mcp.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Set version
	server.SetVersion("1.0.0")

	// Get RTM API credentials from environment or config
	apiKey := os.Getenv("RTM_API_KEY")
	if apiKey == "" {
		apiKey = cfg.RTM.APIKey
	}

	sharedSecret := os.Getenv("RTM_SHARED_SECRET")
	if sharedSecret == "" {
		sharedSecret = cfg.RTM.SharedSecret
	}

	if apiKey == "" || sharedSecret == "" {
		log.Fatalf("RTM API key and shared secret must be provided in config or environment variables")
	}

	// Get token path from config or env
	tokenPath := os.Getenv("RTM_TOKEN_PATH")
	if tokenPath == "" {
		tokenPath = cfg.Auth.TokenPath
	}

	// Expand ~ in token path if present
	expandedPath, err := config.ExpandPath(tokenPath)
	if err != nil {
		log.Fatalf("Failed to expand token path: %v", err)
	}
	tokenPath = expandedPath

	// Create and register RTM auth provider
	authProvider, err := rtm.NewAuthProvider(apiKey, sharedSecret, tokenPath)
	if err != nil {
		log.Fatalf("Failed to create RTM auth provider: %v", err)
	}
	server.RegisterResourceProvider(authProvider)

	// Handle graceful shutdown
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
		<-signals
		log.Println("Shutting down server...")
		if err := server.Stop(); err != nil {
			log.Printf("Error stopping server: %v", err)
		}
	}()

	// Start server
	log.Printf("Starting CowGnition MCP server on %s", cfg.GetServerAddress())
	if err := server.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
