// Package helpers provides testing utilities for the CowGnition MCP server.
package common

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// TestConfig holds configuration for tests that interact with external services.
type TestConfig struct {
	RTM struct {
		APIKey       string `json:"api_key"`
		SharedSecret string `json:"shared_secret"`
		AuthToken    string `json:"auth_token,omitempty"`
	} `json:"rtm"`
	Options struct {
		SkipLiveTests bool `json:"skip_live_tests"`
		DebugMode     bool `json:"debug_mode"`
		// Maximum number of API requests to make in a single test run
		MaxAPIRequests int `json:"max_api_requests"`
	} `json:"options"`
}

// DefaultConfigFile returns the default path for the test configuration file.
func DefaultConfigFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".cowgnition_test_config.json"
	}
	return filepath.Join(home, ".config", "cowgnition", "test_config.json")
}

// LoadTestConfig loads test configuration from a file or environment variables.
// If configPath is empty, it will try standard locations:
// 1. Environment variables (RTM_API_KEY, RTM_SHARED_SECRET, etc.)
// 2. ~/.config/cowgnition/test_config.json
// 3. ./.cowgnition_test_config.json.
func LoadTestConfig(configPath string) (*TestConfig, error) {
	var config TestConfig

	// Set reasonable defaults
	config.Options.MaxAPIRequests = 50 // Default limit to avoid excessive API usage

	// Try to load from file if path is provided
	if configPath != "" {
		file, err := os.ReadFile(configPath)
		if err == nil {
			if err := json.Unmarshal(file, &config); err != nil {
				return nil, fmt.Errorf("error parsing test config file: %w", err)
			}
			log.Printf("Loaded test config from: %s", configPath)
			return &config, nil
		}
	}

	// Try standard locations if no path provided
	standardPaths := []string{
		DefaultConfigFile(),
		".cowgnition_test_config.json",
	}

	for _, path := range standardPaths {
		file, err := os.ReadFile(path)
		if err == nil {
			if err := json.Unmarshal(file, &config); err != nil {
				log.Printf("Warning: Failed to parse test config from %s: %v", path, err)
				continue
			}
			log.Printf("Loaded test config from: %s", path)
			return &config, nil
		}
	}

	// Fall back to environment variables
	config.RTM.APIKey = os.Getenv("RTM_API_KEY")
	config.RTM.SharedSecret = os.Getenv("RTM_SHARED_SECRET")
	config.RTM.AuthToken = os.Getenv("RTM_AUTH_TOKEN")
	config.Options.SkipLiveTests = os.Getenv("RTM_SKIP_LIVE_TESTS") == "true"
	config.Options.DebugMode = os.Getenv("RTM_TEST_DEBUG") == "true"

	if config.RTM.APIKey != "" || config.RTM.SharedSecret != "" {
		log.Printf("Using RTM credentials from environment variables")
	}

	return &config, nil
}

// SaveTestConfig saves the test configuration to a file.
func SaveTestConfig(config *TestConfig, configPath string) error {
	if configPath == "" {
		configPath = DefaultConfigFile()
	}

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

	// Write to file with secure permissions
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	log.Printf("Saved test config to: %s", configPath)
	return nil
}

// HasRTMCredentials checks if the configuration contains valid RTM credentials.
func (c *TestConfig) HasRTMCredentials() bool {
	return c.RTM.APIKey != "" && c.RTM.SharedSecret != ""
}

// SetRTMCredentials sets the RTM API credentials.
func (c *TestConfig) SetRTMCredentials(apiKey, sharedSecret string) {
	c.RTM.APIKey = apiKey
	c.RTM.SharedSecret = sharedSecret
}

// SetRTMAuthToken sets the RTM authentication token.
func (c *TestConfig) SetRTMAuthToken(token string) {
	c.RTM.AuthToken = token
}
