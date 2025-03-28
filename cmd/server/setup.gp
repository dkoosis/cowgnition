// cmd/server/setup.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
)

// ClaudeDesktopConfig represents the structure of Claude Desktop's configuration file
type ClaudeDesktopConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// MCPServerConfig represents a server configuration in Claude Desktop
type MCPServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// runSetup performs the setup process for CowGnition.
// It configures both the local application and integrates with Claude Desktop.
func runSetup(configPath string) error {
	// Get executable path
	exePath, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "failed to get executable path")
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return errors.Wrap(err, "failed to get absolute executable path")
	}

	// Check and create local config
	err = createDefaultConfig(configPath)
	if err != nil {
		return errors.Wrap(err, "failed to create default configuration")
	}

	// Configure Claude Desktop
	err = configureClaudeDesktop(exePath, configPath)
	if err != nil {
		fmt.Printf("Warning: Failed to configure Claude Desktop automatically: %v\n", err)
		fmt.Println("You'll need to configure Claude Desktop manually.")
		printManualSetupInstructions(exePath, configPath)
	}

	// Print success message
	fmt.Println("✅ CowGnition setup complete!")
	fmt.Println("Next steps:")
	fmt.Println("1. Run 'cowgnition serve' to start the server")
	fmt.Println("2. Open Claude Desktop to start using CowGnition")
	fmt.Println("3. Type 'What are my RTM tasks?' to test the connection")

	return nil
}

// createDefaultConfig creates a default configuration file if none exists
func createDefaultConfig(configPath string) error {
	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Configuration file already exists at %s\n", configPath)
		return nil
	}

	// Ensure directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create configuration directory")
	}

	// Create default config
	fmt.Printf("Creating default configuration at %s\n", configPath)

	// Default config sample - we'll need to implement this based on your config structure
	defaultConfig := `server:
  name: "CowGnition RTM"
  port: 8080

rtm:
  api_key: ""
  shared_secret: ""

auth:
  token_path: "~/.config/cowgnition/tokens"
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		return errors.Wrap(err, "failed to write default configuration file")
	}

	fmt.Println("Please edit the configuration file to add your RTM API key and shared secret.")
	return nil
}

// configureClaudeDesktop updates Claude Desktop's configuration to include CowGnition
func configureClaudeDesktop(exePath, configPath string) error {
	// Determine Claude Desktop config path based on OS
	claudeConfigPath := getClaudeConfigPath()

	// Create args for the server
	args := []string{"serve", "--transport", "stdio", "--config", configPath}

	// Build the server configuration
	serverConfig := MCPServerConfig{
		Command: exePath,
		Args:    args,
	}

	// Read existing Claude config if it exists
	var claudeConfig ClaudeDesktopConfig
	if _, err := os.Stat(claudeConfigPath); err == nil {
		data, err := os.ReadFile(claudeConfigPath)
		if err != nil {
			return errors.Wrap(err, "failed to read Claude Desktop configuration")
		}

		if err := json.Unmarshal(data, &claudeConfig); err != nil {
			// If the file exists but is invalid, create a new one
			claudeConfig = ClaudeDesktopConfig{
				MCPServers: make(map[string]MCPServerConfig),
			}
		}
	} else {
		// Create new config if it doesn't exist
		claudeConfig = ClaudeDesktopConfig{
			MCPServers: make(map[string]MCPServerConfig),
		}
	}

	// Add our server to the config
	claudeConfig.MCPServers["cowgnition"] = serverConfig

	// Write the updated config
	data, err := json.MarshalIndent(claudeConfig, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal Claude Desktop configuration")
	}

	// Ensure directory exists
	claudeConfigDir := filepath.Dir(claudeConfigPath)
	if err := os.MkdirAll(claudeConfigDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create Claude Desktop configuration directory")
	}

	if err := os.WriteFile(claudeConfigPath, data, 0644); err != nil {
		return errors.Wrap(err, "failed to write Claude Desktop configuration")
	}

	fmt.Printf("Successfully configured Claude Desktop at %s\n", claudeConfigPath)
	return nil
}

// getClaudeConfigPath returns the path to Claude Desktop's configuration file based on the OS
func getClaudeConfigPath() string {
	var configDir string

	switch runtime.GOOS {
	case "darwin":
		homeDir, _ := os.UserHomeDir()
		configDir = filepath.Join(homeDir, "Library", "Application Support", "Claude")
	case "windows":
		configDir = filepath.Join(os.Getenv("APPDATA"), "Claude")
	default:
		homeDir, _ := os.UserHomeDir()
		configDir = filepath.Join(homeDir, ".config", "Claude")
	}

	return filepath.Join(configDir, "claude_desktop_config.json")
}

// printManualSetupInstructions prints instructions for manually configuring Claude Desktop
func printManualSetupInstructions(exePath, configPath string) {
	claudeConfigPath := getClaudeConfigPath()

	fmt.Println("\n==== Manual Claude Desktop Configuration ====")
	fmt.Printf("1. Create or edit the file at: %s\n", claudeConfigPath)
	fmt.Println("2. Add the following configuration:")

	configExample := fmt.Sprintf(`{
  "mcpServers": {
    "cowgnition": {
      "command": "%s",
      "args": ["serve", "--transport", "stdio", "--config", "%s"]
    }
  }
}`, exePath, configPath)

	fmt.Println(configExample)
	fmt.Println("3. Restart Claude Desktop to apply the changes")
	fmt.Println("==============================================")
}
