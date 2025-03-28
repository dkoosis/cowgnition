// cmd/server/server.go
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/mcp"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/dkoosis/cowgnition/internal/rtm"
)

// runServer starts the MCP server with the given configuration.
func runServer(transportType, configPath string, requestTimeout, shutdownTimeout time.Duration) error {
	// If no config path provided, try to find one
	if configPath == "" {
		foundConfig, configFound := findOrCreateConfig()
		if !configFound {
			return cgerr.ErrorWithDetails(
				errors.New("Failed to find or create a configuration file"),
				cgerr.CategoryConfig,
				cgerr.CodeInternalError,
				map[string]interface{}{
					"tried_locations": []string{
						"./configs/config.yaml",
						"./configs/cowgnition.yaml",
						"~/.config/cowgnition/cowgnition.yaml",
					},
				},
			)
		}
		configPath = foundConfig
	}

	if debugMode {
		log.Printf("Using configuration from: %s", configPath)
	}

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

// createAndConfigureServer creates and configures the MCP server.
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
