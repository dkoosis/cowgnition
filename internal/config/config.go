// Package config handles loading, parsing, and validating application configuration.
// It defines the structure for configuration settings, provides default values,
// loads settings from files (e.g., YAML), and applies overrides from environment variables.
// file: internal/config/config.go.
package config

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	"gopkg.in/yaml.v3"
)

// ServerConfig contains settings specific to the MCP server component.
type ServerConfig struct {
	// Name is the human-readable name for the server, potentially displayed in logs or client UIs.
	Name string `yaml:"name"`
	// Port is the network port the server listens on when using HTTP transport. Ignored for stdio.
	Port int `yaml:"port"`
}

// RTMConfig contains settings required for integrating with the Remember The Milk API.
type RTMConfig struct {
	// APIKey is the application's API key obtained from RTM developer settings. Required.
	APIKey string `yaml:"api_key"`
	// SharedSecret is the secret corresponding to the APIKey, used for signing API requests. Required.
	SharedSecret string `yaml:"shared_secret"`
}

// AuthConfig contains settings related to authentication, particularly token storage.
type AuthConfig struct {
	// TokenPath specifies the file path where the RTM authentication token should be stored.
	// If secure OS storage (keychain/keyring) is unavailable, this file acts as a fallback.
	// Supports '~' expansion for home directory.
	TokenPath string `yaml:"token_path"`
}

// SchemaConfig holds settings related to JSON schema loading and validation.
type SchemaConfig struct {
	// SchemaOverrideURI allows specifying an external source (file:// or http(s)://)
	// for the MCP schema, overriding the default embedded schema. If empty, the embedded schema is used.
	// Loading failure from a specified URI is typically a fatal error during server startup.
	SchemaOverrideURI string `yaml:"schemaOverrideURI,omitempty"`
}

// Config is the root configuration structure for the CowGnition application.
// It aggregates configurations for different components like the server, RTM integration,
// authentication, and schema handling.
type Config struct {
	// Server holds general server settings (name, port).
	Server ServerConfig `yaml:"server"`
	// RTM holds credentials for the Remember The Milk API.
	RTM RTMConfig `yaml:"rtm"`
	// Auth holds authentication-related settings, primarily token storage location.
	Auth AuthConfig `yaml:"auth"`
	// Schema holds configuration for loading the MCP JSON schema.
	Schema SchemaConfig `yaml:"schema"`
}

// DefaultConfig returns a configuration populated with default values.
// It also attempts to read initial RTM credentials from standard environment variables
// (RTM_API_KEY, RTM_SHARED_SECRET) and sets a default token path within the user's config directory.
func DefaultConfig() *Config {
	homeDir, err := os.UserHomeDir()
	tokenPath := ""
	if err == nil {
		// Default path within user's config directory.
		tokenPath = filepath.Join(homeDir, ".config", "cowgnition", "rtm_token.json")
	} else {
		// Fallback if home directory cannot be determined.
		tokenPath = "rtm_token.json" //nolint:gosec // G101: Fallback path, not a secret itself.
	}

	cfg := &Config{
		Server: ServerConfig{
			Name: "CowGnition RTM", // Default server name.
			Port: 8080,             // Default HTTP port.
		},
		RTM: RTMConfig{
			APIKey:       os.Getenv("RTM_API_KEY"),       // Load from environment initially.
			SharedSecret: os.Getenv("RTM_SHARED_SECRET"), // Load from environment initially.
		},
		Auth: AuthConfig{
			TokenPath: tokenPath, // Set calculated default token path.
		},
		Schema: SchemaConfig{
			// SchemaOverrideURI is empty by default, meaning use embedded schema.
		},
	}
	// Apply any further environment overrides on top of these defaults.
	applyEnvironmentOverrides(cfg, logging.GetLogger("config_default"))
	return cfg
}

// LoadFromFile loads configuration from the specified YAML file path.
// It starts with default values, merges the values from the YAML file,
// and finally applies any environment variable overrides.
// Supports '~' expansion in the file path.
func LoadFromFile(path string) (*Config, error) {
	// Expand ~ character in path to user's home directory.
	if len(path) > 0 && path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get home directory to expand path")
		}
		path = filepath.Join(homeDir, path[1:])
	}

	// Read the configuration file.
	// #nosec G304 -- Path comes from command-line flag or default, considered trusted input.
	data, err := os.ReadFile(path)
	if err != nil {
		// Return error if file doesn't exist or cannot be read.
		// Use os.IsNotExist(err) to check specifically for file not found.
		return nil, errors.Wrapf(err, "failed to read config file: %s", path)
	}

	// Start with default configuration values.
	config := DefaultConfig()

	// Parse the YAML data, overriding defaults.
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, errors.Wrapf(err, "failed to parse config file YAML: %s", path)
	}

	// Apply environment variables, potentially overriding values from the file.
	applyEnvironmentOverrides(config, logging.GetLogger("config_load"))
	return config, nil
}

// applyEnvironmentOverrides applies configuration overrides from environment variables.
// Environment variables take precedence over values set in configuration files or defaults.
// Logs debug messages indicating the source of final values (default, config, env).
func applyEnvironmentOverrides(config *Config, logger logging.Logger) {
	apiKeyMissing := false
	sharedSecretMissing := false
	apiKeySource := "default"
	sharedSecretSource := "default"

	// Determine initial source before checking environment.
	if config.RTM.APIKey != "" {
		apiKeySource = "config file" // Assumes if not default, it came from the file load.
	}
	if config.RTM.SharedSecret != "" {
		sharedSecretSource = "config file"
	}

	// RTM overrides (API Key).
	if apiKeyEnv := os.Getenv("RTM_API_KEY"); apiKeyEnv != "" {
		config.RTM.APIKey = apiKeyEnv
		apiKeySource = "environment variable"
	}
	if config.RTM.APIKey == "" {
		apiKeyMissing = true
	}

	// RTM overrides (Shared Secret).
	if sharedSecretEnv := os.Getenv("RTM_SHARED_SECRET"); sharedSecretEnv != "" {
		config.RTM.SharedSecret = sharedSecretEnv
		sharedSecretSource = "environment variable"
	}
	if config.RTM.SharedSecret == "" {
		sharedSecretMissing = true
	}

	// Log final source and warnings if missing.
	logger.Debug("RTM API Key source determined.", "source", apiKeySource)
	if apiKeyMissing {
		logger.Warn("Required RTM_API_KEY is missing (checked environment and config file).")
	}
	logger.Debug("RTM Shared Secret source determined.", "source", sharedSecretSource)
	if sharedSecretMissing {
		logger.Warn("Required RTM_SHARED_SECRET is missing (checked environment and config file).")
	}

	// Server overrides.
	if portStr := os.Getenv("SERVER_PORT"); portStr != "" {
		if port, err := strconv.Atoi(portStr); err == nil && port > 0 && port < 65536 {
			logger.Debug("Overriding server port from environment.", "envVar", "SERVER_PORT", "value", port)
			config.Server.Port = port
		} else {
			logger.Warn("Invalid SERVER_PORT environment variable ignored.", "value", portStr, "error", err)
		}
	}
	if serverName := os.Getenv("SERVER_NAME"); serverName != "" {
		logger.Debug("Overriding server name from environment.", "envVar", "SERVER_NAME", "value", serverName)
		config.Server.Name = serverName
	}

	// Auth overrides.
	if tokenPath := os.Getenv("COWGNITION_TOKEN_PATH"); tokenPath != "" {
		logger.Debug("Overriding auth token path from environment.", "envVar", "COWGNITION_TOKEN_PATH", "value", tokenPath)
		// Expand ~ in environment variable path as well.
		if len(tokenPath) > 0 && tokenPath[0] == '~' {
			homeDir, err := os.UserHomeDir()
			if err == nil {
				tokenPath = filepath.Join(homeDir, tokenPath[1:])
			} else {
				logger.Warn("Could not expand '~' in COWGNITION_TOKEN_PATH env var.", "error", err)
			}
		}
		config.Auth.TokenPath = tokenPath
	}

	// Schema override.
	if schemaOverride := os.Getenv("COWGNITION_SCHEMA_OVERRIDE_URI"); schemaOverride != "" {
		logger.Debug("Overriding schema source from environment.", "envVar", "COWGNITION_SCHEMA_OVERRIDE_URI", "value", schemaOverride)
		config.Schema.SchemaOverrideURI = schemaOverride
	}
}
