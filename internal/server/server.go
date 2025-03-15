// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/cowgnition/cowgnition/internal/auth"
	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/rtm"
)

// MCPServer represents an MCP server for RTM integration.
type MCPServer struct {
	config       *config.Config
	rtmService   *rtm.Service
	httpServer   *http.Server
	tokenManager *auth.TokenManager
	// Add version information
	version      string
}

// NewMCPServer creates a new MCP server with the provided configuration.
// It initializes the RTM service and authentication token manager.
func NewMCPServer(cfg *config.Config) (*MCPServer, error) {
	// Create token manager
	tokenManager, err := auth.NewTokenManager(cfg.Auth.TokenPath)
	if err != nil {
		return nil, fmt.Errorf("error creating token manager: %w", err)
	}

	// Create RTM service
	rtmService := rtm.NewService(
		cfg.RTM.APIKey,
		cfg.RTM.SharedSecret,
		cfg.Auth.TokenPath,
	)

	// Initialize RTM service
	if err := rtmService.Initialize(); err != nil {
		return nil, fmt.Errorf("error initializing RTM service: %w", err)
	}

	return &MCPServer{
		config:       cfg,
		rtmService:   rtmService,
		tokenManager: tokenManager,
		version:      "1.0.0", // This should be injected from build information
	}, nil
}

// Start starts the MCP server and returns an error if it fails to start.
func (s *MCPServer) Start() error {
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

	// Add middleware
	handler := logMiddleware(recoveryMiddleware(corsMiddleware(mux)))

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
		return fmt.Errorf("HTTP server error: %w", err)
	}

	return nil
}

// Stop gracefully stops the MCP server with the given context timeout.
func (s *MCPServer) Stop(ctx context.Context) error {
	log.Println("Shutting down MCP server...")
	return s.httpServer.Shutdown(ctx)
}

// handleHealthCheck provides a simple health check endpoint.
func (s *MCPServer) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	// Check if RTM service is healthy
	if s.rtmService == nil {
		http.Error(w, "RTM service not initialized", http.StatusServiceUnavailable)
		return
	}

	// Return simple health check response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy"}`))
}

// handleSendNotification is a placeholder for notification support.
// The MCP spec may evolve to include proper notification support.
func (s *MCPServer) handleSendNotification(w http.ResponseWriter, r *http.Request) {
	// Currently, we don't support notifications, so return appropriate error
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Return unsupported operation for now
	writeErrorResponse(w, http.StatusNotImplemented, "Notifications not yet supported")
}

// GetVersion returns the server version.
func (s *MCPServer) GetVersion() string {
	return s.version
}

// SetVersion sets the server version.
func (s *MCPServer) SetVersion(version string) {
	s.version = version
}

// corsMiddleware adds CORS headers for development scenarios.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add CORS headers for development
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}