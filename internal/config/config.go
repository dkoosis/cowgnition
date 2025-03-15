package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server ServerConfig `yaml:"server"`
	RTM    RTMConfig    `yaml:"rtm"`
	Auth   AuthConfig   `yaml:"auth"`
	// Add logging configuration
	Logging LoggingConfig `yaml:"logging"`
}

// ServerConfig contains server configuration
type ServerConfig struct {
	Name         string `yaml:"name"`
	Port         int    `yaml:"port"`
	StatusSecret string `yaml:"status_secret"`
	// Add development mode flag
	DevMode bool `yaml:"dev_mode"`
}

// RTMConfig contains Remember The Milk API configuration
type RTMConfig struct {
	APIKey       string `yaml:"api_key"`
	SharedSecret string `yaml:"shared_secret"`
	// Add default permission level
	Permission string `yaml:"permission"`
	// Add token refresh interval in hours
	TokenRefresh int `yaml:"token_refresh"`
}

// AuthConfig contains authentication configuration
type AuthConfig struct {
	TokenPath string `yaml:"token_path"`
	// Add option to disable token encryption for development
	DisableEncryption bool `yaml:"disable_encryption"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	File   string `yaml:"file"`
}

// LoadConfig loads configuration from the specified file
func LoadConfig(path string) (*Config, error) {
	// Validate and sanitize the path
	cleanPath := filepath.Clean(path)

	// Check for suspicious path elements
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("suspicious config path contains directory traversal: %s", path)
	}

	// Define safe prefixes for config files
	safeLocations := []string{
		"configs/",
		"/etc/cowgnition/",
		filepath.Join(os.Getenv("HOME"), ".config", "cowgnition"),
	}

	// Check if path is within a safe location
	pathIsValid := false
	if !filepath.IsAbs(cleanPath) {
		// Relative paths are allowed if they start with "configs/"
		if strings.HasPrefix(cleanPath, "configs/") {
			pathIsValid = true
		}
	} else {
		// For absolute paths, check against our safe locations
		for _, loc := range safeLocations {
			// Convert loc to absolute if needed
			absLoc := loc
			if !filepath.IsAbs(loc) {
				// Get working directory for relative paths
				wd, err := os.Getwd()
				if err != nil {
					continue
				}
				absLoc = filepath.Join(wd, loc)
			}

			if strings.HasPrefix(cleanPath, absLoc) {
				pathIsValid = true
				break
			}
		}
	}

	if !pathIsValid {
		return nil, fmt.Errorf("config file path is outside of allowed locations: %s", path)
	}

	// Read the config file
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Parse the config file
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	// Apply environment variable overrides
	applyEnvironmentOverrides(&config)

	// Set defaults for optional fields
	setDefaults(&config)

	// Validate the config
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Expand paths
	config.Auth.TokenPath = expandPath(config.Auth.TokenPath)
	if config.Logging.File != "" {
		config.Logging.File = expandPath(config.Logging.File)
	}

	return &config, nil
}

// applyEnvironmentOverrides applies environment variable overrides to the config
func applyEnvironmentOverrides(config *Config) {
	// RTM API key and secret can be overridden from environment
	if envAPIKey := os.Getenv("RTM_API_KEY"); envAPIKey != "" {
		config.RTM.APIKey = envAPIKey
	}
	if envSecret := os.Getenv("RTM_SHARED_SECRET"); envSecret != "" {
		config.RTM.SharedSecret = envSecret
	}

	// Port can be overridden
	if envPort := os.Getenv("PORT"); envPort != "" {
		if port, err := parseInt(envPort); err == nil && port > 0 {
			config.Server.Port = port
		}
	}

	// Token path can be overridden
	if envTokenPath := os.Getenv("RTM_TOKEN_PATH"); envTokenPath != "" {
		config.Auth.TokenPath = envTokenPath
	}

	// Status secret can be overridden
	if envStatusSecret := os.Getenv("STATUS_SECRET"); envStatusSecret != "" {
		config.Server.StatusSecret = envStatusSecret
	}
}

// setDefaults sets default values for optional configuration fields
func setDefaults(config *Config) {
	// Default server port
	if config.Server.Port <= 0 {
		config.Server.Port = 8080
	}

	// Default permission level
	if config.RTM.Permission == "" {
		config.RTM.Permission = "delete"
	}

	// Default token refresh interval
	if config.RTM.TokenRefresh <= 0 {
		config.RTM.TokenRefresh = 24 // 24 hours
	}

	// Default logging level
	if config.Logging.Level == "" {
		config.Logging.Level = "info"
	}

	// Default logging format
	if config.Logging.Format == "" {
		config.Logging.Format = "text"
	}

	// Generate a random status secret if none provided
	if config.Server.StatusSecret == "" {
		config.Server.StatusSecret = fmt.Sprintf("secret-%d", os.Getpid())
	}
}

// validateConfig checks if the configuration is valid
func validateConfig(config *Config) error {
	if config.Server.Name == "" {
		return fmt.Errorf("server name is required")
	}

	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}

	if config.RTM.APIKey == "" {
		return fmt.Errorf("RTM API key is required")
	}

	if config.RTM.SharedSecret == "" {
		return fmt.Errorf("RTM shared secret is required")
	}

	if config.Auth.TokenPath == "" {
		return fmt.Errorf("auth token path is required")
	}

	// Validate permission level
	validPermissions := map[string]bool{"read": true, "write": true, "delete": true}
	if !validPermissions[config.RTM.Permission] {
		return fmt.Errorf("invalid permission level: %s (must be read, write, or delete)", config.RTM.Permission)
	}

	// Validate logging level
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[config.Logging.Level] {
		return fmt.Errorf("invalid logging level: %s (must be debug, info, warn, or error)", config.Logging.Level)
	}

	// Validate logging format
	validFormats := map[string]bool{"text": true, "json": true}
	if !validFormats[config.Logging.Format] {
		return fmt.Errorf("invalid logging format: %s (must be text or json)", config.Logging.Format)
	}

	return nil
}

// expandPath expands ~ to the user's home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// parseInt parses a string to an integer with error handling
func parseInt(s string) (int, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}
