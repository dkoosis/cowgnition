// cmd/server/server_config.go
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"gopkg.in/yaml.v3"
)

// Initialize the logger at the package level.
var configLogger = logging.GetLogger("server_config")

// findOrCreateConfig tries to find an existing config or create a default one.
// Returns the path to the config file and a boolean indicating success.
// NOTE: This function was marked nolint:unused in the original code.
// Ensure it's actually needed or remove it if truly unused.
//
//nolint:unused
func findOrCreateConfig() (string, bool) {
	// Try standard locations in this order
	possiblePaths := []string{
		"./configs/config.yaml",
		"./configs/cowgnition.yaml",
	}

	// Try to add user's home directory config
	homeDir, err := os.UserHomeDir()
	if err == nil && homeDir != "" {
		userConfig := filepath.Join(homeDir, ".config", "cowgnition", "cowgnition.yaml")
		possiblePaths = append(possiblePaths, userConfig)
	} else if err != nil {
		configLogger.Warn("Could not determine user home directory", "error", err)
	}

	// Log potential config locations (Debug Level)
	// Replaces L21 log.Printf
	configLogger.Debug("Searching for config file", "possible_paths", possiblePaths)

	// Check each path for an existing config
	for _, path := range possiblePaths {
		// Expand ~ in path if present
		expandedPath, expandErr := config.ExpandPath(path) // Renamed err variable
		if expandErr != nil {
			// Replaces L25 log.Printf
			// Log error during expansion but continue searching
			configLogger.Debug("Error expanding path, skipping", "path", path, "error", fmt.Sprintf("%+v", expandErr))
			continue
		}

		// Check if file exists
		if _, statErr := os.Stat(expandedPath); statErr == nil { // Renamed err variable
			// Replaces L30 log.Printf
			configLogger.Info("Found existing configuration", "path", expandedPath)
			return expandedPath, true
		} else if !os.IsNotExist(statErr) {
			// Log unexpected error checking file existence
			configLogger.Warn("Error checking config file status", "path", expandedPath, "error", statErr)
		}
	}

	configLogger.Info("No existing configuration found, attempting to create default.")

	// --- Try to create one ---

	// Helper function to attempt creation
	tryCreate := func(path string) (string, bool) {
		expandedPath, expandErr := config.ExpandPath(path)
		if expandErr != nil {
			configLogger.Warn("Cannot create config, failed to expand path", "path", path, "error", fmt.Sprintf("%+v", expandErr))
			return "", false
		}
		configDir := filepath.Dir(expandedPath)
		if mkdirErr := os.MkdirAll(configDir, 0755); mkdirErr != nil { // Renamed err variable
			configLogger.Warn("Failed to create config directory", "dir", configDir, "error", mkdirErr)
			return "", false
		}
		// Assuming createDefaultConfig exists elsewhere in the package
		if createErr := createDefaultConfig(expandedPath); createErr != nil { // Renamed err variable
			// Replaces L71 log.Printf
			configLogger.Warn("Failed to create default config file", "path", expandedPath, "error", fmt.Sprintf("%+v", createErr))
			return "", false
		}
		// Replaces L51 log.Printf
		configLogger.Info("Created new default configuration", "path", expandedPath)
		return expandedPath, true
	}

	// First try in user's home directory
	if homeDir != "" {
		userPath := filepath.Join(homeDir, ".config", "cowgnition", "cowgnition.yaml")
		if createdPath, ok := tryCreate(userPath); ok {
			return createdPath, true
		}
	}

	// Next try local configs directory
	localPath := "./configs/config.yaml"
	if createdPath, ok := tryCreate(localPath); ok {
		return createdPath, true
	}

	// Failed to find or create config
	// Replaces L98 log.Printf
	configLogger.Error("Failed to find or create any configuration file after checking all locations.")
	return "", false
}

// loadConfiguration loads the server configuration from a file.
func loadConfiguration(configPath string) (*config.Settings, error) {
	// Create default configuration first
	cfg := config.New() // Assumes config.New() logs its own debug message

	// Load configuration from file only if a path is provided
	if configPath == "" {
		configLogger.Warn("No config path provided, using default settings only.")
		return cfg, nil
	}

	// Expand path in case it contains ~
	expandedPath, expandErr := config.ExpandPath(configPath)
	if expandErr != nil {
		configLogger.Error("Failed to expand provided config path", "config_path", configPath, "error", fmt.Sprintf("%+v", expandErr))
		// Return error as we cannot proceed without a valid path
		return nil, errors.Wrapf(expandErr, "loadConfiguration: failed to expand config path '%s'", configPath)
	}
	configPath = expandedPath // Use expanded path going forward

	// Replaces L117 log.Printf
	configLogger.Info("Loading configuration", "config_path", configPath)

	// Read the file
	data, err := os.ReadFile(configPath)
	if err != nil {
		// Existing error handling is good (uses cgerr)
		wrappedErr := errors.Wrap(err, "loadConfiguration: failed to read configuration file")
		return nil, cgerr.ErrorWithDetails(
			wrappedErr,
			cgerr.CategoryConfig,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"config_path": configPath,
				// "file_exists": false, // This might be misleading if it was a permission error
				"os_user": os.Getenv("USER"),
			},
		)
	}

	// Debug: Print raw YAML data snippet
	// Replaces L141 log.Printf
	if logging.IsDebugEnabled() { // Check effective log level
		yamlData := string(data)
		// Use structured logging for the snippet
		configLogger.Debug("Raw YAML data snippet read from file", "config_path", configPath, "snippet", sanitizeForLogging(yamlData, 100))
	}

	// Unmarshal YAML into config struct
	if err := yaml.Unmarshal(data, cfg); err != nil {
		// Existing error handling is good (uses cgerr)
		wrappedErr := errors.Wrap(err, "loadConfiguration: failed to parse configuration file (YAML)")
		return nil, cgerr.ErrorWithDetails(
			wrappedErr,
			cgerr.CategoryConfig,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"config_path":      configPath,
				"data_size":        len(data),
				"data_starts_with": sanitizeForLogging(string(data), 50), // Use sanitize helper
				"yaml_error":       err.Error(),                          // Include specific YAML error message
			},
		)
	}

	// Debug: Print the loaded config values using structured logging
	// Replaces multiple log.Printf calls
	if logging.IsDebugEnabled() { // Check effective log level
		configLogger.Debug("Loaded configuration values",
			"config_path", configPath,
			"server_name", cfg.Server.Name,
			"server_port", cfg.Server.Port,
			"rtm_api_key_present", cfg.RTM.APIKey != "",
			"rtm_shared_secret_present", cfg.RTM.SharedSecret != "",
			"auth_token_path", cfg.Auth.TokenPath,
		)
		// Example of logging potentially sensitive data if REALLY needed for debug (use with caution)
		// configLogger.Debug("Sensitive Config Values (Masked)",
		// 	"rtm_api_key", maskCredential(cfg.RTM.APIKey),
		// 	"rtm_shared_secret", maskCredential(cfg.RTM.SharedSecret),
		// )
	}

	// Replaces final log.Printf
	configLogger.Info("Configuration loaded successfully", "config_path", configPath)
	return cfg, nil
}

// --- Helper Functions ---

// Assume createDefaultConfig exists elsewhere in package main
// func createDefaultConfig(path string) error { ... }

// min returns the minimum of two integers. (Keep as is)
//
//nolint:unused
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Helper function to sanitize data for logging. (Keep as is).
func sanitizeForLogging(data string, maxLen int) string {
	if len(data) <= maxLen {
		return data
	}
	return data[:maxLen] + "..."
}

// Helper function to mask credentials in logs. (Keep as is)
//
//nolint:unused
func maskCredential(cred string) string {
	if len(cred) < 6 {
		return "****" // Mask short credentials completely
	}
	// Mask middle part, leaving first/last 2 chars visible
	return cred[:2] + "****" + cred[len(cred)-2:]
}
