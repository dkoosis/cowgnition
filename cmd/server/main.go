// file: cmd/server/main.go
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Version information - should be set during build via ldflags
var (
	Version    = "0.1.0-dev" // Default development version
	commitHash = "unknown"
	buildDate  = "unknown"
)

// Global debugging flag.
var debugMode bool

func main() {
	// Check if we have a subcommand.
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Process subcommands.
	switch os.Args[1] {
	case "setup":
		setupCmd := flag.NewFlagSet("setup", flag.ExitOnError)
		setupConfigPath := setupCmd.String("config", getDefaultConfigPath(), "Path to configuration file.")

		// Check error from Parse.
		if err := setupCmd.Parse(os.Args[2:]); err != nil {
			log.Fatalf("Failed to parse setup command flags: %+v", err)
		}

		if err := runSetup(*setupConfigPath); err != nil {
			log.Fatalf("Setup failed: %+v", err)
		}

	case "serve":
		serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
		// Changed the default to "stdio" instead of "http"
		transportType := serveCmd.String("transport", "stdio", "Transport type (http or stdio).")
		serveConfigPath := serveCmd.String("config", "", "Path to configuration file.")
		requestTimeout := serveCmd.Duration("request-timeout", 30*time.Second, "Timeout for JSON-RPC requests.")
		shutdownTimeout := serveCmd.Duration("shutdown-timeout", 5*time.Second, "Timeout for graceful shutdown.")
		debug := serveCmd.Bool("debug", false, "Enable debug logging.")

		// Check error from Parse.
		if err := serveCmd.Parse(os.Args[2:]); err != nil {
			log.Fatalf("Failed to parse serve command flags: %+v", err)
		}

		// Set debug mode if specified
		if *debug {
			debugMode = true
			log.Printf("Debug logging enabled")
		}

		if err := runServer(*transportType, *serveConfigPath, *requestTimeout, *shutdownTimeout); err != nil {
			log.Fatalf("Server failed: %+v", err)
		}

	default:
		printUsage()
		os.Exit(1)
	}
}

// printUsage prints usage information for the command.
func printUsage() {
	log.Println("Usage:")
	log.Println("  cowgnition setup [options]  - Set up CowGnition and Claude Desktop integration")
	log.Println("  cowgnition serve [options]  - Start the CowGnition server")
	log.Println("\nRun 'cowgnition <command> -h' for help on a specific command.")
}

// TODO: fix configuration file handling
// getDefaultConfigPath returns the default path for the configuration file.
func getDefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "configs/cowgnition.yaml" // Fallback to local directory.
	}
	return filepath.Join(homeDir, ".config", "cowgnition", "cowgnition.yaml")
}
