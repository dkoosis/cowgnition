// Package config handles loading, parsing, and validating application configuration.
package config

// file: internal/config/config.go

import (
	"fmt"
	"os"
	"path/filepath"
	"strings" // Added strings import.

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging" // Assuming logger is needed.
	"gopkg.in/yaml.v3"
)

// ServerConfig contains settings specific to the server component.
type ServerConfig struct {
	// Name is the human-readable name for the server, displayed in logs and UI.
	Name string `yaml:"name"`

	// Port is the HTTP port to listen on when using HTTP transport.
	Port int `yaml:"port"`
}

// RTMConfig contains settings for the Remember The Milk API integration.
type RTMConfig struct {
	// APIKey is the Remember The Milk API key from developer.rememberthemilk.com.
	APIKey string `yaml:"api_key"`

	// SharedSecret is the shared secret for the RTM API from developer.rememberthemilk.com.
	SharedSecret string `yaml:"shared_secret"`
}

// AuthConfig contains authentication-related settings.
type AuthConfig struct {
	// TokenPath is the path where auth tokens will be stored securely.
	TokenPath string `yaml:"token_path"`
}

// Config is the root configuration structure for the application.
type Config struct {
	// Server contains server-specific configuration.
	Server ServerConfig `yaml:"server"`

	// RTM contains Remember The Milk API configuration.
	RTM RTMConfig `yaml:"rtm"`

	// Auth contains authentication-related configuration.
	Auth AuthConfig `yaml:"auth"`
}

// DefaultConfig returns a configuration with sensible defaults.
// This is used when no configuration file is specified.
func DefaultConfig() *Config {
	homeDir, err := os.UserHomeDir()
	tokenPath := ""

	if err == nil {
		tokenPath = filepath.Join(homeDir, ".config", "cowgnition", "tokens")
	}

	cfg := &Config{
		Server: ServerConfig{
			Name: "CowGnition RTM",
			Port: 8080,
		},
		RTM: RTMConfig{
			// Note: These will likely be empty and require setting
			// through environment variables or configuration file.
			APIKey:       os.Getenv("RTM_API_KEY"),
			SharedSecret: os.Getenv("RTM_SHARED_SECRET"),
		},
		Auth: AuthConfig{
			TokenPath: tokenPath,
		},
	}

	// Apply overrides and checks after creating the default structure.
	// Pass a default logger for the check, adjust if you have a better way.
	applyEnvironmentOverrides(cfg, logging.GetLogger("config_env_check"))

	return cfg
}

// LoadFromFile loads configuration from the specified YAML file.
// Returns an error if the file cannot be read or contains invalid YAML.
func LoadFromFile(path string) (*Config, error) {
	// Expand ~ in file path if present.
	if len(path) > 0 && path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get home directory")
		}
		path = filepath.Join(homeDir, path[1:])
	}

	// Read the file.
	// nolint:gosec
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config file")
	}

	// Parse YAML.
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, errors.Wrap(err, "failed to parse config file")
	}

	// Apply any environment variable overrides and checks.
	// Pass a default logger for the check, adjust if needed.
	applyEnvironmentOverrides(&config, logging.GetLogger("config_env_check"))

	return &config, nil
}

// applyEnvironmentOverrides applies any configuration overrides from environment variables.
// It also checks for potential misspellings if required RTM variables are missing.
// This function now requires a logger instance.
func applyEnvironmentOverrides(config *Config, logger logging.Logger) {
	apiKeyMissing := false
	sharedSecretMissing := false

	// Override RTM API key if environment variable is set.
	if apiKey := os.Getenv("RTM_API_KEY"); apiKey != "" {
		config.RTM.APIKey = apiKey
	} else if config.RTM.APIKey == "" { // Check if still empty after potential file load.
		apiKeyMissing = true
	}

	// Override RTM shared secret if environment variable is set.
	if sharedSecret := os.Getenv("RTM_SHARED_SECRET"); sharedSecret != "" {
		config.RTM.SharedSecret = sharedSecret
	} else if config.RTM.SharedSecret == "" { // Check if still empty after potential file load.
		sharedSecretMissing = true
	}

	// --- Added Check for Misspelled RTM Variables ---
	if apiKeyMissing || sharedSecretMissing {
		foundAlternatives := findAlternativeRTMEnvVars()
		if len(foundAlternatives) > 0 {
			missingVars := []string{}
			if apiKeyMissing {
				missingVars = append(missingVars, "RTM_API_KEY")
			}
			if sharedSecretMissing {
				missingVars = append(missingVars, "RTM_SHARED_SECRET")
			}
			logger.Warn(
				"Required RTM environment variable(s) missing.",
				"missing", strings.Join(missingVars, ", "),
				"suggestion", "Found possible alternatives in environment. Did you misspell the variable name?",
				"foundNames", strings.Join(foundAlternatives, ", "),
			)
		}
	}
	// --- End Added Check ---

	// Override server port if environment variable is set.
	if portStr := os.Getenv("SERVER_PORT"); portStr != "" {
		if port, err := parsePort(portStr); err == nil && port > 0 {
			config.Server.Port = port
		}
	}
}

// findAlternativeRTMEnvVars scans the environment for variables that might be
// misspelled versions of the required RTM keys.
func findAlternativeRTMEnvVars() []string {
	alternatives := []string{}
	prefixes := []string{"rtm_", "remember_", "rmilk_"}
	exactMatches := map[string]bool{
		"rtm_key":           true,
		"rtm_secret":        true,
		"rtm_shared_secret": true,
	}

	for _, envVar := range os.Environ() {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) < 1 {
			continue
		}
		varName := parts[0]
		lowerVarName := strings.ToLower(varName)

		// Skip the correctly named variables.
		if lowerVarName == "rtm_api_key" || lowerVarName == "rtm_shared_secret" {
			continue
		}

		// Check prefixes.
		matchedPrefix := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(lowerVarName, prefix) {
				matchedPrefix = true
				break
			}
		}

		// Check exact (but incorrect) names.
		matchedExact := exactMatches[lowerVarName]

		if matchedPrefix || matchedExact {
			// Add the *original* case name to the list.
			alternatives = append(alternatives, varName)
		}
	}
	return alternatives
}

// parsePort is a helper function to convert a string port to an integer.
func parsePort(portStr string) (int, error) {
	var port int
	_, err := fmt.Sscanf(portStr, "%d", &port)
	return port, err
}
