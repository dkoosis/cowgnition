// Package server implements the Model Context Protocol server for RTM integration.
// This file handles HTTP endpoint routing and defers to specialized handlers.
package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// handleInitialize routes the MCP initialize request to specialized handler.
func (s *Server) handleInitialize(w http.ResponseWriter, r *http.Request) {
	s.handleMCPInitialize(w, r)
}

// handleListResources routes the MCP list_resources request to specialized handler.
func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	s.handleMCPListResources(w, r)
}

// handleReadResource routes the MCP read_resource request to specialized handler.
func (s *Server) handleReadResource(w http.ResponseWriter, r *http.Request) {
	s.handleMCPReadResource(w, r)
}

// handleListTools routes the MCP list_tools request to specialized handler.
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	s.handleMCPListTools(w, r)
}

// handleCallTool routes the MCP call_tool request to specialized handler.
func (s *Server) handleCallTool(w http.ResponseWriter, r *http.Request) {
	s.handleMCPCallTool(w, r)
}

// handleSendNotification routes the MCP send_notification request to specialized handler.
func (s *Server) handleSendNotification(w http.ResponseWriter, r *http.Request) {
	s.handleMCPSendNotification(w, r)
}

// handleHealthCheck provides a simple health check endpoint.
func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	// Check if RTM service is healthy
	if s.rtmService == nil {
		writeStandardErrorResponse(w, InternalError,
			"RTM service not initialized",
			map[string]string{"component": "rtm_service"})
		return
	}

	// Return simple health check response
	writeJSONResponse(w, http.StatusOK, map[string]string{
		"status": "healthy",
		"server": s.config.Server.Name,
	})
}

// handleStatusCheck provides detailed status information for monitoring.
func (s *Server) handleStatusCheck(w http.ResponseWriter, r *http.Request) {
	// Only allow access from localhost or if a special header is present
	clientIP := r.RemoteAddr
	if !strings.HasPrefix(clientIP, "127.0.0.1") && !strings.HasPrefix(clientIP, "[::1]") &&
		r.Header.Get("X-Status-Secret") != s.config.Server.StatusSecret {
		writeStandardErrorResponse(w, AuthError,
			"Forbidden: Status endpoint requires authentication",
			map[string]string{
				"required_header": "X-Status-Secret",
				"remote_addr":     r.RemoteAddr,
			})
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

	writeJSONResponse(w, http.StatusOK, status)
}

// dispatchToolRequest routes the tool request to the appropriate handler.
// This is extracted from handleCallTool to reduce complexity.
func (s *Server) dispatchToolRequest(toolName string, args map[string]interface{}) (string, error) {
	switch toolName {
	case "add_task":
		return s.handleAddTaskTool(args)
	case "complete_task":
		return s.handleCompleteTaskTool(args)
	case "uncomplete_task":
		return s.handleUncompleteTaskTool(args)
	case "delete_task":
		return s.handleDeleteTaskTool(args)
	case "set_due_date":
		return s.handleSetDueDateTool(args)
	case "set_priority":
		return s.handleSetPriorityTool(args)
	case "add_tags":
		return s.handleAddTagsTool(args)
	case "logout":
		return s.handleLogoutTool(args)
	case "auth_status":
		return s.handleAuthStatusTool(args)
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}
