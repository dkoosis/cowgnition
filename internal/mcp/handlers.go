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

	// Get resources from all registered providers
	resources := s.resourceManager.GetAllResourceDefinitions()

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

	// Parse additional arguments
	queryParams := r.URL.Query()
	args := make(map[string]string)
	for k, v := range queryParams {
		if k != "name" && len(v) > 0 {
			args[k] = v[0]
		}
	}

	// Read the resource
	content, mimeType, err := s.resourceManager.ReadResource(r.Context(), name, args)
	if err != nil {
		code := httputils.InternalError
		if err == ErrResourceNotFound {
			code = httputils.ResourceError
		} else if err == ErrInvalidArguments {
			code = httputils.InvalidParams
		}

		httputils.WriteErrorResponse(w, code,
			fmt.Sprintf("Failed to read resource: %v", err),
			map[string]interface{}{
				"resource_name": name,
				"args":          args,
			})
		return
	}

	// Return the resource content
	response := ResourceResponse{
		Content:  content,
		MimeType: mimeType,
	}

	httputils.WriteJSONResponse(w, response)
}

// handleListTools handles the MCP list_tools request.
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. List tools endpoint requires GET.",
			map[string]string{"allowed_method": "GET"})
		return
	}

	// Get tools from all registered providers
	tools := s.toolManager.GetAllToolDefinitions()

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

	// Call the tool
	result, err := s.toolManager.CallTool(r.Context(), req.Name, req.Arguments)
	if err != nil {
		code := httputils.InternalError
		if err == ErrToolNotFound {
			code = httputils.ToolError
		} else if err == ErrInvalidArguments {
			code = httputils.InvalidParams
		}

		httputils.WriteErrorResponse(w, code,
			fmt.Sprintf("Failed to call tool: %v", err),
			map[string]interface{}{
				"tool_name": req.Name,
				"args":      req.Arguments,
			})
		return
	}

	// Return the tool result
	response := ToolResponse{
		Result: result,
	}

	httputils.WriteJSONResponse(w, response)
}
