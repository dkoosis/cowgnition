// internal/mcp/server.go
package mcp

import (
	"log"
	"net/http"
	"time"

	"github.com/dkoosis/cowgnition/internal/config"
)

// Server represents an MCP server.
type Server struct {
	config     *config.Config
	httpServer *http.Server
	version    string
	startTime  time.Time
}

// NewServer creates a new MCP server.
func NewServer(cfg *config.Config) (*Server, error) {
	return &Server{
		config:    cfg,
		version:   "1.0.0", // Default version
		startTime: time.Now(),
	}, nil
}

// Start starts the MCP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Register MCP protocol handlers
	mux.HandleFunc("/mcp/initialize", s.handleInitialize)
	mux.HandleFunc("/mcp/list_resources", s.handleListResources)
	mux.HandleFunc("/mcp/read_resource", s.handleReadResource)
	mux.HandleFunc("/mcp/list_tools", s.handleListTools)
	mux.HandleFunc("/mcp/call_tool", s.handleCallTool)

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         s.config.GetServerAddress(),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server
	log.Printf("Starting MCP server on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Stop stops the MCP server.
func (s *Server) Stop() error {
	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

// SetVersion sets the server version.
func (s *Server) SetVersion(version string) {
	s.version = version
}

// GetUptime returns the server's uptime.
func (s *Server) GetUptime() time.Duration {
	return time.Since(s.startTime)
}
