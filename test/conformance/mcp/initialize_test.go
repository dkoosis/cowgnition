// file: test/conformance/mcp/initialize_test.go
// Package conformance provides tests to verify MCP protocol compliance.
package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/helpers"
	"github.com/cowgnition/cowgnition/test/mocks"
	validators "github.com/cowgnition/cowgnition/test/validators/mcp"
)

// TestMCPInitializeEndpointEnhanced tests the /mcp/initialize endpoint with more
// thorough validation of the MCP protocol requirements.
func TestMCPInitializeEndpointEnhanced(t *testing.T) {
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

	// Create a mock RTM server.
	rtmMock := mocks.NewRTMServer(t)
	defer rtmMock.Close()

	// Override RTM API endpoint in client.
	if err := os.Setenv("RTM_API_ENDPOINT", rtmMock.BaseURL); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer os.Unsetenv("RTM_API_ENDPOINT")

	// Create and start the MCP server.
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	s.SetVersion("test-version")

	// Create MCP test client.
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// Test cases for initialization.
	testCases := []struct {
		name          string
		serverName    string
		serverVersion string
		wantStatus    int
		wantError     bool
	}{
		{
			name:          "Valid initialization",
			serverName:    "Test Client",
			serverVersion: "1.0.0",
			wantStatus:    http.StatusOK,
			wantError:     false,
		},
		{
			name:          "Empty server name",
			serverName:    "",
			serverVersion: "1.0.0",
			wantStatus:    http.StatusOK, // Should still accept this.
			wantError:     false,
		},
		{
			name:          "Empty server version",
			serverName:    "Test Client",
			serverVersion: "",
			wantStatus:    http.StatusOK, // Should still accept this.
			wantError:     false,
		},
		{
			name:          "Very long server name",
			serverName:    strings.Repeat("Long", 100), // 400 character name.
			serverVersion: "1.0.0",
			wantStatus:    http.StatusOK, // Should handle long names.
			wantError:     false,
		},
		{
			name:          "Non-standard version format",
			serverName:    "Test Client",
			serverVersion: "dev.build.123+456",
			wantStatus:    http.StatusOK, // Should accept non-standard versions.
			wantError:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Send initialization request.
			reqBody := map[string]interface{}{
				"server_name":    tc.serverName,
				"server_version": tc.serverVersion,
			}
			body, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("Failed to marshal request: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/mcp/initialize", strings.NewReader(string(body)))
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Client.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			// Verify status code.
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("Unexpected status code: got %d, want %d", resp.StatusCode, tc.wantStatus)
			}

			// Parse response.
			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				if !tc.wantError {
					t.Fatalf("Failed to decode response: %v", err)
				}
				return
			}

			// Validate response structure.
			// 1. Verify server_info is present and correct.
			serverInfo, ok := result["server_info"].(map[string]interface{})
			if !ok {
				t.Error("Response missing server_info field")
			} else if !validators.ValidateServerInfo(t, serverInfo, cfg.Server.Name) {
				t.Error("server_info validation failed")
			}

			// 2. Verify capabilities structure follows the MCP specification.
			capabilities, ok := result["capabilities"].(map[string]interface{})
			if !ok {
				t.Error("Response missing capabilities field")
			} else if !validators.ValidateCapabilities(t, capabilities) {
				t.Error("capabilities validation failed")
			}
		})
	}

	// Test with malformed JSON.
	t.Run("Malformed JSON", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		malformedJSON := `{"server_name": "Test", "server_version": "1.0.0", invalid_json}`
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/mcp/initialize", strings.NewReader(malformedJSON))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Should return an error status.
		if resp.StatusCode == http.StatusOK {
			t.Errorf("Expected error status for malformed JSON, got %d", resp.StatusCode)
		}
	})

	// Test with wrong HTTP method.
	t.Run("Wrong HTTP Method", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.BaseURL+"/mcp/initialize", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Should return an error status - initialize should only accept POST.
		if resp.StatusCode == http.StatusOK {
			t.Errorf("Expected error status for wrong HTTP method, got %d", resp.StatusCode)
		}
	})
}
