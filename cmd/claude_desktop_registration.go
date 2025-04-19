// cmd/server/claude_desktop_registration.go
package main

import (
	"bufio" // Added import.
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings" // Added import.

	"github.com/cockroachdb/errors"
)

// ClaudeDesktopConfig represents the structure of Claude Desktop's configuration file.
// It holds the mapping of MCP server configurations available to Claude Desktop.
type ClaudeDesktopConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// MCPServerConfig represents a server configuration in Claude Desktop.
// It contains the command to execute, arguments, and optional environment variables.
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// runSetup performs the setup process for CowGnition.
// It configures both the local application and integrates with Claude Desktop.
// The function creates a default configuration if one doesn't exist and
// attempts to automatically configure Claude Desktop for seamless integration.
//
// configPath string: Path where the configuration file should be created or exists.
//
// Returns:
//
//	error: An error if setup fails, nil on success.
func runSetup(configPath string) error {
	// Get executable path.
	exePath, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "failed to get executable path")
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return errors.Wrap(err, "failed to get absolute executable path")
	}

	if debugMode {
		log.Printf("Using executable path: %s", exePath)
	}

	// Prompt for RTM Credentials.
	apiKey, sharedSecret, promptErr := promptForRTMCredentials()
	if promptErr != nil {
		return errors.Wrap(promptErr, "failed to get RTM credentials")
	}

	// Check and create local config.
	err = createDefaultConfig(configPath)
	if err != nil {
		return errors.Wrap(err, "failed to create default configuration")
	}

	// Configure Claude Desktop.
	err = configureClaudeDesktop(exePath, configPath, apiKey, sharedSecret)
	if err != nil {
		fmt.Printf("Warning: Failed to configure Claude Desktop automatically: %v\n", err)
		fmt.Println("You'll need to configure Claude Desktop manually.")
		printManualSetupInstructions(exePath, configPath)
	}

	// Print success message.
	fmt.Println("✅ CowGnition setup complete.")
	fmt.Println("Next steps:")
	fmt.Println("1. Run 'cowgnition serve' to start the server")
	fmt.Println("2. Open Claude Desktop to start using CowGnition")
	fmt.Println("3. Type 'What are my RTM tasks?' to test the connection")

	return nil
}

// promptForRTMCredentials interactively asks the user for RTM credentials.
//
// Returns:
//
//	apiKey string: The entered RTM API Key.
//	sharedSecret string: The entered RTM Shared Secret.
//	err error: An error if reading fails or input is empty.
func promptForRTMCredentials() (apiKey string, sharedSecret string, err error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter RTM API Key: ")
	apiKey, err = reader.ReadString('\n')
	if err != nil {
		return "", "", errors.Wrap(err, "failed to read API Key")
	}
	apiKey = strings.TrimSpace(apiKey)

	fmt.Print("Enter RTM Shared Secret: ")
	sharedSecret, err = reader.ReadString('\n')
	if err != nil {
		return "", "", errors.Wrap(err, "failed to read Shared Secret")
	}
	sharedSecret = strings.TrimSpace(sharedSecret)

	if apiKey == "" || sharedSecret == "" {
		return "", "", errors.New("API Key and Shared Secret cannot be empty")
	}

	return apiKey, sharedSecret, nil
}

// createDefaultConfig creates a default configuration file if none exists.
// If the file already exists, it leaves it unchanged.
//
// configPath string: Path where the configuration file should be created.
//
// Returns:
//
//	error: An error if file creation fails, nil on success.
func createDefaultConfig(configPath string) error {
	// Check if config already exists.
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Configuration file already exists at %s\n", configPath)
		return nil
	}

	// Ensure directory exists.
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return errors.Wrap(err, "failed to create configuration directory")
	}

	// Create default config.
	fmt.Printf("Creating default configuration at %s\n", configPath)

	// Default config sample with Remember The Milk settings.
	defaultConfig := `server:
  name: "CowGnition RTM"
  port: 8080

# RTM credentials are now set via 'cowgnition setup' and configured
# in Claude Desktop's config to be passed as environment variables.
# You generally do not need to set them here.
# rtm:
#  api_key: ""
#  shared_secret: ""

auth:
  # Default path for storing the RTM auth *token* (not API key/secret).
  token_path: "~/.config/cowgnition/rtm_token.json"
`

	// Use more secure file permissions (0600 instead of 0644).
	if err := os.WriteFile(configPath, []byte(defaultConfig), 0600); err != nil {
		return errors.Wrap(err, "failed to write default configuration file")
	}

	// fmt.Println("Please edit the configuration file to add your RTM API key and shared secret.") // Removed this instruction.
	return nil
}

// configureClaudeDesktop updates Claude Desktop's configuration to include CowGnition.
// It reads the existing configuration (if any), adds the CowGnition server entry with env vars,
// and writes the updated configuration back to disk.
//
// exePath string: Path to the CowGnition executable.
// configPath string: Path to the CowGnition configuration file.
// apiKey string: The RTM API Key.
// sharedSecret string: The RTM Shared Secret.
//
// Returns:
//
//	error: An error if configuration fails, nil on success.
func configureClaudeDesktop(exePath, configPath, apiKey, sharedSecret string) error {
	// Determine Claude Desktop config path based on OS.
	claudeConfigPath := getClaudeConfigPath()

	if debugMode {
		log.Printf("Claude Desktop config path: %s", claudeConfigPath)
	}

	// Create args for the server.
	args := []string{"serve", "--transport", "stdio", "--config", configPath}

	// Build the server configuration with environment variables.
	serverConfig := MCPServerConfig{
		Command: exePath,
		Args:    args,
		Env:     make(map[string]string), // Initialize the Env map.
	}
	// Add credentials to the environment map for the server process.
	serverConfig.Env["RTM_API_KEY"] = apiKey
	serverConfig.Env["RTM_SHARED_SECRET"] = sharedSecret
	// Optionally add log level if desired.
	// serverConfig.Env["LOG_LEVEL"] = "debug".

	// Read existing Claude config if it exists.
	var claudeConfig ClaudeDesktopConfig
	if _, err := os.Stat(claudeConfigPath); err == nil {
		// #nosec G304 -- Path is determined based on OS, not user input.
		data, err := os.ReadFile(claudeConfigPath)
		if err != nil {
			return errors.Wrap(err, "failed to read Claude Desktop configuration")
		}

		if err := json.Unmarshal(data, &claudeConfig); err != nil {
			// If the file exists but is invalid, create a new one.
			if debugMode {
				log.Printf("Failed to parse existing Claude Desktop config, creating new one: %v", err)
			}
			claudeConfig = ClaudeDesktopConfig{
				MCPServers: make(map[string]MCPServerConfig),
			}
		}
	} else {
		// Create new config if it doesn't exist.
		claudeConfig = ClaudeDesktopConfig{
			MCPServers: make(map[string]MCPServerConfig),
		}
	}

	// Add our server to the config.
	claudeConfig.MCPServers["cowgnition"] = serverConfig

	// Write the updated config.
	data, err := json.MarshalIndent(claudeConfig, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal Claude Desktop configuration")
	}

	// Ensure directory exists.
	claudeConfigDir := filepath.Dir(claudeConfigPath)
	if err := os.MkdirAll(claudeConfigDir, 0700); err != nil {
		return errors.Wrap(err, "failed to create Claude Desktop configuration directory")
	}

	// Use more secure file permissions (0600 instead of 0644).
	if err := os.WriteFile(claudeConfigPath, data, 0600); err != nil {
		return errors.Wrap(err, "failed to write Claude Desktop configuration")
	}

	fmt.Printf("Successfully configured Claude Desktop at %s\n", claudeConfigPath)
	return nil
}

// getClaudeConfigPath returns the path to Claude Desktop's configuration file based on the OS.
// It handles the different filesystem locations for each supported operating system.
//
// Returns:
//
//	string: The path to the Claude Desktop configuration file.
func getClaudeConfigPath() string {
	var configDir string

	switch runtime.GOOS {
	case "darwin":
		homeDir, _ := os.UserHomeDir()
		configDir = filepath.Join(homeDir, "Library", "Application Support", "Claude")
	case "windows":
		configDir = filepath.Join(os.Getenv("APPDATA"), "Claude")
	default: // Assume Linux/other Unix-like.
		homeDir, _ := os.UserHomeDir()
		configDir = filepath.Join(homeDir, ".config", "Claude")
	}

	return filepath.Join(configDir, "claude_desktop_config.json")
}

// printManualSetupInstructions prints instructions for manually configuring Claude Desktop.
// This is used as a fallback when automatic configuration fails.
//
// exePath string: Path to the CowGnition executable.
// configPath string: Path to the CowGnition configuration file.
func printManualSetupInstructions(exePath, configPath string) {
	claudeConfigPath := getClaudeConfigPath()

	fmt.Println("\n==== Manual Claude Desktop Configuration ====")
	fmt.Printf("1. Create or edit the file at: %s\n", claudeConfigPath)
	fmt.Println("2. Add the following configuration (replace placeholders):")

	// Example excludes the Env block as manual setup requires user intervention anyway.
	// They would need to get the API Key/Secret separately.
	configExample := fmt.Sprintf(`{
  "mcpServers": {
    "cowgnition": {
      "command": "%s",
      "args": ["serve", "--transport", "stdio", "--config", "%s"],
      "env": {
          "RTM_API_KEY": "YOUR_RTM_API_KEY",
          "RTM_SHARED_SECRET": "YOUR_RTM_SHARED_SECRET"
      }
    }
    // Add other servers here if needed...
  }
}`, exePath, configPath) // Using printf for formatting.

	fmt.Println(configExample)
	fmt.Println("3. Replace YOUR_RTM_API_KEY and YOUR_RTM_SHARED_SECRET with your actual credentials.")
	fmt.Println("4. Restart Claude Desktop to apply the changes.")
	fmt.Println("==============================================")
}
