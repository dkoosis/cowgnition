// file: internal/server/api/handlers.go
// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"net/http"
)

// HandleInitialize provides a public API for the initialize handler.
// This is primarily used for testing purposes.
func (s *Server) HandleInitialize(w http.ResponseWriter, r *http.Request) {
	s.handleInitialize(w, r)
}

// HandleListResources provides a public API for the list_resources handler.
// This is primarily used for testing purposes.
func (s *Server) HandleListResources(w http.ResponseWriter, r *http.Request) {
	s.handleListResources(w, r)
}

// HandleReadResource provides a public API for the read_resource handler.
// This is primarily used for testing purposes.
func (s *Server) HandleReadResource(w http.ResponseWriter, r *http.Request) {
	s.handleReadResource(w, r)
}

// HandleListTools provides a public API for the list_tools handler.
// This is primarily used for testing purposes.
func (s *Server) HandleListTools(w http.ResponseWriter, r *http.Request) {
	s.handleListTools(w, r)
}

// HandleCallTool provides a public API for the call_tool handler.
// This is primarily used for testing purposes.
func (s *Server) HandleCallTool(w http.ResponseWriter, r *http.Request) {
	s.handleCallTool(w, r)
}

// HandleHealthCheck provides a public API for the health check handler.
// This is primarily used for testing purposes.
func (s *Server) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	s.handleHealthCheck(w, r)
}

// HandleSendNotification provides a public API for the notification handler.
// This is primarily used for testing purposes.
func (s *Server) HandleSendNotification(w http.ResponseWriter, r *http.Request) {
	s.handleSendNotification(w, r)
}

// HandleStatusCheck provides a public API for the status check handler.
// This is primarily used for testing purposes.
func (s *Server) HandleStatusCheck(w http.ResponseWriter, r *http.Request) {
	s.handleStatusCheck(w, r)
}
