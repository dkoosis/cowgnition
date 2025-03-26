// cmd/server/main.go
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourusername/cowgnition/internal/config"
	"github.com/yourusername/cowgnition/internal/mcp"
)

func main() {
	// Load configuration
	cfg := config.NewConfig()

	// Create and start MCP server
	server, err := mcp.NewServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Set version
	server.SetVersion("1.0.0")

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
	if err := server.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
