// Package conformance provides tests to verify MCP protocol compliance.
package conformance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/helpers"
)

// TestReadResourceLive tests the read_resource endpoint with a real RTM API connection.
// This test will be skipped if RTM credentials are not available or if live tests are disabled.
func TestReadResourceLive(t *testing.T) {
	// Load test configuration
	testConfig, err := helpers.LoadTestConfig("")
	if err != nil {
		t.Logf("Error loading test config: %v", err)
	}

	// Skip if live tests are disabled
	if testConfig.Options.SkipLiveTests || helpers.ShouldSkipLiveTests() {
		t.Skip("Skipping live RTM tests (RTM_SKIP_LIVE_TESTS=true)")
	}

	// Skip if credentials are not available
	if testConfig.RTM.APIKey == "" || testConfig.RTM.SharedSecret == "" {
		t.Skip("Skipping live RTM tests (no credentials available)")
	}

	// Create a test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name: "Live Test MCP Server",
			Port: 8080,
		},
		RTM: config.RTMConfig{
			APIKey:       testConfig.RTM.APIKey,
			SharedSecret: testConfig.RTM.SharedSecret,
		},
		Auth: config.AuthConfig{
			TokenPath: t.TempDir() + "/token",
		},
	}

	// Create and initialize server with real RTM credentials
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create MCP test client
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// First, test auth resource which should be available without authentication
	t.Run("auth_resource", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Create a direct request instead of using a helper function
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			client.BaseURL+"/mcp/read_resource?name="+url.QueryEscape("auth://rtm"), nil)
		if err != nil {
			t.Fatalf("Error creating request: %v", err)
		}

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Error sending request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("Unexpected status code: %d - %s", resp.StatusCode, string(bodyBytes))
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("Error decoding response: %v", err)
		}

		// Verify response has correct structure
		content, ok := result["content"].(string)
		if !ok || content == "" {
			t.Error("Auth resource should return non-empty content")
		}

		mimeType, ok := result["mime_type"].(string)
		if !ok || mimeType == "" {
			t.Error("Auth resource should return non-empty mime_type")
		}

		t.Logf("Auth resource content type: %s", mimeType)

		// Look for authentication URL and frob in content
		if len(content) < 100 {
			t.Logf("Surprisingly short auth content: %s", content)
		}
	})

	// If we already have a token from config, see if we can authenticate the server directly
	if testConfig.RTM.AuthToken != "" {
		// Get RTM service and try to authenticate
		rtmService := s.GetRTMService()
		if rtmService == nil {
			t.Fatal("Failed to get RTM service")
		}

		rtmClient, err := helpers.NewRTMTestClient(testConfig.RTM.APIKey, testConfig.RTM.SharedSecret)
		if err != nil {
			t.Fatalf("Failed to create RTM test client: %v", err)
		}
		defer rtmClient.Close()

		// Set the token directly and check if it's valid
		rtmClient.SetAuthToken(testConfig.RTM.AuthToken)
		valid, err := rtmClient.CheckToken()
		if err != nil {
			t.Logf("Failed to validate token from config: %v", err)
			// Don't fail the test, just skip authenticated tests
		} else if valid {
			t.Logf("Successfully validated existing token")

			// TODO: Need to find a way to set token on server RTM service
			// This will likely require modifying the RTM service to allow setting tokens

			// For now, we'll skip authenticated resource tests
			t.Skip("Setting tokens directly on server not yet implemented")
		}
	}

	// If we can't authenticate automatically, print instructions for manual authentication
	t.Logf("To perform authenticated tests, you need to authenticate manually:")
	t.Logf("1. Access the auth://rtm resource")
	t.Logf("2. Follow the authentication URL")
	t.Logf("3. Authorize the application")
	t.Logf("4. Use the frob to complete authentication")
	t.Logf("5. Save the token in your test config for future use")
}
