// Package conformance provides tests to verify MCP protocol compliance.
package conformance

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/conformance/stubs" // Import the stubs package.
	"github.com/cowgnition/cowgnition/test/helpers"
)

// RTMLiveTestFramework provides a framework for running tests with the real RTM API.
type RTMLiveTestFramework struct {
	T               *testing.T
	Server          *server.MCPServer
	Client          *helpers.MCPClient
	RTMClient       *helpers.RTMTestClient
	TestConfig      *helpers.TestConfig
	StartTime       time.Time
	InitialReqCount int
}

// NewRTMLiveTestFramework creates a new framework for running tests with the real RTM API.
func NewRTMLiveTestFramework(t *testing.T) (*RTMLiveTestFramework, error) {
	t.Helper()

	// Load test configuration.
	testConfig, err := helpers.LoadTestConfig("")
	if err != nil {
		t.Logf("Warning: Error loading test config: %v", err)
		return nil, fmt.Errorf("failed to load test config: %w", err) // Return the error.
	}

	// Skip if live tests are disabled.
	if testConfig.Options.SkipLiveTests || helpers.ShouldSkipLiveTests() {
		t.Skip("Skipping live RTM tests (RTM_SKIP_LIVE_TESTS=true)")
	}

	// Skip if credentials are not available.
	if !testConfig.HasRTMCredentials() {
		t.Skip("Skipping live RTM tests (no credentials available)")
	}

	// Create a test configuration.
	serverCfg := &config.Config{
		Server: config.ServerConfig{
			Name: "Live Test MCP Server",
			Port: 8080,
		},
		RTM: config.RTMConfig{
			APIKey:       testConfig.RTM.APIKey,
			SharedSecret: testConfig.RTM.SharedSecret,
			Permission:   "delete", // Request full access for testing.
		},
		Auth: config.AuthConfig{
			TokenPath: t.TempDir() + "/token",
		},
	}

	// Create and initialize server.
	s, err := server.NewServer(serverCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	// Create MCP test client.
	client := helpers.NewMCPClient(t, s)

	// Create RTM test client for interacting directly with the RTM API.
	rtmClient, err := helpers.NewRTMTestClient(testConfig.RTM.APIKey, testConfig.RTM.SharedSecret)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create RTM test client: %w", err)
	}

	// Set authenticated token if available.
	if testConfig.RTM.AuthToken != "" {
		// Try to set the token on the server.
		if err := stubs.SetAuthTokenOnServer(s, testConfig.RTM.AuthToken); err != nil {
			t.Logf("Warning: %v", err)

			// Also try setting it on the RTM client.
			rtmClient.SetAuthToken(testConfig.RTM.AuthToken)
			valid, err := rtmClient.CheckToken()
			if err != nil {
				t.Logf("Warning: Error checking token: %v", err)
			} else if !valid {
				t.Logf("Token in test configuration is invalid")
			} else {
				t.Logf("Successfully validated token, but couldn't set it on server")
			}
		} else {
			t.Logf("Successfully set authentication token on server")
		}
	}

	startTime := time.Now()
	initialReqCount := rtmClient.GetRequestCount()

	return &RTMLiveTestFramework{
		T:               t,
		Server:          s,
		Client:          client,
		RTMClient:       rtmClient,
		TestConfig:      testConfig,
		StartTime:       startTime,
		InitialReqCount: initialReqCount,
	}, nil
}

// Close cleans up resources used by the framework.
func (f *RTMLiveTestFramework) Close() {
	// Log API usage.
	requests := f.RTMClient.GetRequestCount() - f.InitialReqCount
	duration := time.Since(f.StartTime)
	f.T.Logf("Test ran for %v and made %d RTM API requests (%.2f req/sec)",
		duration, requests, float64(requests)/duration.Seconds())

	// Close clients.
	f.Client.Close()
	f.RTMClient.Close()
}

// RequireAuthenticated ensures the server is authenticated with RTM.
func (f *RTMLiveTestFramework) RequireAuthenticated(ctx context.Context, interactive bool) bool {
	// Check if already authenticated.
	if stubs.IsServerAuthenticated(ctx, f.Client) {
		f.T.Logf("Server is already authenticated")
		return true
	}

	// If not interactive, just fail.
	if !interactive {
		f.T.Logf("Server is not authenticated and interactive mode is disabled")
		return false
	}

	// Get auth resource to start authentication flow.
	resp, err := stubs.ReadResource(ctx, f.Client, "auth://rtm")
	if err != nil {
		f.T.Logf("Failed to read auth resource: %v", err)
		return false
	}

	content := fmt.Sprintf("%v", resp["content"])
	if content == "" {
		f.T.Logf("Auth resource returned invalid content")
		return false
	}

	// Extract auth URL and frob from content.
	authURL, frob := stubs.ExtractAuthInfoFromContent(content)
	if authURL == "" || frob == "" {
		f.T.Logf("Could not extract auth URL and frob from content")

		// Get frob directly from RTM API for testing.
		var err error
		frob, err = f.RTMClient.GetFrob()
		if err != nil {
			f.T.Logf("Failed to get frob from RTM API: %v", err)
			return false
		}

		authURL = f.RTMClient.GetAuthURL(frob, "delete")
	}

	// Prompt user to authenticate.
	fmt.Printf("\n\n")
	fmt.Printf("┌────────────────────────────────────────────────────────────────────┐\n")
	fmt.Printf("│                         AUTHENTICATION REQUIRED                     │\n")
	fmt.Printf("└────────────────────────────────────────────────────────────────────┘\n\n")
	fmt.Printf("To proceed with live testing, please authenticate with Remember The Milk:\n\n")
	fmt.Printf("1. Open this URL in your browser: %s\n\n", authURL)
	fmt.Printf("2. Log in and authorize the application\n\n")
	fmt.Printf("3. After authorizing, enter any key to continue the test\n\n")

	// Wait for user to authenticate.
	_, err = fmt.Scanln() // Check error return value
	if err != nil {
		f.T.Logf("Error reading input: %v", err)
		// Continue anyway since we just need any input
	}

	// Now that the user has authenticated, exchange the frob for a token.
	token, err := f.RTMClient.GetToken(frob)
	if err != nil {
		f.T.Logf("Failed to get token: %v", err)
		return false
	}

	// Save the token for future tests.
	f.TestConfig.SetRTMAuthToken(token)
	if err := helpers.SaveTestConfig(f.TestConfig, ""); err != nil {
		f.T.Logf("Warning: Failed to save test config: %v", err)
	} else {
		f.T.Logf("Saved authentication token for future tests")
	}

	// Set token on server.
	if err := stubs.SetAuthTokenOnServer(f.Server, token); err != nil {
		f.T.Logf("Warning: %v", err)

		// Complete authentication using the call_tool interface.
		result, err := stubs.CallTool(ctx, f.Client, "authenticate", map[string]interface{}{
			"frob": frob,
		})
		if err != nil {
			f.T.Logf("Failed to call authenticate tool: %v", err)
			return false
		}

		f.T.Logf("Authentication result: %v", result["result"])
	} else {
		f.T.Logf("Successfully set authentication token on server")
	}

	// Verify authentication was successful.
	return stubs.IsServerAuthenticated(ctx, f.Client)
}

// RunAuthenticatedTest runs a test function that requires authentication.
func (f *RTMLiveTestFramework) RunAuthenticatedTest(name string, interactive bool, testFn func(t *testing.T)) {
	f.T.Run(name, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Check if the server is authenticated.
		if !f.RequireAuthenticated(ctx, interactive) {
			t.Skip("Skipping authenticated test: server is not authenticated")
		}

		// Run the test function.
		testFn(t)
	})
}
