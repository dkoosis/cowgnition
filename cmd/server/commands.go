// Package main implements the CowGnition CLI application.
// file: cmd/server/main.go
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
// It provides a structure for defining and handling command-line operations.
type Command struct {
	Name        string
	Description string
	Run         func(argsstring) error
}

// RegisterCommands registers all available CLI commands.
// Returns a map of command names to Command objects.
// This function defines the available commands for the CowGnition CLI,
// associating each command name with its corresponding Command structure.
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
		"check": {
			Name:        "check",
			Description: "Check authentication status with RTM",
			Run:         checkCommand,
		},
	}
}

// serveCommand starts the MCP server with the specified configuration.
// It handles graceful shutdown on receiving termination signals.
// This command initializes the server, loads the configuration,
// and starts the server, listening for MCP requests.  It also manages
// graceful shutdown when the application receives an interrupt signal,
// ensuring a clean exit.
func serveCommand(argsstring) error {
	// Parse command-specific flags
	// It uses the flag package to define and parse command-line flags
	// specific to the 'serve' command, such as the configuration file path
	// and debug mode.
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to configuration file")
	debugMode := fs.Bool("debug", false, "Enable debug logging")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("fs.Parse: failed to parse arguments: %w", err)
	}

	// Find config file if not specified
	// It uses the findConfigFile function to locate the configuration file.
	// If a configuration path is provided as a flag, it uses that path;
	// otherwise, it searches for a default configuration file.
	configFile := findConfigFile(*configPath)

	// Load config
	// It uses the config.LoadConfig function to load the server configuration
	// from the specified file. This configuration is used to initialize
	// the server and define its behavior.
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("config.LoadConfig: error loading config: %w", err)
	}

	// Set debug mode if requested
	// If the debug mode flag is set, it configures the server to output
	// debug-level logging. This is useful for development and troubleshooting.
	if *debugMode {
		log.Printf("Debug mode enabled")
		// In a real implementation, we would configure logging level here
	}

	// Create and start server
	// It creates a new server instance using the loaded configuration.
	// This server is responsible for handling MCP requests and interacting
	// with the RTM service.
	srv, err := server.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("server.NewServer: error creating server: %w", err)
	}

	// Set server version
	// It sets the server's version information, which may be used in responses
	// or logging.
	srv.SetVersion(version)

	// Start server in goroutine
	// It starts the server in a separate goroutine to allow for asynchronous
	// operation and signal handling. The server's Start method begins
	// listening for and handling MCP requests.
	errCh := make(chan error, 1)
	go func() {
		log.Printf("Starting MCP server '%s' on port %d", cfg.Server.Name, cfg.Server.Port)
		errCh <- srv.Start()
	}()

	// Wait for interrupt or error
	// It sets up a signal handler to listen for interrupt and terminate signals
	// from the operating system. This allows the server to shut down gracefully
	// when requested by the user or the system.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Wait for signal or error
	// This select statement blocks until either an error is received from the
	// server's goroutine or a signal is received from the operating system.
	// This allows the main function to wait for the server to start or
	// for a shutdown signal.
	select {
	case err := <-errCh:
		return fmt.Errorf("srv.Start: server error: %w", err)
	case sig := <-sigCh:
		log.Printf("Received signal %s, shutting down...", sig)
	}

	// Graceful shutdown
	// It initiates a graceful shutdown of the server, allowing it to
	// complete any ongoing requests before exiting.  A timeout is used
	// to prevent the shutdown process from hanging indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		return fmt.Errorf("srv.Stop: error stopping server: %w", err)
	}

	log.Println("Server shutdown complete")
	return nil
}

// versionCommand displays the current version information.
// This command prints the application's version to the console.
func versionCommand(_string) error {
	printVersion()
	return nil
}

// checkCommand checks the RTM authentication status.
// This command verifies the application's authentication status with
// the Remember The Milk (RTM) service. It loads the configuration,
// initializes the server components necessary for authentication checks,
// and then performs the check.
func checkCommand(argsstring) error {
	// Parse command-specific flags
	// It uses the flag package to define and parse command-line flags
	// specific to the 'check' command, such as the configuration file path.
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to configuration file")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("fs.Parse: failed to parse arguments: %w", err)
	}

	// Find config file if not specified
	// It uses the findConfigFile function to locate the configuration file.
	// If a configuration path is provided as a flag, it uses that path;
	// otherwise, it searches for a default configuration file.
	configFile := findConfigFile(*configPath)

	// Load config
	// It uses the config.LoadConfig function to load the server configuration
	// from the specified file. This configuration is used to initialize
	// the server components.
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("config.LoadConfig: error loading config: %w", err)
	}

	fmt.Println("Checking RTM authentication status...")

	// Create server (but don't start HTTP server)
	// It creates a new server instance using the loaded configuration.
	// This server instance provides access to the RTM service for
	// authentication checking, but it does not start the HTTP server.
	svr, err := server.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("server.NewServer: error creating server: %w", err)
	}

	// Check if authenticated using the RTM service
	// It retrieves the RTM service from the server instance and checks
	// its authentication status.  The result of this check is then
	// printed to the console.
	fmt.Println("Authentication status check:")
	rtmService := svr.GetRTMService()
	if rtmService != nil && rtmService.IsAuthenticated() {
		fmt.Println("✓ Authenticated with Remember The Milk")
	} else {
		fmt.Println("✗ Not authenticated with Remember The Milk")
		fmt.Println("Run the server and access the auth://rtm resource to authenticate")
	}

	return nil
}

// helpCommand displays help information for all commands or a specific command.
// This command provides help information to the user, either for a
// specific command or for the application as a whole. It parses the
// command-line arguments to determine the scope of the help request
// and then displays the appropriate information.
func helpCommand(argsstring) error {
	// Parse command-specific flags
	// It uses the flag package to parse command-line flags.
	fs := flag.NewFlagSet("help", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("fs.Parse: failed to parse arguments: %w", err)
	}

	// Get requested command
	// It retrieves the registered commands.
	cmds := RegisterCommands()
	cmdName := ""
	if fs.NArg() > 0 {
		cmdName = fs.Arg(0)
	}

	// If specific command requested, show help for it
	// If the user has requested help for a specific command, it retrieves
	// the corresponding Command structure and prints the command's
	// name, description, and usage information.
	if cmdName != "" {
		cmd, ok := cmds[cmdName]
		if !ok {
			return fmt.Errorf("unknown command: %s", cmdName)
		}

		// Show command help
		fmt.Printf("Command: %s\n", cmd.Name)
		fmt.Printf("Description: %s\n", cmd.Description)

		// Add command-specific usage information
		switch cmdName {
		case "serve":
			fmt.Println("\nUsage:")
			fmt.Println("  cowgnition serve [options]")
			fmt.Println("\nOptions:")
			fmt.Println("  -config string   Path to configuration file")
			fmt.Println("  -debug           Enable debug logging")
		case "check":
			fmt.Println("\nUsage:")
			fmt.Println("  cowgnition check [options]")
			fmt.Println("\nOptions:")
			fmt.Println("  -config string   Path to configuration file")
		}

		return nil
	}

	// Otherwise, show general help
	// If no specific command is requested, it prints general help
	// information for the application, including usage instructions,
	// a list of available commands, and version information.
	fmt.Println("CowGnition - Remember The Milk MCP Server")
	fmt.Println("\nUsage:")
	fmt.Println("  cowgnition [command] [options]")
	fmt.Println("\nAvailable Commands:")
	for _, cmd := range cmds {
		fmt.Printf("  %-10s %s\n", cmd.Name, cmd.Description)
	}
	fmt.Println("\nUse 'cowgnition help [command]' for more information about a command.")
	fmt.Println("\nVersion:")
	fmt.Printf("  %s\n", version)

	return nil
}

// DocEnhanced (2024-02-29)
