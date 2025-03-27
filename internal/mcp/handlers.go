// Package mcp handles the Model Context Protocol (MCP) server functionality.
// file: internal/mcp/handlers.go
package mcp

import (
	"context"
	"encoding/json"
	"errors" // Added for errors.Is
	"fmt"
	"log"
	"net/http"

	"github.com/dkoosis/cowgnition/internal/httputils"
)

// handleInitialize handles the MCP initialize request.
// This function processes the initialization request from an MCP client,
// providing essential server information and capabilities.
// It ensures that only POST requests are allowed for this endpoint
// and returns an error if any other method is used.
// The request body is expected to contain an InitializeRequest,
// which is parsed to extract the client's server name and version.
// The server then responds with an InitializeResponse,
// including its own server information and a list of supported capabilities.
//
// w http.ResponseWriter: The http.ResponseWriter used to construct the HTTP response.
// r *http.Request: The http.Request representing the client's request.
func (s *Server) handleInitialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. Initialize endpoint requires POST.",
			map[string]string{"allowed_method": "POST"})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
	defer cancel()

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

	// Check for timeout
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			httputils.WriteErrorResponse(w, httputils.InternalError,
				"Request timed out",
				map[string]string{"error": "deadline_exceeded"})
			return
		}
	default:
		httputils.WriteJSONResponse(w, response)
	}
}

// handleListResources handles the MCP list_resources request.
// This function processes the request to list all available resources.
// It only allows GET requests and retrieves resource definitions
// from the resource manager.
// The server responds with a ListResourcesResponse containing the list of resources.
//
// w http.ResponseWriter: The http.ResponseWriter used to construct the HTTP response.
// r *http.Request: The http.Request representing the client's request.
func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. List resources endpoint requires GET.",
			map[string]string{"allowed_method": "GET"})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
	defer cancel()

	// Get resources from all registered providers
	resources := s.resourceManager.GetAllResourceDefinitions()

	response := ListResourcesResponse{
		Resources: resources,
	}

	// Check for timeout
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			httputils.WriteErrorResponse(w, httputils.InternalError,
				"Request timed out",
				map[string]string{"error": "deadline_exceeded"})
			return
		}
	default:
		httputils.WriteJSONResponse(w, response)
	}
}

// handleReadResource handles the MCP read_resource request.
// This function processes the request to read a specific resource.
// It only allows GET requests and requires a 'name' parameter to identify the resource.
// Additional query parameters are treated as arguments for reading the resource.
// If the resource is found, the server responds with its content and MIME type;
// otherwise, it returns an appropriate error.
//
// w http.ResponseWriter: The http.ResponseWriter used to construct the HTTP response.
// r *http.Request: The http.Request representing the client's request.
func (s *Server) handleReadResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. Read resource endpoint requires GET.",
			map[string]string{"allowed_method": "GET"})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
	defer cancel()

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
	var content string
	var mimeType string
	var resourceErr error

	// Use a channel to communicate the result of the resource reading operation
	resultCh := make(chan struct {
		content  string
		mimeType string
		err      error
	}, 1)

	go func() {
		c, m, e := s.resourceManager.ReadResource(ctx, name, args)
		resultCh <- struct {
			content  string
			mimeType string
			err      error
		}{c, m, e}
	}()

	// Wait for result or timeout
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			httputils.WriteErrorResponse(w, httputils.InternalError,
				"Request timed out",
				map[string]string{"error": "deadline_exceeded"})
			return
		}
	case result := <-resultCh:
		content = result.content
		mimeType = result.mimeType
		resourceErr = result.err
	}

	if resourceErr != nil {
		code := httputils.InternalError
		if errors.Is(resourceErr, ErrResourceNotFound) {
			code = httputils.ResourceError
		} else if errors.Is(resourceErr, ErrInvalidArguments) {
			code = httputils.InvalidParams
		}

		httputils.WriteErrorResponse(w, code,
			fmt.Sprintf("Failed to read resource: %v", resourceErr),
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
// This function processes the request to list all available tools.
// It only allows GET requests and retrieves tool definitions
// from the tool manager.
// The server responds with a ListToolsResponse containing the list of tools.
//
// w http.ResponseWriter: The http.ResponseWriter used to construct the HTTP response.
// r *http.Request: The http.Request representing the client's request.
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. List tools endpoint requires GET.",
			map[string]string{"allowed_method": "GET"})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
	defer cancel()

	// Get tools from all registered providers
	tools := s.toolManager.GetAllToolDefinitions()

	response := ListToolsResponse{
		Tools: tools,
	}

	// Check for timeout
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			httputils.WriteErrorResponse(w, httputils.InternalError,
				"Request timed out",
				map[string]string{"error": "deadline_exceeded"})
			return
		}
	default:
		httputils.WriteJSONResponse(w, response)
	}
}

// handleCallTool handles the MCP call_tool request.
// This function processes the request to call a specific tool.
// It only allows POST requests and expects a CallToolRequest in the request body,
// which includes the tool's name and arguments.
// The server calls the specified tool with the provided arguments
// and responds with the tool's result.
// If the tool call fails, it returns an appropriate error.
//
// w http.ResponseWriter: The http.ResponseWriter used to construct the HTTP response.
// r *http.Request: The http.Request representing the client's request.
func (s *Server) handleCallTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. Call tool endpoint requires POST.",
			map[string]string{"allowed_method": "POST"})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
	defer cancel()

	// Parse request
	var req CallToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.WriteErrorResponse(w, httputils.ParseError,
			fmt.Sprintf("Failed to decode call_tool request: %v", err),
			map[string]string{"request_path": r.URL.Path})
		return
	}

	// Call the tool
	var result string
	var toolErr error

	// Use a channel to communicate the result of the tool call operation
	resultCh := make(chan struct {
		result string
		err    error
	}, 1)

	go func() {
		r, e := s.toolManager.CallTool(ctx, req.Name, req.Arguments)
		resultCh <- struct {
			result string
			err    error
		}{r, e}
	}()

	// Wait for result or timeout
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			httputils.WriteErrorResponse(w, httputils.InternalError,
				"Request timed out",
				map[string]string{"error": "deadline_exceeded"})
			return
		}
	case toolResult := <-resultCh:
		result = toolResult.result
		toolErr = toolResult.err
	}

	if toolErr != nil {
		code := httputils.InternalError
		if errors.Is(toolErr, ErrToolNotFound) {
			code = httputils.ToolError
		} else if errors.Is(toolErr, ErrInvalidArguments) {
			code = httputils.InvalidParams
		}

		httputils.WriteErrorResponse(w, code,
			fmt.Sprintf("Failed to call tool: %v", toolErr),
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
