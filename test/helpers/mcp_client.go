// Package helpers provides testing utilities for the CowGnition MCP server.
package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/internal/server"
)

// MCPClient is a test client for the MCP server.
type MCPClient struct {
	Server   *httptest.Server
	BaseURL  string
	Client   *http.Client
	ServerID string
}

// NewMCPClient creates a new MCP test client with the provided server.
func NewMCPClient(_ *testing.T, s *server.MCPServer) *MCPClient {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route requests to the MCP server based on path
		switch r.URL.Path {
		case "/mcp/initialize":
			s.HandleInitialize(w, r)
		case "/mcp/list_resources":
			s.HandleListResources(w, r)
		case "/mcp/read_resource":
			s.HandleReadResource(w, r)
		case "/mcp/list_tools":
			s.HandleListTools(w, r)
		case "/mcp/call_tool":
			s.HandleCallTool(w, r)
		case "/health":
			s.HandleHealthCheck(w, r)
		default:
			http.NotFound(w, r)
		}
	}))

	return &MCPClient{
		Server:   ts,
		BaseURL:  ts.URL,
		Client:   ts.Client(),
		ServerID: fmt.Sprintf("test-%d", time.Now().UnixNano()),
	}
}

// Close closes the test server.
func (c *MCPClient) Close() {
	c.Server.Close()
}

// Initialize sends an initialization request to the MCP server.
func (c *MCPClient) Initialize(_ *testing.T, serverName, serverVersion string) (map[string]interface{}, error) {
	reqBody := map[string]interface{}{
		"server_name":    serverName,
		"server_version": serverVersion,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/mcp/initialize", bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return result, nil
}

// ListResources sends a list_resources request to the MCP server.
func (c *MCPClient) ListResources(_ *testing.T) (map[string]interface{}, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/mcp/list_resources", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return result, nil
}

// ReadResource sends a read_resource request to the MCP server.
func (c *MCPClient) ReadResource(_ *testing.T, resourceName string) (map[string]interface{}, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/mcp/read_resource?name="+url.QueryEscape(resourceName), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return result, nil
}

// ListTools sends a list_tools request to the MCP server.
func (c *MCPClient) ListTools(_ *testing.T) (map[string]interface{}, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/mcp/list_tools", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return result, nil
}

// CallTool sends a call_tool request to the MCP server.
func (c *MCPClient) CallTool(_ *testing.T, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
	reqBody := map[string]interface{}{
		"name":      toolName,
		"arguments": args,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/mcp/call_tool", bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	return result, nil
}

// RunServer starts a test server with the provided handler and returns a client.
// This is useful for more complex integration tests.
func RunServer(handler http.Handler) (*httptest.Server, string) {
	server := httptest.NewServer(handler)
	return server, server.URL
}

// CreateMCPTestServer creates a test server with the MCP server handler.
// The returned function should be called to close the server when done.
func CreateMCPTestServer(_ *testing.T, s *server.MCPServer) (*httptest.Server, func()) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp/initialize", s.HandleInitialize)
	mux.HandleFunc("/mcp/list_resources", s.HandleListResources)
	mux.HandleFunc("/mcp/read_resource", s.HandleReadResource)
	mux.HandleFunc("/mcp/list_tools", s.HandleListTools)
	mux.HandleFunc("/mcp/call_tool", s.HandleCallTool)
	mux.HandleFunc("/health", s.HandleHealthCheck)

	server := httptest.NewServer(mux)

	cleanup := func() {
		server.Close()
		// Additional cleanup if needed (e.g., stopping the MCP server)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	}

	return server, cleanup
}
