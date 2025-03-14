// Package main implements the CowGnition CLI application.
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

// Command represents a CLI command with its name, description, and implementation.
type Command struct {
	Name        string
	Description string
	Run         func(args []string) error
}

// RegisterCommands registers all available CLI commands.
// Returns a map of command names to Command objects.
func RegisterCommands() map[string]Command {
	return map[string]Command{
		"serve": {
			Name:        "serve",
			Description: "Start the MCP server",
			Run:         serveCommand,
		},
		"version": {
			Name:        "version",
			Description: "Show version information",
			Run:         versionCommand,
		},
		"help": {
			Name:        "help",
			Description: "Show help for commands",
			Run:         helpCommand,
		},
	}
}

// serveCommand starts the MCP server with the specified configuration.
// It handles graceful shutdown on receiving termination signals.
func serveCommand(args []string) error {
	// Parse command-specific flags
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "configs/config.yaml", "Path to configuration file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Load config
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	// Create and start server
	mcp, err := server.NewMCPServer(cfg)
	if err != nil {
		return fmt.Errorf("error creating server: %w", err)
	}

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		log.Printf("Starting MCP server on port %d", cfg.Server.Port)
		errCh <- mcp.Start()
	}()

	// Wait for interrupt or error
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal or error
	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case sig := <-sigCh:
		log.Printf("Received signal %s, shutting down...", sig)
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mcp.Stop(ctx); err != nil {
		return fmt.Errorf("error stopping server: %w", err)
	}

	log.Println("Server shutdown complete")
	return nil
}

// versionCommand displays the current version information.
func versionCommand(args []string) error {
	fmt.Printf("CowGnition version %s\n", version)
	return nil
}

// helpCommand displays help information for all commands or a specific command.
func helpCommand(args []string) error {
	// Parse command-specific flags
	fs := flag.NewFlagSet("help", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Get requested command
	cmds := RegisterCommands()
	cmdName := ""
	if fs.NArg() > 0 {
		cmdName = fs.Arg(0)
	}

	// If specific command requested, show help for it
	if cmdName != "" {
		cmd, ok := cmds[cmdName]
		if !ok {
			return fmt.Errorf("unknown command: %s", cmdName)
		}

		// Show command help
		fmt.Printf("Command: %s\n", cmd.Name)
		fmt.Printf("Description: %s\n", cmd.Description)
		// TODO: Add command-specific usage information if needed
		return nil
	}

	// Otherwise, show general help
	fmt.Println("CowGnition - Remember The Milk MCP Server")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  cowgnition [command] [options]")
	fmt.Println()
	fmt.Println("Available Commands:")
	for _, cmd := range cmds {
		fmt.Printf("  %-10s %s\n", cmd.Name, cmd.Description)
	}
	fmt.Println()
	fmt.Println("Use 'cowgnition help [command]' for more information about a command.")

	return nil
}
