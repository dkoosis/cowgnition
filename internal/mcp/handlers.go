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
	// Ensure only POST requests are handled.
	if r.Method != http.MethodPost {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. Initialize endpoint requires POST.",
			map[string]string{"allowed_method": "POST"})
		return
	}

	// Create a context with timeout to prevent indefinite waiting.
	ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
	defer cancel()

	// Parse the InitializeRequest from the request body.
	var req InitializeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.WriteErrorResponse(w, httputils.ParseError,
			fmt.Sprintf("Failed to decode initialize request: %v", err),
			map[string]string{"request_url": r.URL.Path})
		return
	}

	// Log the initialization request for auditing and debugging.
	log.Printf("MCP initialization requested by: %s (version: %s)",
		req.ServerName, req.ServerVersion)

	// Construct the server information to be included in the response.
	serverInfo := ServerInfo{
		Name:    s.config.GetServerName(), // Get server name from configuration.
		Version: s.version,                // Get server version.
	}

	// Define the capabilities of this MCP server, indicating supported resources and tools.
	capabilities := map[string]interface{}{
		"resources": map[string]interface{}{
			"list": true, // Server supports listing resources.
			"read": true, // Server supports reading resources.
		},
		"tools": map[string]interface{}{
			"list": true, // Server supports listing tools.
			"call": true, // Server supports calling tools.
		},
	}

	// Construct the InitializeResponse to send back to the client.
	response := InitializeResponse{
		ServerInfo:   serverInfo,
		Capabilities: capabilities,
	}

	// Check if the request timed out before completing.
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			httputils.WriteErrorResponse(w, httputils.InternalError,
				"Request timed out",
				map[string]string{"error": "deadline_exceeded"})
			return
		}
	default:
		// If no timeout, write the JSON response.
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
	// Ensure only GET requests are allowed for listing resources.
	if r.Method != http.MethodGet {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. List resources endpoint requires GET.",
			map[string]string{"allowed_method": "GET"})
		return
	}

	// Create a context with timeout to manage long-running requests.
	ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
	defer cancel()

	// Get all resource definitions from the resource manager.
	resources := s.resourceManager.GetAllResourceDefinitions()

	// Construct the response containing the list of resources.
	response := ListResourcesResponse{
		Resources: resources,
	}

	// Check for timeout before sending the response.
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			httputils.WriteErrorResponse(w, httputils.InternalError,
				"Request timed out",
				map[string]string{"error": "deadline_exceeded"})
			return
		}
	default:
		// Send the JSON response with the list of resources.
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
	// Ensure that only GET requests are allowed for reading a resource.
	if r.Method != http.MethodGet {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. Read resource endpoint requires GET.",
			map[string]string{"allowed_method": "GET"})
		return
	}

	// Create a context with timeout to prevent requests from running indefinitely.
	ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
	defer cancel()

	// Extract the resource name from the query parameters.
	name := r.URL.Query().Get("name")
	if name == "" {
		httputils.WriteErrorResponse(w, httputils.InvalidParams,
			"Missing required resource name parameter.",
			map[string]string{"required_parameter": "name"})
		return
	}

	// Parse any additional query parameters as arguments for reading the resource.
	queryParams := r.URL.Query()
	args := make(map[string]string)
	for k, v := range queryParams {
		// Exclude the 'name' parameter, as it's already handled.
		if k != "name" && len(v) > 0 {
			args[k] = v[0]
		}
	}

	// Prepare variables to store the resource content, MIME type, and any error encountered.
	var content string
	var mimeType string
	var resourceErr error

	// Use a channel to communicate the result of the resource reading operation
	// between the goroutine and the main function, ensuring thread safety.
	resultCh := make(chan struct {
		content  string
		mimeType string
		err      error
	}, 1)

	// Launch a goroutine to read the resource, allowing non-blocking operations and timeout handling.
	go func() {
		c, m, e := s.resourceManager.ReadResource(ctx, name, args)
		resultCh <- struct {
			content  string
			mimeType string
			err      error
		}{c, m, e}
	}()

	// Wait for the result from the resource reading operation or for the context to timeout.
	select {
	case <-ctx.Done():
		// Handle timeout scenario.
		if ctx.Err() == context.DeadlineExceeded {
			httputils.WriteErrorResponse(w, httputils.InternalError,
				"Request timed out",
				map[string]string{"error": "deadline_exceeded"})
			return
		}
	case result := <-resultCh:
		// Retrieve the result from the channel.
		content = result.content
		mimeType = result.mimeType
		resourceErr = result.err
	}

	// Handle any error that occurred while reading the resource.
	if resourceErr != nil {
		code := httputils.InternalError // Default error code.
		// Adjust error code based on the specific error type.
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

	// Construct the response containing the resource content and MIME type.
	response := ResourceResponse{
		Content:  content,
		MimeType: mimeType,
	}

	// Send the JSON response back to the client.
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
	// Ensure only GET requests are allowed for listing tools.
	if r.Method != http.MethodGet {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. List tools endpoint requires GET.",
			map[string]string{"allowed_method": "GET"})
		return
	}

	// Create a context with timeout to prevent indefinite waiting.
	ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
	defer cancel()

	// Get all tool definitions from the tool manager.
	tools := s.toolManager.GetAllToolDefinitions()

	// Construct the response containing the list of tools.
	response := ListToolsResponse{
		Tools: tools,
	}

	// Check for timeout before sending the response.
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			httputils.WriteErrorResponse(w, httputils.InternalError,
				"Request timed out",
				map[string]string{"error": "deadline_exceeded"})
			return
		}
	default:
		// Send the JSON response with the list of tools.
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
	// Ensure only POST requests are allowed for calling a tool.
	if r.Method != http.MethodPost {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. Call tool endpoint requires POST.",
			map[string]string{"allowed_method": "POST"})
		return
	}

	// Create a context with timeout to manage potentially long-running tool calls.
	ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
	defer cancel()

	// Parse the CallToolRequest from the request body.
	var req CallToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.WriteErrorResponse(w, httputils.ParseError,
			fmt.Sprintf("Failed to decode call_tool request: %v", err),
			map[string]string{"request_path": r.URL.Path})
		return
	}

	// Prepare variables to store the tool's result and any error encountered during the call.
	var result string
	var toolErr error

	// Use a channel to communicate the result of the tool call operation
	// between the goroutine and the main function, ensuring thread safety.
	resultCh := make(chan struct {
		result string
		err    error
	}, 1)

	// Launch a goroutine to call the tool, allowing non-blocking operations and timeout handling.
	go func() {
		r, e := s.toolManager.CallTool(ctx, req.Name, req.Arguments)
		resultCh <- struct {
			result string
			err    error
		}{r, e}
	}()

	// Wait for the result from the tool call operation or for the context to timeout.
	select {
	case <-ctx.Done():
		// Handle timeout scenario.
		if ctx.Err() == context.DeadlineExceeded {
			httputils.WriteErrorResponse(w, httputils.InternalError,
				"Request timed out",
				map[string]string{"error": "deadline_exceeded"})
			return
		}
	case toolResult := <-resultCh:
		// Retrieve the result from the channel.
		result = toolResult.result
		toolErr = toolResult.err
	}

	// Handle any error that occurred during the tool call.
	if toolErr != nil {
		code := httputils.InternalError // Default error code.
		// Adjust error code based on the specific error type.
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

	// Construct the response containing the tool's result.
	response := ToolResponse{
		Result: result,
	}

	// Send the JSON response back to the client.
	httputils.WriteJSONResponse(w, response)
}
