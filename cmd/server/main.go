// cmd/server/main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/mcp"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/dkoosis/cowgnition/internal/rtm"
	"gopkg.in/yaml.v3"
)

// ClaudeDesktopConfig represents the structure of Claude Desktop's configuration file.
type ClaudeDesktopConfig struct {
	MCPServers map[string]MCPServerConfig `json:"mcpServers"`
}

// MCPServerConfig represents a server configuration in Claude Desktop.
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

func main() {
	// Check if we have a subcommand.
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Process subcommands.
	switch os.Args[1] {
	case "setup":
		setupCmd := flag.NewFlagSet("setup", flag.ExitOnError)
		setupConfigPath := setupCmd.String("config", getDefaultConfigPath(), "Path to configuration file.")

		// Fix: Check error from Parse.
		if err := setupCmd.Parse(os.Args[2:]); err != nil {
			log.Fatalf("Failed to parse setup command flags: %+v", err)
		}

		if err := runSetup(*setupConfigPath); err != nil {
			log.Fatalf("Setup failed: %+v", err)
		}

	case "serve":
		serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
		transportType := serveCmd.String("transport", "http", "Transport type (http or stdio).")
		serveConfigPath := serveCmd.String("config", "", "Path to configuration file.")
		requestTimeout := serveCmd.Duration("request-timeout", 30*time.Second, "Timeout for JSON-RPC requests.")
		shutdownTimeout := serveCmd.Duration("shutdown-timeout", 5*time.Second, "Timeout for graceful shutdown.")

		// Fix: Check error from Parse.
		if err := serveCmd.Parse(os.Args[2:]); err != nil {
			log.Fatalf("Failed to parse serve command flags: %+v", err)
		}

		if err := runServer(*transportType, *serveConfigPath, *requestTimeout, *shutdownTimeout); err != nil {
			log.Fatalf("Server failed: %+v", err)
		}

	default:
		printUsage()
		os.Exit(1)
	}
}

// printUsage prints usage information for the command.
func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  cowgnition setup [options]  - Set up CowGnition and Claude Desktop integration")
	fmt.Println("  cowgnition serve [options]  - Start the CowGnition server")
	fmt.Println("\nRun 'cowgnition <command> -h' for help on a specific command.")
}

// getDefaultConfigPath returns the default path for the configuration file.
func getDefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "configs/cowgnition.yaml" // Fallback to local directory.
	}
	return filepath.Join(homeDir, ".config", "cowgnition", "cowgnition.yaml")
}

// runSetup performs the setup process for CowGnition and Claude Desktop integration.
func runSetup(configPath string) error {
	fmt.Println("Setting up CowGnition...")

	// Get executable path.
	exePath, err := os.Executable()
	if err != nil {
		return errors.Wrap(err, "failed to get executable path")
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return errors.Wrap(err, "failed to get absolute executable path")
	}

	// Check and create local config.
	err = createDefaultConfig(configPath)
	if err != nil {
		return errors.Wrap(err, "failed to create default configuration")
	}

	// Configure Claude Desktop.
	err = configureClaudeDesktop(exePath, configPath)
	if err != nil {
		fmt.Printf("Warning: Failed to configure Claude Desktop automatically: %v\n", err)
		fmt.Println("You'll need to configure Claude Desktop manually.")
		printManualSetupInstructions(exePath, configPath)
	}

	// Print success message.
	fmt.Println("✅ CowGnition setup complete!")
	fmt.Println("Next steps:")
	fmt.Println("1. Run 'cowgnition serve' to start the server")
	fmt.Println("2. Open Claude Desktop to start using CowGnition")
	fmt.Println("3. Type 'What are my RTM tasks?' to test the connection")

	return nil
}

// createDefaultConfig creates a default configuration file if none exists.
func createDefaultConfig(configPath string) error {
	// Check if config already exists.
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("Configuration file already exists at %s\n", configPath)
		return nil
	}

	// Ensure directory exists.
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create configuration directory")
	}

	// Create default config.
	fmt.Printf("Creating default configuration at %s\n", configPath)

	// Default config sample.
	defaultConfig := `server:
  name: "cowgnition"
  port: 8080

rtm:
  api_key: ""
  shared_secret: ""

auth:
  token_path: "~/.config/cowgnition/tokens"
`

	// Fix: Use more restrictive permissions (0600 instead of 0644).
	if err := os.WriteFile(configPath, []byte(defaultConfig), 0600); err != nil {
		return errors.Wrap(err, "failed to write default configuration file")
	}

	fmt.Println("Please edit the configuration file to add your RTM API key and shared secret.")
	return nil
}

// configureClaudeDesktop updates Claude Desktop's configuration to include CowGnition.
func configureClaudeDesktop(exePath, configPath string) error {
	// Determine Claude Desktop config path based on OS.
	claudeConfigPath := getClaudeConfigPath()

	// Create args for the server.
	args := []string{"serve", "--transport", "stdio", "--config", configPath}

	// Build the server configuration.
	serverConfig := MCPServerConfig{
		Command: exePath,
		Args:    args,
	}

	// Read existing Claude config if it exists.
	var claudeConfig ClaudeDesktopConfig
	if _, err := os.Stat(claudeConfigPath); err == nil {
		data, err := os.ReadFile(claudeConfigPath)
		if err != nil {
			return errors.Wrap(err, "failed to read Claude Desktop configuration")
		}

		if err := json.Unmarshal(data, &claudeConfig); err != nil {
			// If the file exists but is invalid, create a new one.
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
	if err := os.MkdirAll(claudeConfigDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create Claude Desktop configuration directory")
	}

	// Fix: Use more restrictive permissions (0600 instead of 0644).
	if err := os.WriteFile(claudeConfigPath, data, 0600); err != nil {
		return errors.Wrap(err, "failed to write Claude Desktop configuration")
	}

	fmt.Printf("Successfully configured Claude Desktop at %s\n", claudeConfigPath)
	return nil
}

// getClaudeConfigPath returns the path to Claude Desktop's configuration file based on the OS.
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

// printManualSetupInstructions prints instructions for manually configuring Claude Desktop.
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
	fmt.Println("3. Restart Claude Desktop to apply the changes.")
	fmt.Println("==============================================")
}

// runServer starts the MCP server with the given configuration.
func runServer(transportType, configPath string, requestTimeout, shutdownTimeout time.Duration) error {
	// Load configuration.
	cfg := config.New()

	// Load configuration from file if specified.
	if configPath != "" {
		log.Printf("Loading configuration from %s", configPath)

		// Read the file
		data, err := os.ReadFile(configPath)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to read configuration file: %s", configPath))
		}

		// Unmarshal YAML into config struct
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return errors.Wrap(err, "failed to parse configuration file")
		}

		log.Printf("Configuration loaded successfully")
	}

	// Create and start MCP server.
	server, err := mcp.NewServer(cfg)
	if err != nil {
		return errors.Wrap(err, "failed to create server")
	}

	// Set version.
	server.SetVersion("1.0.0")

	// Set transport type.
	if err := server.SetTransport(transportType); err != nil {
		err = cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to set transport"),
			cgerr.CategoryConfig,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"transport_type": transportType,
				"valid_types":    []string{"http", "stdio"},
			},
		)
		return err
	}

	// Set timeout configurations.
	server.SetRequestTimeout(requestTimeout)
	server.SetShutdownTimeout(shutdownTimeout)

	// Get RTM API credentials from environment or config.
	apiKey := os.Getenv("RTM_API_KEY")
	if apiKey == "" {
		apiKey = cfg.RTM.APIKey
	}

	sharedSecret := os.Getenv("RTM_SHARED_SECRET")
	if sharedSecret == "" {
		sharedSecret = cfg.RTM.SharedSecret
	}

	// Ensure API key and shared secret are available.
	if apiKey == "" || sharedSecret == "" {
		err := cgerr.ErrorWithDetails(
			errors.New("missing RTM API credentials"),
			cgerr.CategoryConfig,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"has_api_key":       apiKey != "",
				"has_shared_secret": sharedSecret != "",
				"rtm_api_key_env":   os.Getenv("RTM_API_KEY") != "",
				"rtm_secret_env":    os.Getenv("RTM_SHARED_SECRET") != "",
			},
		)
		return err
	}

	// Get token path from config or env.
	tokenPath := os.Getenv("RTM_TOKEN_PATH")
	if tokenPath == "" {
		tokenPath = cfg.Auth.TokenPath
	}

	// Expand ~ in token path if present.
	expandedPath, err := config.ExpandPath(tokenPath)
	if err != nil {
		return errors.Wrap(err, "failed to expand token path")
	}
	tokenPath = expandedPath

	// Create and register RTM auth provider.
	authProvider, err := rtm.NewAuthProvider(apiKey, sharedSecret, tokenPath)
	if err != nil {
		return errors.Wrap(err, "failed to create RTM auth provider")
	}
	server.RegisterResourceProvider(authProvider)

	// Handle graceful shutdown.
	if transportType == "http" {
		go func() {
			signals := make(chan os.Signal, 1)
			signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
			<-signals
			log.Println("Shutting down server...")
			if err := server.Stop(); err != nil {
				log.Printf("Error stopping server: %+v", err)
			}
		}()
	}

	// Start server.
	log.Printf("Starting CowGnition MCP server with %s transport", transportType)
	if err := server.Start(); err != nil {
		return errors.Wrap(err, "server failed to start")
	}

	return nil
}
