// internal/mcp/handlers.go
package mcp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/dkoosis/cowgnition/internal/httputils"
)

// handleInitialize handles the MCP initialize request.
func (s *Server) handleInitialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. Initialize endpoint requires POST.",
			map[string]string{"allowed_method": "POST"})
		return
	}

	// Parse request
	var req InitializeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.WriteErrorResponse(w, httputils.ParseError,
			fmt.Sprintf("Failed to decode initialize request: %v", err),
			map[string]string{"request_url": r.URL.Path})
		return
	}

	// Log initialization request
	log.Printf("MCP initialization requested by: %s (version: %s)",
		req.ServerName, req.ServerVersion)

	// Construct server information
	serverInfo := ServerInfo{
		Name:    s.config.GetServerName(),
		Version: s.version,
	}

	// Define capabilities
	capabilities := map[string]interface{}{
		"resources": map[string]interface{}{
			"list": true,
			"read": true,
		},
		"tools": map[string]interface{}{
			"list": true,
			"call": true,
		},
	}

	// Construct response
	response := InitializeResponse{
		ServerInfo:   serverInfo,
		Capabilities: capabilities,
	}

	httputils.WriteJSONResponse(w, response)
}

// handleListResources handles the MCP list_resources request.
func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. List resources endpoint requires GET.",
			map[string]string{"allowed_method": "GET"})
		return
	}

	// Initially return empty list
	resources := []ResourceDefinition{}

	response := ListResourcesResponse{
		Resources: resources,
	}

	httputils.WriteJSONResponse(w, response)
}

// handleReadResource handles the MCP read_resource request.
func (s *Server) handleReadResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. Read resource endpoint requires GET.",
			map[string]string{"allowed_method": "GET"})
		return
	}

	// Get resource name from query parameters
	name := r.URL.Query().Get("name")
	if name == "" {
		httputils.WriteErrorResponse(w, httputils.InvalidParams,
			"Missing required resource name parameter.",
			map[string]string{"required_parameter": "name"})
		return
	}

	// Initially just return resource not found for any resource
	httputils.WriteErrorResponse(w, httputils.ResourceError,
		fmt.Sprintf("Resource not found: %s", name),
		map[string]interface{}{
			"resource_uri": name,
		})
}

// handleListTools handles the MCP list_tools request.
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. List tools endpoint requires GET.",
			map[string]string{"allowed_method": "GET"})
		return
	}

	// Initially return empty list
	tools := []ToolDefinition{}

	response := ListToolsResponse{
		Tools: tools,
	}

	httputils.WriteJSONResponse(w, response)
}

// handleCallTool handles the MCP call_tool request.
func (s *Server) handleCallTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. Call tool endpoint requires POST.",
			map[string]string{"allowed_method": "POST"})
		return
	}

	// Parse request
	var req CallToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.WriteErrorResponse(w, httputils.ParseError,
			fmt.Sprintf("Failed to decode call_tool request: %v", err),
			map[string]string{"request_path": r.URL.Path})
		return
	}

	// Initially just return tool not found for any tool
	httputils.WriteErrorResponse(w, httputils.ToolError,
		fmt.Sprintf("Tool not found: %s", req.Name),
		map[string]interface{}{
			"tool_name": req.Name,
		})
}
