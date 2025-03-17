// Package conformance provides tests to verify MCP protocol compliance.
package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
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
		return nil, fmt.Errorf("failed to load test config: %w", err)
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
		if err := helpers.SetAuthTokenOnServer(s, testConfig.RTM.AuthToken); err != nil {
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
	if helpers.IsAuthenticated(f.Client) {
		f.T.Logf("Server is already authenticated")
		return true
	}

	// If not interactive, just fail.
	if !interactive {
		f.T.Logf("Server is not authenticated and interactive mode is disabled")
		return false
	}

	// Get auth resource to start authentication flow.
	resp, err := readResource(ctx, f.Client, "auth://rtm")
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
	authURL, frob := ExtractAuthInfoFromContent(content)
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
	fmt.Scanln()

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
	if err := helpers.SetAuthTokenOnServer(f.Server, token); err != nil {
		f.T.Logf("Warning: %v", err)

		// Complete authentication using the call_tool interface.
		result, err := callTool(ctx, f.Client, "authenticate", map[string]interface{}{
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
	return helpers.IsAuthenticated(f.Client)
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

// readResource sends a read_resource request to the MCP server, with retry logic.
func readResource(ctx context.Context, client *helpers.MCPClient, resourceName string) (map[string]interface{}, error) {
	return withRetry(ctx, func() (map[string]interface{}, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			client.BaseURL+"/mcp/read_resource?name="+url.QueryEscape(resourceName), nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}

		resp, err := client.Client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error sending request: %w", err)
		}
		defer resp.Body.Close()

		// Read response body.
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading response: %w", err)
		}

		// Check response status.
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status code: %d, body: %s",
				resp.StatusCode, string(body))
		}

		// Parse JSON response.
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("error parsing response: %w", err)
		}

		return result, nil
	})
}

// callTool sends a call_tool request to the MCP server, with retry logic.
func callTool(ctx context.Context, client *helpers.MCPClient, name string, args map[string]interface{}) (map[string]interface{}, error) {
	return withRetry(ctx, func() (map[string]interface{}, error) {
		reqBody := map[string]interface{}{
			"name":      name,
			"arguments": args,
		}

		body, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("error marshaling request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			client.BaseURL+"/mcp/call_tool", bytes.NewBuffer(body))
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error sending request: %w", err)
		}
		defer resp.Body.Close()

		// Check response status.
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("unexpected status code: %d, body: %s",
				resp.StatusCode, string(body))
		}

		// Parse JSON response.
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("error parsing response: %w", err)
		}

		return result, nil
	})
}

// withRetry performs an action with retries and exponential backoff.
func withRetry(ctx context.Context, fn func() (map[string]interface{}, error)) (map[string]interface{}, error) {
	const maxRetries = 3
	const initialDelay = 1 * time.Second

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := initialDelay * time.Duration(1<<attempt) // Exponential backoff.
			log.Printf("Retrying after error: %v, waiting %v", lastErr, delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err
		// Check for specific error codes (e.g., rate limiting).
		if strings.Contains(err.Error(), "unexpected status code: 429") { // 429 Too Many Requests.
			continue // Retry.
		}
		if strings.Contains(err.Error(), "unexpected status code: 5") { // 5xx Server Error.
			continue // Retry
		}

		return nil, err // Don't retry other errors.
	}

	return nil, fmt.Errorf("max retries exceeded, last error: %w", lastErr)
}

// findURLEndIndex locates the end of a URL within content starting from startIdx.
func findURLEndIndex(content string, startIdx int) int {
	endIdx := startIdx

	for i := startIdx; i < len(content); i++ {
		// URL ends at any whitespace or common ending punctuation
		if content[i] == '\n' || content[i] == '\r' || content[i] == ' ' ||
			content[i] == '"' || content[i] == ')' || content[i] == ']' {
			return i
		}
		endIdx = i
	}

	// If we reach end of content without finding endpoint
	return endIdx + 1
}

// extractFrobFromURL attempts to extract the frob parameter from a URL.
func extractFrobFromURL(authURL string) string {
	// Look for the frob parameter
	frobPrefix := "frob="
	idx := strings.Index(authURL, frobPrefix)
	if idx == -1 {
		return ""
	}

	startIdx := idx + len(frobPrefix)
	endIdx := startIdx

	// Find the end of the frob value (& or end of string)
	for i := startIdx; i < len(authURL); i++ {
		if authURL[i] == '&' {
			endIdx = i
			break
		}
		endIdx = i + 1
	}

	return authURL[startIdx:endIdx]
}

// extractFrobFromContent tries to find a frob value within content using known patterns.
func extractFrobFromContent(content string) string {
	// Common patterns that precede a frob in content text
	patterns := []string{
		"frob ",
		"frob: ",
		"Frob: ",
		"frob=",
		"\"frob\": \"",
	}

	for _, pattern := range patterns {
		idx := strings.Index(content, pattern)
		if idx == -1 {
			continue
		}

		startIdx := idx + len(pattern)
		endIdx := findURLEndIndex(content, startIdx)

		if endIdx > startIdx {
			return content[startIdx:endIdx]
		}
	}

	return ""
}

// ExtractAuthInfoFromContent attempts to extract auth URL and frob from content.
func ExtractAuthInfoFromContent(content string) (string, string) {
	// Look for URL in content
	urlIdx := strings.Index(content, "https://www.rememberthemilk.com/services/auth/")
	if urlIdx == -1 {
		return "", ""
	}

	// Extract URL
	endURLIdx := findURLEndIndex(content, urlIdx)
	authURL := content[urlIdx:endURLIdx]

	// Try to extract frob, first from URL then from content text
	frob := extractFrobFromURL(authURL)

	// If frob not found in URL, look in content text
	if frob == "" {
		frob = extractFrobFromContent(content)
	}

	return authURL, frob
}
