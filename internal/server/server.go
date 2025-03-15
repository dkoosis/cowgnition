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

	// Add middleware
	handler := logMiddleware(recoveryMiddleware(mux))

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Server.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server
	log.Printf("Starting MCP server on port %d", s.config.Server.Port)
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
