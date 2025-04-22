// file: cmd/keychain_diagnostic/main.go
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath" // Keep for config path default, even if unused

	// Removed unused config import: "github.com/dkoosis/cowgnition/internal/config".
	"github.com/cockroachdb/errors" // Keep if used in helper functions if added later
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/rtm"
)

func main() {
	// Parse flags
	// configPath is kept for potential future use or consistency, but marked as unused
	_ = flag.String("config", getDefaultConfigPath(), "Path to configuration file (optional).")
	flag.Parse()

	// Call the diagnostic function (which no longer needs configPath)
	if err := runKeychainDiagnostics(); err != nil {
		fmt.Printf("Keychain diagnostics failed: %+v\n", err)
		os.Exit(1)
	}
}

// getDefaultConfigPath returns the default path for the configuration file.
// Kept for consistency with main.go, even if config isn't used here.
func getDefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Log to stderr since logger isn't set up yet
		fmt.Fprintf(os.Stderr, "Warning: Could not get user home directory: %v. Using relative fallback path.\n", err)
		return "configs/cowgnition.yaml" // Fallback to local directory.
	}
	return filepath.Join(homeDir, ".config", "cowgnition", "cowgnition.yaml")
}

// runKeychainDiagnostics runs tests to check keychain functionality and provides troubleshooting advice.
func runKeychainDiagnostics() error { // Removed configPath parameter
	// Setup logging
	logging.SetupDefaultLogger("debug") // Use debug for diagnostics
	logger := logging.GetLogger("keychain_diag_tool")

	logger.Info("Starting keychain diagnostics...")

	// Load config - REMOVED as cfg is unused
	// cfg, err := config.LoadFromFile(configPath)
	// if err != nil {
	// 	// Handle file not found gracefully for this tool
	// 	if os.IsNotExist(err) {
	// 		logger.Warn("Configuration file not found, proceeding with defaults/env vars for potential RTM config (if needed).", "path", configPath)
	// 	} else {
	// 		// For other errors loading config, return the error
	// 		return errors.Wrap(err, "failed to load configuration")
	// 	}
	// }
	// REMOVED: `cfg` variable is no longer declared or used.

	// Create secure storage directly using the logger
	secureStorage := rtm.NewSecureTokenStorage(logger)

	// Print diagnostic information using the methods from SecureTokenStorage
	fmt.Println("\n=== MacOS Keychain Diagnostics ===")
	fmt.Printf("Keyring Service: %s\n", secureStorage.GetKeychainServiceName())
	fmt.Printf("Keyring User: %s\n", secureStorage.GetKeychainUserName()) // CORRECTED CALL

	// Run availability check
	fmt.Println("\nAvailability check:")
	available := secureStorage.IsAvailable()
	fmt.Printf("Keychain reported as available: %t\n", available)

	// Run comprehensive diagnostics using the method from SecureTokenStorage
	fmt.Println("\nRunning keychain operations test:")
	results := secureStorage.DiagnoseKeychain()

	// Print results in a more readable format
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

	// Provide recommendations using the method from SecureTokenStorage
	fmt.Println("\nRecommendations:")
	if !available || results["set_success"] == false {
		fmt.Println(secureStorage.GetKeychainAdvice())
	} else {
		fmt.Println("Keychain appears to be working correctly. If you're still experiencing issues:")
		fmt.Println("1. Try deleting any existing 'CowGnitionRTM' entries in Keychain Access.")
		fmt.Println("2. Ensure your login keychain is unlocked.")
		fmt.Println("3. Look for permission dialogs when the application runs.")
	}

	// Return potential error from keychain operations if needed, otherwise nil
	// Example: return the first error encountered during diagnosis
	if errStr, ok := results["set_error"].(string); ok && errStr != "" {
		return errors.New(errStr)
	}
	if errStr, ok := results["get_error"].(string); ok && errStr != "" {
		return errors.New(errStr)
	}
	if errStr, ok := results["delete_error"].(string); ok && errStr != "" {
		return errors.New(errStr)
	}

	return nil // No critical errors found during diagnosis
}
