// ==START OF FILE SECTION server.go PART 1/1==
// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
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
	version string
	// Add startup time for uptime tracking
	startTime time.Time
	// Add server instance ID for debugging
	instanceID string
}

// NewServer creates a new MCP server with the provided configuration.
// It initializes the RTM service and authentication token manager.
func NewServer(cfg *config.Config) (*MCPServer, error) {
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

	return &MCPServer{
		config:       cfg,
		rtmService:   rtmService,
		tokenManager: tokenManager,
		version:      "2.0.0", // This should be injected from build information
		startTime:    time.Now(),
		instanceID:   instanceID,
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

	// Add status endpoint for monitoring
	mux.HandleFunc("/status", s.handleStatusCheck)

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
		// SUGGESTION (Readability): Improved HTTP server error message.
		return fmt.Errorf("Start: HTTP server ListenAndServe error: %w", err)
	}

	return nil
}

// Stop gracefully stops the MCP server with the given context timeout.
func (s *MCPServer) Stop(ctx context.Context) error {
	log.Println("Shutting down MCP server...")
	return s.httpServer.Shutdown(ctx)
}

// GetUptime returns the server's uptime duration.
func (s *MCPServer) GetUptime() time.Duration {
	return time.Since(s.startTime)
}

// handleHealthCheck provides a simple health check endpoint.
func (s *MCPServer) handleHealthCheck(w http.ResponseWriter, _ *http.Request) {
	// Check if RTM service is healthy
	if s.rtmService == nil {
		// SUGGESTION (Readability): Improved health check error message.
		http.Error(w, "handleHealthCheck: RTM service not initialized", http.StatusServiceUnavailable)
		return
	}

	// Return simple health check response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// Check the Write error to satisfy linter
	if _, err := w.Write(byte(`{"status":"healthy"}`)); err != nil {
		log.Printf("Error writing health check response: %v", err)
	}
}

// handleStatusCheck provides detailed status information for monitoring.
func (s *MCPServer) handleStatusCheck(w http.ResponseWriter, r *http.Request) {
	// Only allow access from localhost or if a special header is present
	clientIP := r.RemoteAddr
	if !strings.HasPrefix(clientIP, "127.0.0.1") && !strings.HasPrefix(clientIP, "[::1]") &&
		r.Header.Get("X-Status-Secret") != s.config.Server.StatusSecret {
		// SUGGESTION (Readability): Improved status check error message.
		http.Error(w, "handleStatusCheck: Forbidden", http.StatusForbidden)
		return
	}

	// Gather status information
	status := map[string]interface{}{
		"server": map[string]interface{}{
			"name":        s.config.Server.Name,
			"version":     s.version,
			"uptime":      s.GetUptime().String(),
			"started_at":  s.startTime.Format(time.RFC3339),
			"instance_id": s.instanceID,
		},
		"auth": map[string]interface{}{
			"status":        s.rtmService.GetAuthStatus(),
			"authenticated": s.rtmService.IsAuthenticated(),
			"pending_flows": s.rtmService.GetActiveAuthFlows(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Printf("Error encoding status response: %v", err)
		// SUGGESTION (Readability): Improved encoding error message.
		http.Error(w, "handleStatusCheck: Error encoding JSON response", http.StatusInternalServerError)
	}
}

// handleSendNotification is a placeholder for notification support.
// The MCP spec may evolve to include proper notification support.
func (s *MCPServer) handleSendNotification(w http.ResponseWriter, r *http.Request) {
	// Currently, we don't support notifications, so return appropriate error
	if r.Method != http.MethodPost {
		// SUGGESTION (Readability): Clarified method not allowed message.
		writeErrorResponse(w, http.StatusMethodNotAllowed, "handleSendNotification: Method not allowed. Must use POST for this endpoint.")
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

// GetRTMService returns the server's RTM service.
func (s *MCPServer) GetRTMService() *rtm.Service {
	return s.rtmService
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

// ErrorMsgEnhanced:2024-03-18
