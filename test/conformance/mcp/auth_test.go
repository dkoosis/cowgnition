// Package conformance provides tests to verify MCP protocol compliance.
// file: test/conformance/mcp_authenticated_resources.go
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/helpers"
	"github.com/cowgnition/cowgnition/test/mocks"
)

// TestReadResourceAuthenticated tests the resource endpoints when authenticated.
// This test validates that authenticated resources are properly exposed and
// that their content meets the MCP specification.
func TestReadResourceAuthenticated(t *testing.T) {
	// Create a test configuration.
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name: "Test MCP Server",
			Port: 8080,
		},
		RTM: config.RTMConfig{
			APIKey:       "test_key",
			SharedSecret: "test_secret",
		},
		Auth: config.AuthConfig{
			TokenPath: t.TempDir() + "/token",
		},
	}

	// Create mock RTM server with comprehensive responses.
	rtmMock := mocks.NewRTMServer(t)
	defer rtmMock.Close()

	// Add necessary responses for authenticated resources.
	rtmMock.AddResponse("rtm.auth.getFrob", `<rsp stat="ok"><frob>test_frob_12345</frob></rsp>`)
	rtmMock.AddResponse("rtm.auth.getToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)
	rtmMock.AddResponse("rtm.auth.checkToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)
	rtmMock.AddResponse("rtm.lists.getList", `<rsp stat="ok"><lists><list id="1" name="Inbox" deleted="0" locked="1" archived="0" position="-1" smart="0" /></lists></rsp>`)
	rtmMock.AddResponse("rtm.tasks.getList", `<rsp stat="ok"><tasks><list id="1"><taskseries id="1" created="2025-03-15T12:00:00Z" modified="2025-03-15T12:00:00Z" name="Test Task" source="api"><tags /><participants /><notes /><task id="1" due="" has_due_time="0" added="2025-03-15T12:00:00Z" completed="" deleted="" priority="N" postponed="0" estimate="" /></taskseries></list></tasks></rsp>`)

	// Override RTM API endpoint.
	if err := os.Setenv("RTM_API_ENDPOINT", rtmMock.BaseURL); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer os.Unsetenv("RTM_API_ENDPOINT")

	// Create server with mock RTM service.
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Simulate authentication for testing purposes
	if err := helpers.SimulateAuthentication(s); err != nil {
		t.Logf("Warning: Could not simulate authentication: %v", err)
	}

	// Create test client.
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// Verify authentication succeeded
	if !helpers.IsAuthenticated(client) {
		t.Logf("Warning: Simulation of authentication may not have succeeded, some tests might fail")
	}

	// Step 2: Verify that list_resources now returns additional resources.
	t.Run("authenticated_list_resources", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.BaseURL+"/mcp/list_resources", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Verify status code.
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, http.StatusOK)
			return
		}

		// Parse response.
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify resources field.
		resources, ok := result["resources"].([]interface{})
		if !ok {
			t.Errorf("resources is not an array: %v", result["resources"])
			return
		}

		// Should have multiple resources when authenticated.
		if len(resources) <= 1 {
			t.Errorf("Expected multiple resources when authenticated, got %d", len(resources))
			return
		}

		// Verify that authenticated resources are present.
		expectedResources := map[string]bool{
			"tasks://all":      false,
			"tasks://today":    false,
			"tasks://tomorrow": false,
			"tasks://week":     false,
			"lists://all":      false,
			"tags://all":       false,
		}

		for _, res := range resources {
			resource, ok := res.(map[string]interface{})
			if !ok {
				t.Errorf("Resource is not an object: %v", res)
				continue
			}

			name, ok := resource["name"].(string)
			if !ok {
				t.Errorf("Resource name is not a string: %v", resource["name"])
				continue
			}

			if _, found := expectedResources[name]; found {
				expectedResources[name] = true
			}

			// Validate each resource conforms to MCP spec.
			if !validateMCPResource(t, resource) {
				t.Errorf("Resource %s failed validation", name)
			}
		}

		// Check that we found at least some of the expected resources.
		resourcesFound := 0
		for name, found := range expectedResources {
			if found {
				resourcesFound++
			} else {
				t.Logf("Expected resource not found: %s", name)
			}
		}

		if resourcesFound == 0 {
			t.Error("None of the expected authenticated resources were found")
		}
	})

	// Step 3: Test accessing authenticated resources.
	t.Run("authenticated_resources_access", func(t *testing.T) {
		// Test cases for different authenticated resources.
		testCases := []struct {
			name         string
			resourceName string
		}{
			{
				name:         "Tasks All",
				resourceName: "tasks://all",
			},
			{
				name:         "Tasks Today",
				resourceName: "tasks://today",
			},
			{
				name:         "Lists All",
				resourceName: "lists://all",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				urlPath := client.BaseURL + "/mcp/read_resource?name=" + url.QueryEscape(tc.resourceName)
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath, nil)
				if err != nil {
					t.Fatalf("Failed to create request: %v", err)
				}

				resp, err := client.Client.Do(req)
				if err != nil {
					t.Fatalf("Failed to send request: %v", err)
				}
				defer resp.Body.Close()

				// Should succeed now that we're authenticated.
				if resp.StatusCode != http.StatusOK {
					body, _ := io.ReadAll(resp.Body)
					t.Errorf("Unexpected status code: got %d, want %d. Body: %s",
						resp.StatusCode, http.StatusOK, string(body))
					return
				}

				// Parse response.
				var result map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					t.Fatalf("Failed to decode response: %v", err)
				}

				// Validate resource response.
				if !validateResourceResponse(t, result) {
					t.Errorf("Resource response validation failed")
				}

				// Additional validations based on resource type.
				content, _ := result["content"].(string)
				mimeType, _ := result["mime_type"].(string)

				// Content should not be empty.
				if content == "" {
					t.Error("Resource content is empty")
				}

				// MIME type should be valid.
				if mimeType != "text/plain" && mimeType != "text/markdown" {
					t.Errorf("Unexpected MIME type: %s", mimeType)
				}

				// Content should contain some relevant information.
				// This is a bit of a fuzzy test, but it helps catch egregious issues.
				relevantTerms := map[string][]string{
					"tasks://all":   {"Tasks", "task"},
					"tasks://today": {"Tasks", "today", "due"},
					"lists://all":   {"Lists", "list"},
				}

				if terms, exists := relevantTerms[tc.resourceName]; exists {
					foundTerm := false
					for _, term := range terms {
						if strings.Contains(content, term) {
							foundTerm = true
							break
						}
					}

					if !foundTerm {
						t.Errorf("Resource content doesn't contain any expected terms: %v", terms)
						t.Logf("Content: %s", content)
					}
				}
			})
		}
	})

	// Step 4: Test authentication-related tools.
	t.Run("authentication_tools", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Get list of tools.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.BaseURL+"/mcp/list_tools", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Verify status code.
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, http.StatusOK)
			return
		}

		// Parse response.
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify tools field.
		tools, ok := result["tools"].([]interface{})
		if !ok {
			t.Errorf("tools is not an array: %v", result["tools"])
			return
		}

		// Should have multiple tools when authenticated.
		if len(tools) <= 1 {
			t.Errorf("Expected multiple tools when authenticated, got %d", len(tools))
			return
		}

		// Check for auth_status tool.
		authStatusToolFound := false
		for _, tool := range tools {
			toolObj, ok := tool.(map[string]interface{})
			if !ok {
				continue
			}

			if name, ok := toolObj["name"].(string); ok && name == "auth_status" {
				authStatusToolFound = true
				break
			}
		}

		if !authStatusToolFound {
			t.Error("auth_status tool not found when authenticated")
		}

		// Call auth_status tool to verify authentication.
		reqBody := map[string]interface{}{
			"name":      "auth_status",
			"arguments": map[string]interface{}{},
		}
		body, err := json.Marshal(reqBody)
		if err != nil {
			t.Fatalf("Failed to marshal request: %v", err)
		}

		req, err = http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/mcp/call_tool", bytes.NewBuffer(body))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Verify status code.
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, http.StatusOK)
			return
		}

		// Parse response.
		var authResult map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&authResult); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		// Verify result contains authentication status.
		resultString, ok := authResult["result"].(string)
		if !ok {
			t.Errorf("result is not a string: %v", authResult["result"])
			return
		}

		// Status should indicate we're authenticated.
		if !strings.Contains(resultString, "Authenticated") {
			t.Errorf("auth_status tool doesn't indicate authentication: %s", resultString)
		}
	})
}
