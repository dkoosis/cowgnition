// Package common provides common testing utilities for the CowGnition MCP server.
package common

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

	"github.com/dkoosis/cowgnition/internal/server"
)

// MCPClient is a client for interacting with MCP servers in tests.
type MCPClient struct {
	Client  *http.Client
	BaseURL string
	Server  *server.Server
	close   func()
}

// NewMCPClient creates a new MCP client for testing.
// If s is non-nil, the client will be connected to the server via httptest.
// If s is nil, a new client with customized timeout and redirect policy is returned.
func NewMCPClient(t *testing.T, s *server.Server) *MCPClient {
	t.Helper()

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't follow redirects automatically in tests
			return http.ErrUseLastResponse
		},
	}

	var baseURL string
	var closeFunc func()

	if s != nil {
		// Create test server from existing server
		ts := httptest.NewServer(s.GetHTTPHandler())
		baseURL = ts.URL
		closeFunc = ts.Close
	} else {
		// External server mode - client only
		baseURL = "http://localhost:8080" // Default URL, should be overridden
		closeFunc = func() {}
	}

	return &MCPClient{
		Client:  client,
		BaseURL: baseURL,
		Server:  s,
		close:   closeFunc,
	}
}

// Close releases resources used by the client.
func (c *MCPClient) Close() {
	if c.close != nil {
		c.close()
	}
}

// Initialize sends an initialization request to the MCP server.
func (c *MCPClient) Initialize(t *testing.T, name, version string) (map[string]interface{}, error) {
	t.Helper()

	body := map[string]interface{}{
		"server_name":    name,
		"server_version": version,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/mcp/initialize", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// ListResources sends a list_resources request to the MCP server.
func (c *MCPClient) ListResources(t *testing.T) (map[string]interface{}, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/mcp/list_resources", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// ReadResource sends a read_resource request to the MCP server.
func (c *MCPClient) ReadResource(t *testing.T, resourceName string) (map[string]interface{}, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	urlPath := c.BaseURL + "/mcp/read_resource?name=" + url.QueryEscape(resourceName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// ListTools sends a list_tools request to the MCP server.
func (c *MCPClient) ListTools(t *testing.T) (map[string]interface{}, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/mcp/list_tools", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// CallTool sends a call_tool request to the MCP server.
func (c *MCPClient) CallTool(t *testing.T, name string, args map[string]interface{}) (map[string]interface{}, error) {
	t.Helper()

	body := map[string]interface{}{
		"name":      name,
		"arguments": args,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/mcp/call_tool", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}
