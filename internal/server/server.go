// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/dkoosis/cowgnition/internal/auth"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/rtm"
	"github.com/dkoosis/cowgnition/internal/server/middleware"
)

// Server represents an MCP server for RTM integration.
type Server struct {
	config       *config.Config
	rtmService   *rtm.Service
	httpServer   *http.Server
	tokenManager *auth.TokenManager
	// Add version information
	version string
	// Add startup time for uptime tracking
	startTime time.Time
	// Add server instance ID for debugging
	instanceID string
}

// NewServer creates a new MCP server with the provided configuration.
// It initializes the RTM service and authentication token manager.
func NewServer(cfg *config.Config) (*Server, error) {
	// Create token manager
	tokenManager, err := auth.NewTokenManager(cfg.Auth.TokenPath)
	if err != nil {
		// SUGGESTION (Readability): Improved error message for creating token manager.
		return nil, fmt.Errorf("NewServer: error creating token manager at path %s: %w", cfg.Auth.TokenPath, err)
	}

	// Create RTM service
	rtmService := rtm.NewService(
		cfg.RTM.APIKey,
		cfg.RTM.SharedSecret,
		cfg.Auth.TokenPath,
	)

	// Initialize RTM service
	if err := rtmService.Initialize(); err != nil {
		// SUGGESTION (Readability): Improved error message for initializing RTM service.
		return nil, fmt.Errorf("NewServer: error initializing RTM service: %w", err)
	}

	// Generate unique instance ID
	instanceID := fmt.Sprintf("%s-%d", cfg.Server.Name, time.Now().UnixNano())

	return &Server{
		config:       cfg,
		rtmService:   rtmService,
		tokenManager: tokenManager,
		version:      "2.0.0", // This should be injected from build information
		startTime:    time.Now(),
		instanceID:   instanceID,
	}, nil
}

// Start starts the MCP server and returns an error if it fails to start.
func (s *Server) Start() error {
	// Create router
	mux := http.NewServeMux()

	// Register MCP protocol handlers
	mux.HandleFunc("/mcp/initialize", s.handleInitialize)
	mux.HandleFunc("/mcp/list_resources", s.handleListResources)
	mux.HandleFunc("/mcp/read_resource", s.handleReadResource)
	mux.HandleFunc("/mcp/list_tools", s.handleListTools)
	mux.HandleFunc("/mcp/call_tool", s.handleCallTool)

	// Add notification endpoint for upcoming MCP spec compliance
	mux.HandleFunc("/mcp/send_notification", s.handleSendNotification)

	// Add health check endpoint
	mux.HandleFunc("/health", s.handleHealthCheck)

	// Add status endpoint for monitoring
	mux.HandleFunc("/status", s.handleStatusCheck)

	// Add middleware
	handler := middleware.LogMiddleware(middleware.RecoveryMiddleware(middleware.CorsMiddleware(mux)))

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Server.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server
	log.Printf("Starting MCP server '%s' version %s on port %d",
		s.config.Server.Name, s.version, s.config.Server.Port)
	if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		// SUGGESTION (Readability): Improved HTTP server error message.
		return fmt.Errorf("Start: HTTP server ListenAndServe error: %w", err)
	}

	return nil
}

// Stop gracefully stops the MCP server with the given context timeout.
func (s *Server) Stop(ctx context.Context) error {
	log.Println("Shutting down MCP server...")
	return s.httpServer.Shutdown(ctx)
}

// GetUptime returns the server's uptime duration.
func (s *Server) GetUptime() time.Duration {
	return time.Since(s.startTime)
}

// GetVersion returns the server version.
func (s *Server) GetVersion() string {
	return s.version
}

// SetVersion sets the server version.
func (s *Server) SetVersion(version string) {
	s.version = version
}

// GetRTMService returns the server's RTM service.
func (s *Server) GetRTMService() *rtm.Service {
	return s.rtmService
}
