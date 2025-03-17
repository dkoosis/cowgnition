// Package helpers provides testing utilities for the CowGnition MCP server.
package helpers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TestConfig holds configuration for tests that interact with the real RTM API.
type TestConfig struct {
	RTM struct {
		APIKey       string `json:"api_key"`
		SharedSecret string `json:"shared_secret"`
		AuthToken    string `json:"auth_token,omitempty"`
	} `json:"rtm"`
	Options struct {
		SkipLiveTests bool `json:"skip_live_tests"`
		DebugMode     bool `json:"debug_mode"`
	} `json:"options"`
}

// LoadTestConfig loads test configuration from a file or environment variables.
func LoadTestConfig(configPath string) (*TestConfig, error) {
	var config TestConfig

	// Try to load from file first if path is provided
	if configPath != "" {
		file, err := os.ReadFile(configPath)
		if err == nil {
			if err := json.Unmarshal(file, &config); err != nil {
				return nil, fmt.Errorf("error parsing test config file: %w", err)
			}
			return &config, nil
		}
	}

	// Fall back to environment variables
	config.RTM.APIKey = os.Getenv("RTM_API_KEY")
	config.RTM.SharedSecret = os.Getenv("RTM_SHARED_SECRET")
	config.RTM.AuthToken = os.Getenv("RTM_AUTH_TOKEN")
	config.Options.SkipLiveTests = os.Getenv("RTM_SKIP_LIVE_TESTS") == "true"
	config.Options.DebugMode = os.Getenv("RTM_TEST_DEBUG") == "true"

	// Check if we have valid RTM credentials
	if config.RTM.APIKey == "" || config.RTM.SharedSecret == "" {
		// Try to find a config file in standard locations
		homePath, err := os.UserHomeDir()
		if err == nil {
			// Try ~/.config/cowgnition/test_config.json
			configPath = filepath.Join(homePath, ".config", "cowgnition", "test_config.json")
			if file, err := os.ReadFile(configPath); err == nil {
				if err := json.Unmarshal(file, &config); err != nil {
					return nil, fmt.Errorf("error parsing test config file: %w", err)
				}
				return &config, nil
			}
		}
	}

	return &config, nil
}

// SaveTestConfig saves the test configuration to a file.
func SaveTestConfig(config *TestConfig, configPath string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("error creating directory: %w", err)
	}

	// Marshal config to JSON
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return nil
}
