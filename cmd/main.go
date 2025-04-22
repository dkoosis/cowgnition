// file: cmd/main.go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/dkoosis/cowgnition/cmd/server"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/rtm"
)

// Version information - should be set during build via ldflags.
var (
	Version    = "0.1.0-dev" // Default development version
	commitHash = "unknown"   //nolint:unused // Set via ldflags during build
	buildDate  = "unknown"   //nolint:unused // Set via ldflags during build
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
			// Use internal logger after setup
			logging.SetupDefaultLogger("debug")
			logger := logging.GetLogger("main")
			logger.Info("Debug logging enabled")
		} else {
			// Setup default info logging if not debug
			logging.SetupDefaultLogger("info")
		}

		if err := server.RunServer(*transportType, *serveConfigPath, *requestTimeout, *shutdownTimeout, debugMode); err != nil {
			// Use the configured logger
			logger := logging.GetLogger("main")
			logger.Error("Server failed.", "error", fmt.Sprintf("%+v", err))
			os.Exit(1) // Exit after logging
		}

	case "diagnose-keychain":
		diagnoseCmd := flag.NewFlagSet("diagnose-keychain", flag.ExitOnError)
		// Config path isn't strictly needed for keychain diag anymore,
		// but keep the flag for consistency or future use.
		diagnoseConfigPath := diagnoseCmd.String("config", getDefaultConfigPath(), "Path to configuration file (optional for keychain test).")

		if err := diagnoseCmd.Parse(os.Args[2:]); err != nil {
			log.Fatalf("Failed to parse diagnose-keychain command flags: %+v", err)
		}

		runKeychainDiagnostics(*diagnoseConfigPath) // Pass config path, though it might not be used

	default:
		printUsage()
		os.Exit(1)
	}
}

// printUsage prints usage information for the command.
func printUsage() {
	// Use standard log package for initial usage/errors before logger setup
	log.Println("Usage:")
	log.Println("  cowgnition setup [options]  - Set up CowGnition and Claude Desktop integration")
	log.Println("  cowgnition serve [options]  - Start the CowGnition server")
	log.Println("  cowgnition diagnose-keychain [options] - Test and diagnose keychain access")
	log.Println("\nRun 'cowgnition <command> -h' for help on a specific command.")
}

// getDefaultConfigPath returns the default path for the configuration file.
func getDefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback, but log the error
		log.Printf("Warning: Could not get user home directory: %v. Using relative fallback config path.", err)
		return "configs/cowgnition.yaml" // Fallback to local directory.
	}
	return filepath.Join(homeDir, ".config", "cowgnition", "cowgnition.yaml")
}

// runKeychainDiagnostics runs tests to check keychain functionality and provides troubleshooting advice.
func runKeychainDiagnostics(_ string) { // configPath is now ignored, indicated by _
	// Setup logger specifically for this diagnostic command
	logging.SetupDefaultLogger("debug") // Use debug level for diagnostics
	logger := logging.GetLogger("keychain_diag")

	logger.Info("Starting keychain diagnostics...")

	// Create secure storage directly using the internal logger
	secureStorage := rtm.NewSecureTokenStorage(logger)

	// Print diagnostic information using the new methods
	fmt.Println("\n=== MacOS Keychain Diagnostics ===")
	fmt.Printf("Keyring Service: %s\n", secureStorage.GetKeychainServiceName()) // CORRECTED CALL
	fmt.Printf("Keyring User: %s\n", secureStorage.GetKeychainUserName())       // CORRECTED CALL

	// Run availability check
	fmt.Println("\nAvailability check:")
	available := secureStorage.IsAvailable() // Method exists
	fmt.Printf("Keychain reported as available: %t\n", available)

	// Run comprehensive diagnostics using the new method
	fmt.Println("\nRunning keychain operations test:")
	results := secureStorage.DiagnoseKeychain() // CORRECTED CALL

	// Print results
	fmt.Printf("%-18s: %v\n", "Set Operation", results["set_success"])
	if err, ok := results["set_error"]; ok {
		fmt.Printf("%-18s: %v\n", "Set Error", err)
	}
	fmt.Printf("%-18s: %v\n", "Get Operation", results["get_success"])
	if err, ok := results["get_error"]; ok {
		fmt.Printf("%-18s: %v\n", "Get Error", err)
	}
	if match, ok := results["get_value_match"]; ok {
		fmt.Printf("%-18s: %v\n", "Get Value Match", match)
	}
	fmt.Printf("%-18s: %v\n", "Delete Operation", results["delete_success"])
	if err, ok := results["delete_error"]; ok {
		fmt.Printf("%-18s: %v\n", "Delete Error", err)
	}

	// Provide recommendations using the new method
	fmt.Println("\nRecommendations:")
	if !available || results["set_success"] == false {
		fmt.Println(secureStorage.GetKeychainAdvice()) // CORRECTED CALL
	} else {
		fmt.Println("Keychain appears to be working correctly. If you're still experiencing issues:")
		fmt.Println("1. Try deleting any existing 'CowGnitionRTM' entries in Keychain Access.")
		fmt.Println("2. Ensure your login keychain is unlocked.")
		fmt.Println("3. Look for permission dialogs when the application runs.")
	}
}
