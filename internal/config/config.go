// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Settings represents the application configuration.
type Settings struct {
	Server ServerConfig
	RTM    RTMConfig
	Auth   AuthConfig
}

// ServerConfig contains server configuration.
type ServerConfig struct {
	Name string
	Port int
}

// RTMConfig contains RTM API configuration.
type RTMConfig struct {
	APIKey       string
	SharedSecret string
}

// AuthConfig contains authentication configuration.
type AuthConfig struct {
	TokenPath string
}

// New creates a new configuration with default values.
func New() *Settings {
	return &Settings{
		Server: ServerConfig{
			Name: "CowGnition MCP Server",
			Port: 8080,
		},
		RTM: RTMConfig{
			APIKey:       "",
			SharedSecret: "",
		},
		Auth: AuthConfig{
			TokenPath: "~/.config/cowgnition/tokens",
		},
	}
}

// GetServerName returns the server name.
func (s *Settings) GetServerName() string {
	return s.Server.Name
}

// GetServerAddress returns the server address as host:port.
func (s *Settings) GetServerAddress() string {
	return fmt.Sprintf(":%d", s.Server.Port)
}

// ExpandPath expands ~ in paths to the user's home directory.
func ExpandPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	return filepath.Join(home, path[1:]), nil
}
