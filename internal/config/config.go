// Package config handles loading, parsing, and validating application configuration.
package config

// file: internal/config/config.go

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

// SchemaConfig holds schema-related settings.
type SchemaConfig struct {
	// DefaultVersion specifies the default MCP schema version the server targets (e.g., "2025-03-26").
	DefaultVersion string `yaml:"defaultVersion"`
	// CacheDir specifies the directory to store downloaded schema files. Defaults to OS cache dir.
	CacheDir string `yaml:"cacheDir"`
	// OverrideSource allows forcing the use of a specific schema file path or URL, bypassing default logic.
	OverrideSource string `yaml:"overrideSource,omitempty"`
	// BaseURL allows overriding the base URL for fetching MCP schemas. Defaults to official GitHub repo.
	BaseURL string `yaml:"baseURL,omitempty"`
}

// Config is the root configuration structure for the application.
type Config struct {
	// Server contains server-specific configuration.
	Server ServerConfig `yaml:"server"`

	// RTM contains Remember The Milk API configuration.
	RTM RTMConfig `yaml:"rtm"`

	// Auth contains authentication-related configuration.
	Auth AuthConfig `yaml:"auth"`

	// Schema contains schema loading and validation configuration.
	Schema SchemaConfig `yaml:"schema"`
}

// DefaultConfig returns a configuration with sensible defaults.
// This is used when no configuration file is specified.
func DefaultConfig() *Config {
	homeDir, err := os.UserHomeDir()
	tokenPath := ""
	cacheDir := ""

	if err == nil {
		// Determine default paths based on home directory if available.
		tokenPath = filepath.Join(homeDir, ".config", "cowgnition", "rtm_token.json")
		cacheDir = filepath.Join(homeDir, ".cache", "cowgnition", "schemas")
	} else {
		// Fallbacks if home directory cannot be determined.
		tokenPath = "rtm_token.json" //nolint:gosec // Best effort fallback.
		cacheDir = "schema_cache"    // Best effort fallback.
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
		Schema: SchemaConfig{
			DefaultVersion: "2025-03-26", // Set your primary target version.
			CacheDir:       cacheDir,
			BaseURL:        "https://raw.githubusercontent.com/modelcontextprotocol/specification/main/schema/",
			// OverrideSource is empty by default.
		},
	}

	// Apply overrides and checks after creating the default structure.
	applyEnvironmentOverrides(cfg, logging.GetLogger("config_default")) // Pass logger.

	return cfg
}

// LoadFromFile loads configuration from the specified YAML file.
// Returns an error if the file cannot be read or contains invalid YAML.
func LoadFromFile(path string) (*Config, error) {
	// Expand ~ in file path if present.
	if len(path) > 0 && path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get home directory to expand path")
		}
		path = filepath.Join(homeDir, path[1:])
	}

	// Read the file.
	// nolint:gosec // Path is provided by user command line arg or default.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config file: %s", path)
	}

	// Create a default config to establish base values and structure.
	config := DefaultConfig()

	// Parse YAML over the default config.
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, errors.Wrapf(err, "failed to parse config file: %s", path)
	}

	// Apply any environment variable overrides and checks again after loading from file.
	applyEnvironmentOverrides(config, logging.GetLogger("config_load")) // Pass logger.

	return config, nil
}

// applyEnvironmentOverrides applies any configuration overrides from environment variables.
// It also checks for potential misspellings if required RTM variables are missing.
// This function now requires a logger instance.
func applyEnvironmentOverrides(config *Config, logger logging.Logger) {
	apiKeyMissing := false
	sharedSecretMissing := false
	apiKeySource := "default"       // Track source: default, file, env.
	sharedSecretSource := "default" // Track source: default, file, env.

	// --- RTM Overrides ---
	// Check if RTM API key came from file (non-empty before env check).
	if config.RTM.APIKey != "" {
		apiKeySource = "file"
	}
	if apiKeyEnv := os.Getenv("RTM_API_KEY"); apiKeyEnv != "" {
		config.RTM.APIKey = apiKeyEnv
		apiKeySource = "env"
	}
	if config.RTM.APIKey == "" {
		apiKeyMissing = true
	}

	// Check if RTM shared secret came from file.
	if config.RTM.SharedSecret != "" {
		sharedSecretSource = "file"
	}
	if sharedSecretEnv := os.Getenv("RTM_SHARED_SECRET"); sharedSecretEnv != "" {
		config.RTM.SharedSecret = sharedSecretEnv
		sharedSecretSource = "env"
	}
	if config.RTM.SharedSecret == "" {
		sharedSecretMissing = true
	}

	// Log final sources.
	logger.Debug("RTM API Key source.", "source", apiKeySource)
	logger.Debug("RTM Shared Secret source.", "source", sharedSecretSource)

	// Check for Misspelled RTM Variables.
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
			// Log clearly if required vars are missing and no alternatives found.
			if apiKeyMissing {
				logger.Warn("Required RTM_API_KEY is missing from environment and config file.")
			}
			if sharedSecretMissing {
				logger.Warn("Required RTM_SHARED_SECRET is missing from environment and config file.")
			}
		}
	}

	// --- Server Overrides ---
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

	// --- Auth Overrides ---
	if tokenPath := os.Getenv("COWGNITION_TOKEN_PATH"); tokenPath != "" {
		logger.Debug("Overriding auth token path from environment.", "envVar", "COWGNITION_TOKEN_PATH", "value", tokenPath)
		config.Auth.TokenPath = tokenPath
	}

	// --- Schema Overrides ---
	if schemaDefaultVersion := os.Getenv("COWGNITION_SCHEMA_DEFAULT_VERSION"); schemaDefaultVersion != "" {
		logger.Debug("Overriding schema default version from environment.", "envVar", "COWGNITION_SCHEMA_DEFAULT_VERSION", "value", schemaDefaultVersion)
		config.Schema.DefaultVersion = schemaDefaultVersion
	}
	if schemaCacheDir := os.Getenv("COWGNITION_SCHEMA_CACHE_DIR"); schemaCacheDir != "" {
		logger.Debug("Overriding schema cache directory from environment.", "envVar", "COWGNITION_SCHEMA_CACHE_DIR", "value", schemaCacheDir)
		config.Schema.CacheDir = schemaCacheDir
	}
	if schemaOverrideSource := os.Getenv("COWGNITION_SCHEMA_OVERRIDE_SOURCE"); schemaOverrideSource != "" {
		logger.Debug("Overriding schema source from environment.", "envVar", "COWGNITION_SCHEMA_OVERRIDE_SOURCE", "value", schemaOverrideSource)
		config.Schema.OverrideSource = schemaOverrideSource
	}
	if schemaBaseURL := os.Getenv("COWGNITION_SCHEMA_BASE_URL"); schemaBaseURL != "" {
		logger.Debug("Overriding schema base URL from environment.", "envVar", "COWGNITION_SCHEMA_BASE_URL", "value", schemaBaseURL)
		config.Schema.BaseURL = schemaBaseURL
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
