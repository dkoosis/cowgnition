// cmd/server/commands.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
)

// Command-line variables set during build
var (
	version     = "dev"
	buildCommit = "unknown"
	buildTime   = "unknown"
	goVersion   = "unknown"
)

// Define command struct
type command struct {
	Name        string
	Description string
	Run         func([]string) error
	Help        string
}

var commands = []command{
	{
		Name:        "version",
		Description: "Print the version information",
		Run:         versionCommand,
		Help:        "Usage: cowgnition version",
	},
	{
		Name:        "serve",
		Description: "Start the MCP server",
		Run:         serveCommand,
		Help: `Usage: cowgnition serve [options]

Options:
  -config string     Path to configuration file (default "config.yaml")
  -port int          Server port (overrides config file) (default 8080)
  -dev               Run in development mode
`,
	},
	{
		Name:        "check",
		Description: "Check configuration and connectivity",
		Run:         checkCommand,
		Help:        "Usage: cowgnition check -config [path]",
	},
}

// serveCommand starts the MCP server
func serveCommand(args []string) error {
	// Parse command-specific flags
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to configuration file")
	port := fs.Int("port", 0, "server port (overrides config file)")
	devMode := fs.Bool("dev", false, "run in development mode")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("failed to parse serve command flags: %w", err)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	// Override port if specified
	if *port > 0 {
		cfg.Server.Port = *port
	}

	// Override dev mode if specified
	if *devMode {
		cfg.Server.DevMode = true
	}

	// Create server
	s, err := server.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("error creating server: %w", err)
	}

	// Set version
	s.SetVersion(version)

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		log.Printf("Starting MCP server on port %d", cfg.Server.Port)
		errChan <- s.Start()
	}()

	// Wait for signal or error
	select {
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	case sig := <-sigChan:
		log.Printf("Received signal %v, shutting down...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.Stop(ctx); err != nil {
			return fmt.Errorf("error shutting down server: %w", err)
		}
	}

	return nil
}

// versionCommand prints version information
func versionCommand([]string) error {
	fmt.Printf("CowGnition version %s\n", version)
	fmt.Printf("Build: %s (%s)\n", buildCommit, buildTime)
	fmt.Printf("Compiler: %s\n", goVersion)
	return nil
}

// checkCommand validates configuration and tests connectivity
func checkCommand(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	configPath := fs.String("config", "config.yaml", "path to configuration file")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("failed to parse check command flags: %w", err)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	// Validate RTM API credentials
	fmt.Printf("Configuration loaded from: %s\n", *configPath)
	fmt.Printf("Server name: %s\n", cfg.Server.Name)
	fmt.Printf("RTM API key: %s\n", maskString(cfg.RTM.APIKey))
	fmt.Printf("RTM shared secret: %s\n", maskString(cfg.RTM.SharedSecret))
	fmt.Printf("Token path: %s\n", cfg.Auth.TokenPath)

	// Try to initialize server to verify connections
	fmt.Println("Testing server initialization...")
	s, err := server.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("server initialization error: %w", err)
	}

	fmt.Println("✅ Configuration is valid and server initialization successful!")
	return nil
}

// maskString returns a masked version of a string for secure display
func maskString(s string) string {
	if len(s) <= 8 {
		return "********"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
