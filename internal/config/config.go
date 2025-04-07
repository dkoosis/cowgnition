// file: internal/config/config.go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
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

	return &Config{
		Server: ServerConfig{
			Name: "CowGnition RTM",
			Port: 8080,
		},
		RTM: RTMConfig{
			// Note: These will likely be empty and require setting
			// through environment variables or configuration file
			APIKey:       os.Getenv("RTM_API_KEY"),
			SharedSecret: os.Getenv("RTM_SHARED_SECRET"),
		},
		Auth: AuthConfig{
			TokenPath: tokenPath,
		},
	}
}

// LoadFromFile loads configuration from the specified YAML file.
// Returns an error if the file cannot be read or contains invalid YAML.
func LoadFromFile(path string) (*Config, error) {
	// Expand ~ in file path if present
	if len(path) > 0 && path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get home directory")
		}
		path = filepath.Join(homeDir, path[1:])
	}

	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read config file")
	}

	// Parse YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, errors.Wrap(err, "failed to parse config file")
	}

	// Apply any environment variable overrides
	applyEnvironmentOverrides(&config)

	return &config, nil
}

// applyEnvironmentOverrides applies any configuration overrides from environment variables.
// This allows for easier configuration in containerized environments.
func applyEnvironmentOverrides(config *Config) {
	// Override RTM API key if environment variable is set
	if apiKey := os.Getenv("RTM_API_KEY"); apiKey != "" {
		config.RTM.APIKey = apiKey
	}

	// Override RTM shared secret if environment variable is set
	if sharedSecret := os.Getenv("RTM_SHARED_SECRET"); sharedSecret != "" {
		config.RTM.SharedSecret = sharedSecret
	}

	// Override server port if environment variable is set
	if portStr := os.Getenv("SERVER_PORT"); portStr != "" {
		if port, err := parsePort(portStr); err == nil && port > 0 {
			config.Server.Port = port
		}
	}
}

// parsePort is a helper function to convert a string port to an integer.
func parsePort(portStr string) (int, error) {
	var port int
	_, err := fmt.Sscanf(portStr, "%d", &port)
	return port, err
}
