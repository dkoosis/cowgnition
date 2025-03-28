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

	// Initialize the MCPServers map regardless of whether the file exists
	claudeConfig.MCPServers = make(map[string]MCPServerConfig)

	if _, err := os.Stat(claudeConfigPath); err == nil {
		data, err := os.ReadFile(claudeConfigPath)
		if err != nil {
			return errors.Wrap(err, "configureClaudeDesktop: failed to read Claude Desktop configuration")
		}

		if err := json.Unmarshal(data, &claudeConfig); err != nil {
			// If unmarshalling fails, we've already initialized the map so we can continue
			log.Printf("Failed to parse existing Claude Desktop config, creating new one: %v", err)
		}
	}

	// Add our server to the config
	claudeConfig.MCPServers["cowgnition"] = serverConfig

	// Write the updated config
	data, err := json.MarshalIndent(claudeConfig, "", "  ")
	if err != nil {
		return errors.Wrap(err, "configureClaudeDesktop: failed to marshal Claude Desktop configuration")
	}

	// Ensure directory exists
	claudeConfigDir := filepath.Dir(claudeConfigPath)
	if err := os.MkdirAll(claudeConfigDir, 0755); err != nil {
		return errors.Wrap(err, "configureClaudeDesktop: failed to create Claude Desktop configuration directory")
	}

	if err := os.WriteFile(claudeConfigPath, data, 0600); err != nil {
		return errors.Wrap(err, "configureClaudeDesktop: failed to write Claude Desktop configuration")
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

// cmd/server/main.go - refactored runServer function with improved error handling and documentation

// runServer starts the MCP server with the given configuration.
// It handles loading the configuration, setting up the server with appropriate
// transport options, validating RTM credentials, and starting the server.
// The function properly manages graceful shutdown signals for HTTP transport.
//
// transportType string: The transport type to use ("http" or "stdio").
// configPath string: Path to the YAML configuration file (optional).
// requestTimeout time.Duration: Timeout for processing JSON-RPC requests.
// shutdownTimeout time.Duration: Timeout for graceful server shutdown.
//
// Returns:
//
//	error: Returns various detailed errors if initialization or startup fails.
func runServer(transportType, configPath string, requestTimeout, shutdownTimeout time.Duration) error {
	// Load configuration
	cfg, err := loadConfiguration(configPath)
	if err != nil {
		return err // Already contains appropriate error context
	}

	// Create and configure server
	server, err := createAndConfigureServer(cfg, transportType, requestTimeout, shutdownTimeout)
	if err != nil {
		return err // Already contains appropriate error context
	}

	// Get and validate RTM credentials
	apiKey, sharedSecret, err := getRTMCredentials(cfg)
	if err != nil {
		return err // Already contains appropriate error context
	}

	// Set up token storage
	tokenPath, err := getTokenPath(cfg)
	if err != nil {
		return err // Already contains appropriate error context
	}

	// Register RTM provider
	if err := registerRTMProvider(server, apiKey, sharedSecret, tokenPath); err != nil {
		return err // Already contains appropriate error context
	}

	// Handle graceful shutdown for HTTP transport
	setupGracefulShutdown(server, transportType)

	// Start server
	log.Printf("Starting CowGnition MCP server with %s transport", transportType)
	if err := server.Start(); err != nil {
		return errors.Wrap(err, "runServer: server failed to start")
	}

	return nil
}

// loadConfiguration loads the server configuration from a file.
// It creates a default configuration and then optionally overrides it
// with values from the specified YAML file. This approach ensures that
// even with a missing or partial config file, the server can still run
// with reasonable defaults.
//
// configPath string: Path to the configuration file to load (optional).
//
// Returns:
//
//	*config.Settings: The loaded configuration settings.
//	error: Error if the configuration file cannot be read or parsed.
func loadConfiguration(configPath string) (*config.Settings, error) {
	// Create default configuration
	cfg := config.New()

	// Load configuration from file if specified
	if configPath != "" {
		log.Printf("Loading configuration from %s", configPath)

		// Read the file
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, cgerr.ErrorWithDetails(
				errors.Wrap(err, "loadConfiguration: failed to read configuration file"),
				cgerr.CategoryConfig,
				cgerr.CodeInternalError,
				map[string]interface{}{
					"config_path": configPath,
					"file_exists": false,
					"os_user":     os.Getenv("USER"),
				},
			)
		}

		// Unmarshal YAML into config struct
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, cgerr.ErrorWithDetails(
				errors.Wrap(err, "loadConfiguration: failed to parse configuration file"),
				cgerr.CategoryConfig,
				cgerr.CodeInternalError,
				map[string]interface{}{
					"config_path":      configPath,
					"data_size":        len(data),
					"data_starts_with": string(data[:min(50, len(data))]),
					"yaml_error":       err.Error(),
				},
			)
		}

		log.Printf("Configuration loaded successfully")
	}

	return cfg, nil
}

// min returns the minimum of two integers.
// This helper function is used to safely preview configuration file contents
// without risk of out-of-bounds array access.
//
// a, b int: Two integers to compare.
//
// Returns:
//
//	int: The smaller of the two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// createAndConfigureServer creates and configures the MCP server.
// It initializes the server, sets its version, configures the transport type,
// and sets up timeout parameters. This centralized configuration ensures
// consistent server setup regardless of transport type.
//
// cfg *config.Settings: The configuration settings to use.
// transportType string: The transport type ("http" or "stdio").
// requestTimeout time.Duration: Timeout for processing JSON-RPC requests.
// shutdownTimeout time.Duration: Timeout for graceful server shutdown.
//
// Returns:
//
//	*mcp.Server: The configured server instance.
//	error: Error if server initialization or configuration fails.
func createAndConfigureServer(cfg *config.Settings, transportType string, requestTimeout, shutdownTimeout time.Duration) (*mcp.Server, error) {
	// Create server
	server, err := mcp.NewServer(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "createAndConfigureServer: failed to create server")
	}

	// Set version
	server.SetVersion("1.0.0")

	// Set transport type
	if err := server.SetTransport(transportType); err != nil {
		return nil, cgerr.ErrorWithDetails(
			errors.Wrap(err, "createAndConfigureServer: failed to set transport"),
			cgerr.CategoryConfig,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"transport_type": transportType,
				"valid_types":    []string{"http", "stdio"},
				"server_name":    cfg.GetServerName(),
			},
		)
	}

	// Set timeout configurations
	server.SetRequestTimeout(requestTimeout)
	server.SetShutdownTimeout(shutdownTimeout)

	return server, nil
}

// getRTMCredentials gets and validates RTM API credentials.
// It checks both environment variables and configuration settings
// to allow for flexible credential management. Environment variables
// take precedence over configuration file values to support secure
// deployment scenarios.
//
// cfg *config.Settings: The configuration settings to check.
//
// Returns:
//
//	string: The API key for Remember The Milk.
//	string: The shared secret for Remember The Milk.
//	error: Error if either credential is missing.
func getRTMCredentials(cfg *config.Settings) (string, string, error) {
	// Get RTM API credentials from environment or config
	apiKey := os.Getenv("RTM_API_KEY")
	if apiKey == "" {
		apiKey = cfg.RTM.APIKey
	}

	sharedSecret := os.Getenv("RTM_SHARED_SECRET")
	if sharedSecret == "" {
		sharedSecret = cfg.RTM.SharedSecret
	}

	// Ensure API key and shared secret are available
	if apiKey == "" || sharedSecret == "" {
		return "", "", cgerr.ErrorWithDetails(
			errors.New("getRTMCredentials: missing RTM API credentials - you'll need to mooove these into place"),
			cgerr.CategoryConfig,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"has_api_key":       apiKey != "",
				"has_shared_secret": sharedSecret != "",
				"rtm_api_key_env":   os.Getenv("RTM_API_KEY") != "",
				"rtm_secret_env":    os.Getenv("RTM_SHARED_SECRET") != "",
				"config_has_key":    cfg.RTM.APIKey != "",
				"config_has_secret": cfg.RTM.SharedSecret != "",
				"server_name":       cfg.GetServerName(),
			},
		)
	}

	return apiKey, sharedSecret, nil
}

// getTokenPath gets the expanded token path.
// It resolves the path where authentication tokens should be stored,
// expanding home directory references (tilde ~) for cross-platform support.
// Environment variables can override the configuration setting to support
// different deployment scenarios.
//
// cfg *config.Settings: The configuration settings to use.
//
// Returns:
//
//	string: The expanded token storage path.
//	error: Error if path expansion fails.
func getTokenPath(cfg *config.Settings) (string, error) {
	// Get token path from config or env
	tokenPath := os.Getenv("RTM_TOKEN_PATH")
	if tokenPath == "" {
		tokenPath = cfg.Auth.TokenPath
	}

	// Expand ~ in token path if present
	expandedPath, err := config.ExpandPath(tokenPath)
	if err != nil {
		return "", cgerr.ErrorWithDetails(
			errors.Wrap(err, "getTokenPath: failed to expand token path"),
			cgerr.CategoryConfig,
			cgerr.CodeInternalError,
			map[string]interface{}{
				"token_path": tokenPath,
				"user_home":  os.Getenv("HOME"),
				"os_user":    os.Getenv("USER"),
			},
		)
	}

	return expandedPath, nil
}

// registerRTMProvider creates and registers the RTM authentication provider.
// This function connects the authentication provider to the MCP server,
// enabling Remember The Milk authentication flows. The auth provider
// handles token storage, retrieval, and validation.
//
// server *mcp.Server: The MCP server to register with.
// apiKey string: The RTM API key.
// sharedSecret string: The RTM shared secret.
// tokenPath string: The path where auth tokens should be stored.
//
// Returns:
//
//	error: Error if provider creation or registration fails.
func registerRTMProvider(server *mcp.Server, apiKey, sharedSecret, tokenPath string) error {
	// Create and register RTM auth provider
	authProvider, err := rtm.NewAuthProvider(apiKey, sharedSecret, tokenPath)
	if err != nil {
		// NewAuthProvider already returns a well-formed cgerr error with context
		return errors.Wrap(err, "registerRTMProvider: failed to create RTM auth provider")
	}
	server.RegisterResourceProvider(authProvider)
	return nil
}

// setupGracefulShutdown sets up signal handling for graceful shutdown.
// For HTTP transport, this function creates a goroutine that listens for
// termination signals (SIGINT, SIGTERM) and initiates a graceful server
// shutdown. This ensures ongoing requests can complete before the server exits.
// No graceful shutdown is needed for stdio transport as it terminates with
// the parent process.
//
// server *mcp.Server: The server to set up graceful shutdown for.
// transportType string: The transport type ("http" or "stdio").
func setupGracefulShutdown(server *mcp.Server, transportType string) {
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
}
