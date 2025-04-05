// package config handles application configuration.
// file: internal/config/config.go.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	// Import the cockroachdb/errors package directly ONLY if needed for specific features NOT covered by cgerr helpers.
	// For basic wrapping/creation, we will use cgerr.
	// "github.com/cockroachdb/errors" // Keep only if absolutely necessary elsewhere, otherwise remove.
	"github.com/dkoosis/cowgnition/internal/logging" // Import project logging helper.
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// Initialize the logger at the package level.
var logger = logging.GetLogger("config")

// Settings represents the application configuration.
// Holds nested structs for server, RTM, and auth settings loaded from config file.
type Settings struct {
	Server ServerConfig `yaml:"server"`
	RTM    RTMConfig    `yaml:"rtm"`
	Auth   AuthConfig   `yaml:"auth"`
}

// ServerConfig contains server configuration.
// Defines the name and port for the MCP server.
type ServerConfig struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

// RTMConfig contains RTM API configuration.
// Stores API key and shared secret for interacting with the RTM service.
type RTMConfig struct {
	APIKey       string `yaml:"api_key"`
	SharedSecret string `yaml:"shared_secret"`
}

// AuthConfig contains authentication configuration.
// Specifies the path where authentication tokens are stored.
type AuthConfig struct {
	TokenPath string `yaml:"token_path"`
}

// New creates a new configuration with default values.
// Provides sensible defaults for server name, port, and token path.
func New() *Settings {
	logger.Debug("Creating new configuration settings with defaults.")
	return &Settings{
		Server: ServerConfig{
			Name: "CowGnition MCP Server",
			Port: 8080,
		},
		RTM: RTMConfig{
			APIKey:       "", // API Key must be provided via config file or environment.
			SharedSecret: "", // Shared Secret must be provided via config file or environment.
		},
		Auth: AuthConfig{
			// Use platform-specific config dir lookup if possible in future.
			TokenPath: "~/.config/cowgnition/tokens",
		},
	}
}

// GetServerName returns the server name.
// Accessor method for the configured server name.
func (s *Settings) GetServerName() string {
	return s.Server.Name
}

// GetServerAddress returns the server address as host:port.
// Formats the listen address for the server based on the configured port.
func (s *Settings) GetServerAddress() string {
	return fmt.Sprintf(":%d", s.Server.Port)
}

// ExpandPath expands ~ in paths to the user's home directory.
// Replaces the tilde prefix with the absolute path to the user's home directory.
func ExpandPath(path string) (string, error) {
	logger.Debug("Attempting to expand path.", "input_path", path)
	if !strings.HasPrefix(path, "~") {
		logger.Debug("Path does not start with '~', returning as is.")
		return path, nil // Return it directly.
	}

	home, err := os.UserHomeDir() // Get the user's home directory.
	if err != nil {
		// Wrap the OS error using the standardized helper from cgerr.
		wrappedErr := cgerr.Wrap(err, "ExpandPath: failed to get user home directory")
		// Add details using the standardized function.
		detailedErr := cgerr.ErrorWithDetails(
			wrappedErr, // Use the error wrapped by cgerr.
			cgerr.CategoryConfig,
			cgerr.CodeInternalError, // Or potentially a more specific config error code if defined.
			map[string]interface{}{
				"input_path": path,
				"os_user":    os.Getenv("USER"), // USER might not be reliable, but kept from original.
			},
		)
		// Log the error before returning, %+v ensures stack trace is logged.
		logger.Error("Failed to get user home directory for path expansion.", "error", fmt.Sprintf("%+v", detailedErr))
		return "", detailedErr
	}

	expandedPath := filepath.Join(home, path[1:]) // Join the home directory with the rest of the path.
	logger.Debug("Path expanded successfully.", "input_path", path, "expanded_path", expandedPath)
	return expandedPath, nil
}
