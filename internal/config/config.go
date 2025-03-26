// Package config handles application configuration.
// file: internal/config/config.go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Settings represents the application configuration.
// It encapsulates all configuration settings for the application.
// This design allows for easy management and access to configuration values throughout the codebase.
type Settings struct {
	Server ServerConfig // Server: Configuration related to the server.
	RTM    RTMConfig    // RTM: Configuration for the Remember The Milk API.
	Auth   AuthConfig   // Auth: Configuration for authentication mechanisms.
}

// ServerConfig contains server configuration.
// This is separated to group server-specific settings together,
// promoting modularity and clarity in the configuration structure.
type ServerConfig struct {
	Name string // Name: The name of the server.
	Port int    // Port: The port on which the server listens.
}

// RTMConfig contains RTM API configuration.
// It holds the necessary credentials to interact with the Remember The Milk API.
type RTMConfig struct {
	APIKey       string // APIKey: The API key for RTM.
	SharedSecret string // SharedSecret: The shared secret for RTM.
}

// AuthConfig contains authentication configuration.
// This section manages settings related to user authentication,
// such as where to store tokens.
type AuthConfig struct {
	TokenPath string // TokenPath: The file path to store authentication tokens.
}

// New creates a new configuration with default values.
// This function initializes the configuration with sensible defaults,
// ensuring the application can run out-of-the-box without requiring immediate configuration.
// The use of default values enhances the user experience by providing a working setup initially.
func New() *Settings {
	return &Settings{
		Server: ServerConfig{
			Name: "CowGnition MCP Server", // Default server name.
			Port: 8080,                    // Default server port.
		},
		RTM: RTMConfig{
			APIKey:       "", // Default API key (empty).
			SharedSecret: "", // Default shared secret (empty).
		},
		Auth: AuthConfig{
			TokenPath: "~/.config/cowgnition/tokens", // Default token path in the user's home directory.
		},
	}
}

// GetServerName returns the server name.
// This provides a clean, encapsulated way to access the server name,
// adhering to good object-oriented practices.
func (s *Settings) GetServerName() string {
	return s.Server.Name
}

// GetServerAddress returns the server address as host:port.
// This method formats the server's address, combining the port with a colon,
// which is a common network address representation.
func (s *Settings) GetServerAddress() string {
	return fmt.Sprintf(":%d", s.Server.Port)
}

// ExpandPath expands ~ in paths to the user's home directory.
// This function is crucial for handling user-specific file paths,
// as it allows the application to locate files in a portable way across different systems.
// It abstracts away the complexity of determining the user's home directory.
//
// path string: The path to expand, which may contain ~.
//
// Returns:
//
//	string: The expanded path.
//	error:  An error if retrieving the user's home directory fails.
func ExpandPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~") { // If the path doesn't start with ~, it's already an absolute path.
		return path, nil // Return it directly.
	}

	home, err := os.UserHomeDir() // Get the user's home directory.
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err) // Return an error if we can't get the home directory.
	}

	return filepath.Join(home, path[1:]), nil // Join the home directory with the rest of the path.
}

// DocEnhanced: 2024-08-27
