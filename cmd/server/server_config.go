// cmd/server/server_config.go
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"gopkg.in/yaml.v3"
)

// findOrCreateConfig tries to find an existing config or create a default one.
// Returns the path to the config file and a boolean indicating success.
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
	if err == nil {
		userConfig := filepath.Join(homeDir, ".config", "cowgnition", "cowgnition.yaml")
		possiblePaths = append(possiblePaths, userConfig)
	}

	// Log potential config locations
	if debugMode {
		log.Printf("Searching for config file in: %v", possiblePaths)
	}

	// Check each path for an existing config
	for _, path := range possiblePaths {
		// Expand ~ in path if present
		expandedPath, err := config.ExpandPath(path)
		if err != nil {
			if debugMode {
				log.Printf("Error expanding path %s: %v", path, err)
			}
			continue
		}

		// Check if file exists
		if _, err := os.Stat(expandedPath); err == nil {
			log.Printf("Found existing configuration at: %s", expandedPath)
			return expandedPath, true
		}
	}

	// No existing config found, try to create one

	// First try in user's home directory
	if homeDir != "" {
		configDir := filepath.Join(homeDir, ".config", "cowgnition")
		configPath := filepath.Join(configDir, "cowgnition.yaml")

		if err := os.MkdirAll(configDir, 0755); err == nil {
			if err := createDefaultConfig(configPath); err == nil {
				log.Printf("Created new configuration at: %s", configPath)
				return configPath, true
			} else if debugMode {
				log.Printf("Failed to create config at %s: %v", configPath, err)
			}
		}
	}

	// Next try local configs directory
	configDir := "./configs"
	configPath := filepath.Join(configDir, "config.yaml")

	if err := os.MkdirAll(configDir, 0755); err == nil {
		if err := createDefaultConfig(configPath); err == nil {
			log.Printf("Created new configuration at: %s", configPath)
			return configPath, true
		} else if debugMode {
			log.Printf("Failed to create config at %s: %v", configPath, err)
		}
	}

	// Failed to find or create config
	log.Printf("Failed to find or create configuration file")
	return "", false
}

// createDefaultConfig creates a default configuration file.

// loadConfiguration loads the server configuration from a file.
func loadConfiguration(configPath string) (*config.Settings, error) {
	// Create default configuration
	cfg := config.New()

	// Load configuration from file if specified
	if configPath != "" {
		if debugMode {
			log.Printf("Loading configuration from %s", configPath)
		}

		// Read the file
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, cgerr.ErrorWithDetails(
				errors.Wrap(err, "loadConfiguration: failed to read configuration file"),
				cgerr.CategoryConfig,
				cgerr.CodeInternalError,
				map[string]interface{}{
					"config_path": configPath,
					"file_exists": false,
					"os_user":     os.Getenv("USER"),
				},
			)
		}

		// Debug: Print raw YAML data (sanitized)
		if debugMode {
			yamlData := string(data)
			log.Printf("Raw YAML data (first 100 chars): %s", sanitizeForLogging(yamlData, 100))
		}

		// Unmarshal YAML into config struct
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, cgerr.ErrorWithDetails(
				errors.Wrap(err, "loadConfiguration: failed to parse configuration file"),
				cgerr.CategoryConfig,
				cgerr.CodeInternalError,
				map[string]interface{}{
					"config_path":      configPath,
					"data_size":        len(data),
					"data_starts_with": string(data[:min(50, len(data))]),
					"yaml_error":       err.Error(),
				},
			)
		}

		// Debug: Print the loaded config values
		if debugMode {
			log.Printf("Loaded configuration values:")
			log.Printf("  - Server.Name: %s", cfg.Server.Name)
			log.Printf("  - Server.Port: %d", cfg.Server.Port)
			log.Printf("  - RTM.APIKey: %s (length: %d)", maskCredential(cfg.RTM.APIKey), len(cfg.RTM.APIKey))
			log.Printf("  - RTM.SharedSecret: %s (length: %d)", maskCredential(cfg.RTM.SharedSecret), len(cfg.RTM.SharedSecret))
			log.Printf("  - Auth.TokenPath: %s", cfg.Auth.TokenPath)
		}

		log.Printf("Configuration loaded successfully")
	}

	return cfg, nil
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Helper function to sanitize data for logging.
func sanitizeForLogging(data string, maxLen int) string {
	if len(data) <= maxLen {
		return data
	}
	return data[:maxLen] + "..."
}

// Helper function to mask credentials in logs.
func maskCredential(cred string) string {
	if len(cred) < 6 {
		return "****"
	}
	return cred[:2] + "****" + cred[len(cred)-2:]
}
