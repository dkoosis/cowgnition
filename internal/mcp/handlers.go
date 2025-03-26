// Package mcp handles the Model Context Protocol (MCP) server functionality.
// file: internal/mcp/handlers.go
package mcp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/dkoosis/cowgnition/internal/httputils"
)

// handleInitialize handles the MCP initialize request.
// This function processes the initialization request from an MCP client,
// providing essential server information and capabilities.
// It ensures that only POST requests are allowed for this endpoint
// and returns an error if any other method is used[cite: 1048, 1060, 1061, 1069].
// The request body is expected to contain an InitializeRequest,
// which is parsed to extract the client's server name and version[cite: 1018, 1019, 1020].
// The server then responds with an InitializeResponse,
// including its own server information and a list of supported capabilities[cite: 1015, 1016, 1017].
//
// w http.ResponseWriter: The http.ResponseWriter used to construct the HTTP response.
// r *http.Request: The http.Request representing the client's request.
func (s *Server) handleInitialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. Initialize endpoint requires POST.",
			map[string]string{"allowed_method": "POST"}) // Responds with an error if the method is not POST, indicating the allowed method.
		return
	}

	// Parse request
	var req InitializeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.WriteErrorResponse(w, httputils.ParseError,
			fmt.Sprintf("Failed to decode initialize request: %v", err),
			map[string]string{"request_url": r.URL.Path}) // Responds with a parse error if the request body cannot be decoded into an InitializeRequest.
		return
	}

	// Log initialization request
	log.Printf("MCP initialization requested by: %s (version: %s)",
		req.ServerName, req.ServerVersion) // Logs the initialization request with the client's server name and version.

	// Construct server information
	serverInfo := ServerInfo{
		Name:    s.config.GetServerName(), // Retrieves the server name from the configuration.
		Version: s.version,                // Retrieves the server version.
	}

	// Define capabilities
	capabilities := map[string]interface{}{ // Defines the server's capabilities regarding resources and tools.
		"resources": map[string]interface{}{
			"list": true, // Indicates the server can list resources.
			"read": true, // Indicates the server can read resources.
		},
		"tools": map[string]interface{}{
			"list": true, // Indicates the server can list tools.
			"call": true, // Indicates the server can call tools.
		},
	}

	// Construct response
	response := InitializeResponse{ // Constructs the response containing server information and capabilities.
		ServerInfo:   serverInfo,
		Capabilities: capabilities,
	}

	httputils.WriteJSONResponse(w, response) // Sends the JSON response to the client.
}

// handleListResources handles the MCP list_resources request.
// This function processes the request to list all available resources.
// It only allows GET requests and retrieves resource definitions
// from the resource manager[cite: 1069, 1070].
// The server responds with a ListResourcesResponse containing the list of resources[cite: 1015, 1016, 1017].
//
// w http.ResponseWriter: The http.ResponseWriter used to construct the HTTP response.
// r *http.Request: The http.Request representing the client's request.
func (s *Server) handleListResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. List resources endpoint requires GET.",
			map[string]string{"allowed_method": "GET"}) // Responds with an error if the method is not GET, indicating the allowed method.
		return
	}

	// Get resources from all registered providers
	resources := s.resourceManager.GetAllResourceDefinitions() // Retrieves all resource definitions.

	response := ListResourcesResponse{ // Constructs the response containing the list of resources.
		Resources: resources,
	}

	httputils.WriteJSONResponse(w, response) // Sends the JSON response to the client.
}

// handleReadResource handles the MCP read_resource request.
// This function processes the request to read a specific resource.
// It only allows GET requests and requires a 'name' parameter to identify the resource[cite: 1069, 1070].
// Additional query parameters are treated as arguments for reading the resource.
// If the resource is found, the server responds with its content and MIME type;
// otherwise, it returns an appropriate error[cite: 1018, 1019, 1020].
//
// w http.ResponseWriter: The http.ResponseWriter used to construct the HTTP response.
// r *http.Request: The http.Request representing the client's request.
func (s *Server) handleReadResource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. Read resource endpoint requires GET.",
			map[string]string{"allowed_method": "GET"}) // Responds with an error if the method is not GET, indicating the allowed method.
		return
	}

	// Get resource name from query parameters
	name := r.URL.Query().Get("name") // Retrieves the 'name' parameter from the query.
	if name == "" {
		httputils.WriteErrorResponse(w, httputils.InvalidParams,
			"Missing required resource name parameter.",
			map[string]string{"required_parameter": "name"}) // Responds with an error if the 'name' parameter is missing.
		return
	}

	// Parse additional arguments
	queryParams := r.URL.Query()
	args := make(map[string]string) // Collects additional query parameters as arguments.
	for k, v := range queryParams {
		if k != "name" && len(v) > 0 {
			args[k] = v[0]
		}
	}

	// Read the resource
	content, mimeType, err := s.resourceManager.ReadResource(r.Context(), name, args) // Reads the resource content.
	if err != nil {
		code := httputils.InternalError // Default error code.
		if err == ErrResourceNotFound {
			code = httputils.ResourceError // Specific error code for resource not found.
		} else if err == ErrInvalidArguments {
			code = httputils.InvalidParams // Specific error code for invalid arguments.
		}

		httputils.WriteErrorResponse(w, code,
			fmt.Sprintf("Failed to read resource: %v", err),
			map[string]interface{}{
				"resource_name": name, // Includes resource name in the error response.
				"args":          args, // Includes arguments in the error response.
			})
		return
	}

	// Return the resource content
	response := ResourceResponse{ // Constructs the response containing the resource content and MIME type.
		Content:  content,
		MimeType: mimeType,
	}

	httputils.WriteJSONResponse(w, response) // Sends the JSON response to the client.
}

// handleListTools handles the MCP list_tools request.
// This function processes the request to list all available tools.
// It only allows GET requests and retrieves tool definitions
// from the tool manager[cite: 1069, 1070].
// The server responds with a ListToolsResponse containing the list of tools[cite: 1015, 1016, 1017].
//
// w http.ResponseWriter: The http.ResponseWriter used to construct the HTTP response.
// r *http.Request: The http.Request representing the client's request.
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. List tools endpoint requires GET.",
			map[string]string{"allowed_method": "GET"}) // Responds with an error if the method is not GET, indicating the allowed method.
		return
	}

	// Get tools from all registered providers
	tools := s.toolManager.GetAllToolDefinitions() // Retrieves all tool definitions.

	response := ListToolsResponse{ // Constructs the response containing the list of tools.
		Tools: tools,
	}

	httputils.WriteJSONResponse(w, response) // Sends the JSON response to the client.
}

// handleCallTool handles the MCP call_tool request.
// This function processes the request to call a specific tool.
// It only allows POST requests and expects a CallToolRequest in the request body,
// which includes the tool's name and arguments[cite: 1048, 1060, 1061, 1069].
// The server calls the specified tool with the provided arguments
// and responds with the tool's result.
// If the tool call fails, it returns an appropriate error[cite: 1018, 1019, 1020].
//
// w http.ResponseWriter: The http.ResponseWriter used to construct the HTTP response.
// r *http.Request: The http.Request representing the client's request.
func (s *Server) handleCallTool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputils.WriteErrorResponse(w, httputils.MethodNotFound,
			"Method not allowed. Call tool endpoint requires POST.",
			map[string]string{"allowed_method": "POST"}) // Responds with an error if the method is not POST, indicating the allowed method.
		return
	}

	// Parse request
	var req CallToolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputils.WriteErrorResponse(w, httputils.ParseError,
			fmt.Sprintf("Failed to decode call_tool request: %v", err),
			map[string]string{"request_path": r.URL.Path}) // Responds with a parse error if the request body cannot be decoded into a CallToolRequest.
		return
	}

	// Call the tool
	result, err := s.toolManager.CallTool(r.Context(), req.Name, req.Arguments) // Calls the specified tool with the provided arguments.
	if err != nil {
		code := httputils.InternalError // Default error code.
		if err == ErrToolNotFound {
			code = httputils.ToolError // Specific error code for tool not found.
		} else if err == ErrInvalidArguments {
			code = httputils.InvalidParams // Specific error code for invalid arguments.
		}

		httputils.WriteErrorResponse(w, code,
			fmt.Sprintf("Failed to call tool: %v", err),
			map[string]interface{}{
				"tool_name": req.Name,      // Includes tool name in the error response.
				"args":      req.Arguments, // Includes arguments in the error response.
			})
		return
	}

	// Return the tool result
	response := ToolResponse{ // Constructs the response containing the tool's result.
		Result: result,
	}

	httputils.WriteJSONResponse(w, response) // Sends the JSON response to the client.
}

// DocEnhanced: 2025-03-26
