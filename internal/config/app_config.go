// package config handles application configuration.
// file: internal/config/config.go
package config

import (
	"fmt" // Import slog
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging" // Import project logging helper
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// Initialize the logger at the package level
var logger = logging.GetLogger("config")

// Settings represents the application configuration.
// ... (comments remain the same)
type Settings struct {
	Server ServerConfig `yaml:"server"`
	RTM    RTMConfig    `yaml:"rtm"`
	Auth   AuthConfig   `yaml:"auth"`
}

// ServerConfig contains server configuration.
// ... (comments remain the same)
type ServerConfig struct {
	Name string `yaml:"name"`
	Port int    `yaml:"port"`
}

// RTMConfig contains RTM API configuration.
// ... (comments remain the same)
type RTMConfig struct {
	APIKey       string `yaml:"api_key"`
	SharedSecret string `yaml:"shared_secret"`
}

// AuthConfig contains authentication configuration.
// ... (comments remain the same)
type AuthConfig struct {
	TokenPath string `yaml:"token_path"`
}

// New creates a new configuration with default values.
// ... (comments remain the same)
func New() *Settings {
	logger.Debug("Creating new configuration settings with defaults.")
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
			// Use platform-specific config dir lookup if possible in future
			TokenPath: "~/.config/cowgnition/tokens",
		},
	}
}

// GetServerName returns the server name.
// ... (comments remain the same)
func (s *Settings) GetServerName() string {
	return s.Server.Name
}

// GetServerAddress returns the server address as host:port.
// ... (comments remain the same)
func (s *Settings) GetServerAddress() string {
	return fmt.Sprintf(":%d", s.Server.Port)
}

// ExpandPath expands ~ in paths to the user's home directory.
// ... (comments remain the same)
func ExpandPath(path string) (string, error) {
	logger.Debug("Attempting to expand path", "input_path", path)
	if !strings.HasPrefix(path, "~") {
		logger.Debug("Path does not start with '~', returning as is.")
		return path, nil // Return it directly.
	}

	home, err := os.UserHomeDir() // Get the user's home directory.
	if err != nil {
		// Add function context to Wrap message as per assessment example
		wrappedErr := errors.Wrap(err, "ExpandPath: failed to get user home directory")
		detailedErr := cgerr.ErrorWithDetails(
			wrappedErr, // Use the wrapped error with function context
			cgerr.CategoryConfig,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"input_path": path,
				"os_user":    os.Getenv("USER"), // USER might not be reliable, but kept from original
			},
		)
		// Log the error before returning
		logger.Error("Failed to get user home directory for path expansion", "error", fmt.Sprintf("%+v", detailedErr))
		return "", detailedErr
	}

	expandedPath := filepath.Join(home, path[1:]) // Join the home directory with the rest of the path.
	logger.Debug("Path expanded successfully", "input_path", path, "expanded_path", expandedPath)
	return expandedPath, nil
}
