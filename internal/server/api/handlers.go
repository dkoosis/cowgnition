// Package api provides public API endpoints for the MCP server.
// file: internal/server/api/handlers.go
package api

import (
	"net/http"

	"github.com/dkoosis/cowgnition/internal/server"
)

// HandleInitialize provides a public API for the initialize handler.
// This function is part of the MCP server's API and handles the initialize endpoint.
// It's primarily used for testing purposes to allow external systems to trigger the server's initialization process.
func (s *server.Server) HandleInitialize(w http.ResponseWriter, r *http.Request) {
	s.handleInitialize(w, r)
}

// HandleListResources provides a public API for the list_resources handler.
// This function is part of the MCP server's API and handles the list_resources endpoint.
// It's primarily used for testing purposes, enabling external systems to query the server's available resources.
func (s *server.Server) HandleListResources(w http.ResponseWriter, r *http.Request) {
	s.handleListResources(w, r)
}

// HandleReadResource provides a public API for the read_resource handler.
// This function is part of the MCP server's API and handles the read_resource endpoint.
// It's primarily used for testing purposes, allowing external systems to read specific resources from the server.
func (s *server.Server) HandleReadResource(w http.ResponseWriter, r *http.Request) {
	s.handleReadResource(w, r)
}

// HandleListTools provides a public API for the list_tools handler.
// This function is part of the MCP server's API and handles the list_tools endpoint.
// It's primarily used for testing purposes, providing external systems with a list of tools offered by the server.
func (s *server.Server) HandleListTools(w http.ResponseWriter, r *http.Request) {
	s.handleListTools(w, r)
}

// HandleCallTool provides a public API for the call_tool handler.
// This function is part of the MCP server's API and handles the call_tool endpoint.
// It's primarily used for testing purposes, enabling external systems to execute specific tools provided by the server.
func (s *server.Server) HandleCallTool(w http.ResponseWriter, r *http.Request) {
	s.handleCallTool(w, r)
}

// HandleHealthCheck provides a public API for the health check handler.
// This function is part of the MCP server's API and handles the health check endpoint.
// It's primarily used for testing purposes, allowing external systems to verify the server's health status.
func (s *server.Server) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	s.handleHealthCheck(w, r)
}

// HandleSendNotification provides a public API for the notification handler.
// This function is part of the MCP server's API and handles the notification endpoint.
// It's primarily used for testing purposes, enabling external systems to send notifications through the server.
func (s *server.Server) HandleSendNotification(w http.ResponseWriter, r *http.Request) {
	s.handleSendNotification(w, r)
}

// HandleStatusCheck provides a public API for the status check handler.
// This function is part of the MCP server's API and handles the status check endpoint.
// It's primarily used for testing purposes, allowing external systems to query the server's status.
func (s *server.Server) HandleStatusCheck(w http.ResponseWriter, r *http.Request) {
	s.handleStatusCheck(w, r)
}

// DocEnhanced: 2025-03-25
