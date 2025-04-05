// cmd/server/server_logic.go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/mcp"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/dkoosis/cowgnition/internal/rtm"
)

// Initialize the logger at the package level.
var serverLogicLogger = logging.GetLogger("server_logic")

// runServer loads config, creates, configures, and starts the MCP server.
func runServer(transportType, configPath string, requestTimeout, shutdownTimeout time.Duration) error {
	serverLogicLogger.Info("Starting server run sequence",
		"transport", transportType,
		"config_path", configPath,
		"request_timeout", requestTimeout,
		"shutdown_timeout", shutdownTimeout,
	)

	// Load configuration.
	cfg, err := loadConfiguration(configPath)
	if err != nil {
		return errors.Wrapf(err, "runServer: failed to load configuration from '%s'", configPath)
	}
	serverLogicLogger.Info("Configuration loaded successfully")

	// Create base server.
	// Note: NewServer doesn't currently return an error in the snippet, but handle if it could.
	server, err := mcp.NewServer(cfg)
	if err != nil {
		return errors.Wrapf(err, "runServer: failed to create base server instance")
	}
	serverLogicLogger.Info("Base MCP server instance created", "server_name", cfg.GetServerName())

	// Set version.
	server.SetVersion(Version) // Assuming Version is defined elsewhere in main.
	serverLogicLogger.Debug("Server version set", "version", Version)

	// Set transport type.
	if err := server.SetTransport(transportType); err != nil {
		// L47: Add function context to existing Wrap message within cgerr.
		wrappedErr := errors.Wrap(err, "runServer: failed to set transport type")
		return cgerr.ErrorWithDetails(
			wrappedErr,
			cgerr.CategoryConfig,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"transport_type": transportType,
				"valid_types":    []string{"http", "stdio"},
				"server_name":    cfg.GetServerName(),
			},
		)
	}

	// Set timeout configurations.
	server.SetRequestTimeout(requestTimeout)
	server.SetShutdownTimeout(shutdownTimeout)
	serverLogicLogger.Debug("Server transport and timeouts configured", "transport", transportType, "request_timeout", requestTimeout, "shutdown_timeout", shutdownTimeout)

	// Get RTM credentials
	apiKey, sharedSecret, err := getRTMCredentials(cfg)
	if err != nil {
		return errors.Wrap(err, "runServer: failed to get RTM credentials") // Wrapf not needed if no extra vars.
	}
	serverLogicLogger.Info("RTM credentials obtained/validated")

	// Get and expand token path.
	tokenPath, err := getTokenPath(cfg)
	if err != nil {
		return errors.Wrap(err, "runServer: failed to get token path") // Wrapf not needed
	}
	serverLogicLogger.Debug("Token path determined", "path", tokenPath)

	// Register RTM provider
	if err := registerRTMProvider(server, apiKey, sharedSecret, tokenPath); err != nil {
		return errors.Wrap(err, "runServer: failed to register RTM provider") // Wrapf not needed
	}

	// Create the connection server wrapper
	connServer, err := mcp.NewConnectionServer(server)
	if err != nil {
		return errors.Wrapf(err, "runServer: failed to create connection server wrapper")
	}
	serverLogicLogger.Info("Connection server wrapper created")

	// Setup graceful shutdown *before* starting the server
	// Using cancellable context is often preferred over signal handling here,
	// but keeping signal handling for now as per original logic.
	stopChan := make(chan struct{}) // Channel to signal server stop completion
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
		sig := <-signals
		serverLogicLogger.Info("Received shutdown signal", "signal", sig.String())

		// Initiate graceful shutdown with timeout
		_, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		serverLogicLogger.Info("Attempting graceful server shutdown", "timeout", shutdownTimeout)
		if stopErr := connServer.Stop(); stopErr != nil { // Renamed err
			// L145 area log: Use slog Error with %+v
			serverLogicLogger.Error("Error stopping server during graceful shutdown", "error", fmt.Sprintf("%+v", stopErr))
		} else {
			serverLogicLogger.Info("Server stopped gracefully.")
		}
		close(stopChan) // Signal that shutdown attempt is complete
	}()
	serverLogicLogger.Debug("Graceful shutdown handler configured")

	// Start the connection server
	serverLogicLogger.Info("Starting CowGnition MCP server...", "transport", transportType, "architecture", "connection_state_machine")
	if err := connServer.Start(); err != nil {
		// Check if it's a normal exit signal or a real error? jsonrpc2 might return nil on disconnect.
		// Assuming any error here is problematic for startup.
		wrappedErr := errors.Wrapf(err, "runServer: connection server failed to start or exited unexpectedly")
		serverLogicLogger.Error("Server start failed", "error", fmt.Sprintf("%+v", wrappedErr))
		return wrappedErr
	}

	// If Start() returns nil (e.g., stdio disconnect), wait for shutdown signal goroutine to finish.
	// This prevents the main function exiting before shutdown completes.
	<-stopChan
	serverLogicLogger.Info("Server run sequence finished.")
	return nil
}

// createAndConfigureServer creates and configures the MCP server. (Keep or remove based on usage)
//
//nolint:unused
func createAndConfigureServer(cfg *config.Settings, transportType string, requestTimeout, shutdownTimeout time.Duration) (*mcp.Server, error) {
	serverLogicLogger.Debug("Creating and configuring server (in unused function)", "transport", transportType)
	// Create server
	server, err := mcp.NewServer(cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "createAndConfigureServer: failed to create server")
	}

	// Set version
	server.SetVersion(Version) // Assuming Version defined elsewhere

	// Set transport type
	if err := server.SetTransport(transportType); err != nil {
		wrappedErr := errors.Wrap(err, "createAndConfigureServer: failed to set transport")
		return nil, cgerr.ErrorWithDetails(
			wrappedErr,
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
	serverLogicLogger.Debug("Server configured (in unused function)")
	return server, nil
}

// getRTMCredentials gets and validates RTM API credentials.
func getRTMCredentials(cfg *config.Settings) (string, string, error) {
	serverLogicLogger.Debug("Getting RTM credentials")
	// Get RTM API credentials from environment or config
	apiKey := os.Getenv("RTM_API_KEY")
	usingEnvKey := apiKey != ""
	if !usingEnvKey {
		apiKey = cfg.RTM.APIKey
	}

	sharedSecret := os.Getenv("RTM_SHARED_SECRET")
	usingEnvSecret := sharedSecret != ""
	if !usingEnvSecret {
		sharedSecret = cfg.RTM.SharedSecret
	}
	serverLogicLogger.Debug("Credential source check", "api_key_from_env", usingEnvKey, "shared_secret_from_env", usingEnvSecret)

	// Ensure API key and shared secret are available
	if apiKey == "" || sharedSecret == "" {
		err := cgerr.ErrorWithDetails(
			errors.New("getRTMCredentials: missing RTM API credentials - you'll need to mooove these into place"), // Already has function context
			cgerr.CategoryConfig,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"has_api_key":       apiKey != "",
				"has_shared_secret": sharedSecret != "",
				"rtm_api_key_env":   os.Getenv("RTM_API_KEY") != "", // Re-check env specifically for log detail
				"rtm_secret_env":    os.Getenv("RTM_SHARED_SECRET") != "",
				"config_has_key":    cfg.RTM.APIKey != "",
				"config_has_secret": cfg.RTM.SharedSecret != "",
				"server_name":       cfg.GetServerName(),
			},
		)
		serverLogicLogger.Error("Missing RTM credentials", "error", fmt.Sprintf("%+v", err)) // Log the detailed error
		return "", "", err
	}

	serverLogicLogger.Debug("RTM credentials retrieved successfully")
	return apiKey, sharedSecret, nil
}

// getTokenPath gets the expanded token path.
func getTokenPath(cfg *config.Settings) (string, error) {
	serverLogicLogger.Debug("Getting token path")
	// Get token path from config or env
	tokenPath := os.Getenv("RTM_TOKEN_PATH")
	usingEnv := tokenPath != ""
	if !usingEnv {
		tokenPath = cfg.Auth.TokenPath
	}
	serverLogicLogger.Debug("Token path source check", "from_env", usingEnv, "raw_path", tokenPath)

	// Expand ~ in token path if present
	expandedPath, err := config.ExpandPath(tokenPath) // Assumes config.ExpandPath logs its own errors/debug info
	if err != nil {
		return "", errors.Wrapf(err, "getTokenPath: failed to expand token path '%s'", tokenPath)
	}
	serverLogicLogger.Debug("Token path expanded", "raw_path", tokenPath, "expanded_path", expandedPath)
	return expandedPath, nil
}

// registerRTMProvider creates and registers the RTM authentication provider.
func registerRTMProvider(server *mcp.Server, apiKey, sharedSecret, tokenPath string) error {
	serverLogicLogger.Debug("Registering RTM provider", "token_path", tokenPath)
	// Create RTM auth provider
	authProvider, err := rtm.NewAuthProvider(apiKey, sharedSecret, tokenPath)
	if err != nil {
		return errors.Wrapf(err, "registerRTMProvider: failed to create RTM auth provider")
	}

	// Register provider with the server
	server.RegisterResourceProvider(authProvider) // Assumes RegisterResourceProvider logs success
	serverLogicLogger.Info("RTM authentication provider registered successfully")
	return nil
}
