// Package config handles loading, parsing, and validating application configuration.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
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

// SchemaConfig holds schema-related settings (Simplified).
type SchemaConfig struct {
	// SchemaOverrideURI allows specifying a file path (file://...) or URL (http://...)
	// to load the schema from, overriding the default embedded schema. Optional.
	SchemaOverrideURI string `yaml:"schemaOverrideURI,omitempty"`
}

// Config is the root configuration structure for the application.
type Config struct {
	Server ServerConfig `yaml:"server"`
	RTM    RTMConfig    `yaml:"rtm"`
	Auth   AuthConfig   `yaml:"auth"`
	Schema SchemaConfig `yaml:"schema"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	homeDir, err := os.UserHomeDir()
	tokenPath := ""
	if err == nil {
		tokenPath = filepath.Join(homeDir, ".config", "cowgnition", "rtm_token.json")
	} else {
		tokenPath = "rtm_token.json" //nolint:gosec // Best effort fallback.
	}

	cfg := &Config{
		Server: ServerConfig{
			Name: "CowGnition RTM",
			Port: 8080,
		},
		RTM: RTMConfig{
			APIKey:       os.Getenv("RTM_API_KEY"),
			SharedSecret: os.Getenv("RTM_SHARED_SECRET"),
		},
		Auth: AuthConfig{
			TokenPath: tokenPath,
		},
		Schema: SchemaConfig{
			// SchemaOverrideURI is empty by default, meaning use embedded schema.
		},
	}
	applyEnvironmentOverrides(cfg, logging.GetLogger("config_default"))
	return cfg
}

// LoadFromFile loads configuration from the specified YAML file.
func LoadFromFile(path string) (*Config, error) {
	if len(path) > 0 && path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get home directory to expand path")
		}
		path = filepath.Join(homeDir, path[1:])
	}

	data, err := os.ReadFile(path) // nolint:gosec
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file: %s", path)
	}

	config := DefaultConfig() // Start with defaults.
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, errors.Wrapf(err, "failed to parse config file: %s", path)
	}

	applyEnvironmentOverrides(config, logging.GetLogger("config_load")) // Apply env vars over file config.
	return config, nil
}

// applyEnvironmentOverrides applies configuration overrides from environment variables.
func applyEnvironmentOverrides(config *Config, logger logging.Logger) {
	// RTM overrides (same as before).
	apiKeyMissing := false
	sharedSecretMissing := false
	apiKeySource := "default"
	sharedSecretSource := "default"

	if config.RTM.APIKey != "" {
		apiKeySource = "config"
	}
	if apiKeyEnv := os.Getenv("RTM_API_KEY"); apiKeyEnv != "" {
		config.RTM.APIKey = apiKeyEnv
		apiKeySource = "env"
	}
	if config.RTM.APIKey == "" {
		apiKeyMissing = true
	}

	if config.RTM.SharedSecret != "" {
		sharedSecretSource = "config"
	}
	if sharedSecretEnv := os.Getenv("RTM_SHARED_SECRET"); sharedSecretEnv != "" {
		config.RTM.SharedSecret = sharedSecretEnv
		sharedSecretSource = "env"
	}
	if config.RTM.SharedSecret == "" {
		sharedSecretMissing = true
	}

	logger.Debug("RTM API Key source.", "source", apiKeySource)
	logger.Debug("RTM Shared Secret source.", "source", sharedSecretSource)

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
		} else {
			if apiKeyMissing {
				logger.Warn("Required RTM_API_KEY is missing from environment and config file.")
			}
			if sharedSecretMissing {
				logger.Warn("Required RTM_SHARED_SECRET is missing from environment and config file.")
			}
		}
	}

	// Server overrides (same as before).
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

	// Auth overrides (same as before).
	if tokenPath := os.Getenv("COWGNITION_TOKEN_PATH"); tokenPath != "" {
		logger.Debug("Overriding auth token path from environment.", "envVar", "COWGNITION_TOKEN_PATH", "value", tokenPath)
		config.Auth.TokenPath = tokenPath
	}

	// Schema override.
	if schemaOverride := os.Getenv("COWGNITION_SCHEMA_OVERRIDE_URI"); schemaOverride != "" {
		logger.Debug("Overriding schema source from environment.", "envVar", "COWGNITION_SCHEMA_OVERRIDE_URI", "value", schemaOverride)
		config.Schema.SchemaOverrideURI = schemaOverride
	}
}

// findAlternativeRTMEnvVars scans the environment for potential misspellings.
func findAlternativeRTMEnvVars() []string {
	// (Function implementation remains the same as before).
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

		if lowerVarName == "rtm_api_key" || lowerVarName == "rtm_shared_secret" {
			continue
		}

		matchedPrefix := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(lowerVarName, prefix) {
				matchedPrefix = true
				break
			}
		}
		matchedExact := exactMatches[lowerVarName]

		if matchedPrefix || matchedExact {
			alternatives = append(alternatives, varName)
		}
	}
	return alternatives
}
