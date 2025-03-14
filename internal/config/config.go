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
}

// ServerConfig contains server configuration
type ServerConfig struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

// RTMConfig contains Remember The Milk API configuration
type RTMConfig struct {
	APIKey       string `yaml:"api_key"`
	SharedSecret string `yaml:"shared_secret"`
}

// AuthConfig contains authentication configuration
type AuthConfig struct {
	TokenPath string `yaml:"token_path"`
}

// LoadConfig loads configuration from the specified file
func LoadConfig(path string) (*Config, error) {
	// Read the config file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	// Parse the config file
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("error parsing config file: %w", err)
	}

	// Validate the config
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Expand path
	config.Auth.TokenPath = expandPath(config.Auth.TokenPath)

	return &config, nil
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
