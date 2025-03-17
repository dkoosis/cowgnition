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

// TestMCPResourceLive tests the MCP server with a real RTM API connection.
// This test is skipped if RTM credentials are not available or if RTM_SKIP_LIVE_TESTS=true.
func TestMCPResourceLive(t *testing.T) {
	// Load test configuration
	testConfig, err := helpers.LoadTestConfig("")
	if err != nil {
		t.Logf("Warning: Error loading test config: %v", err)
	}

	// Skip if live tests are disabled
	if testConfig.Options.SkipLiveTests || helpers.ShouldSkipLiveTests() {
		t.Skip("Skipping live RTM tests (RTM_SKIP_LIVE_TESTS=true)")
	}

	// Skip if credentials are not available
	if !testConfig.HasRTMCredentials() {
		t.Skip("Skipping live RTM tests (no credentials available)")
	}

	// Create a test configuration
	serverCfg := &config.Config{
		Server: config.ServerConfig{
			Name: "Live Test MCP Server",
			Port: 8080,
		},
		RTM: config.RTMConfig{
			APIKey:       testConfig.RTM.APIKey,
			SharedSecret: testConfig.RTM.SharedSecret,
			Permission:   "delete", // Request full access for testing
		},
		Auth: config.AuthConfig{
			TokenPath: t.TempDir() + "/token",
		},
	}

	// Create and initialize server
	s, err := server.NewServer(serverCfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create MCP test client
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// Create RTM test client for interacting directly with the RTM API
	rtmClient, err := helpers.NewRTMTestClient(testConfig.RTM.APIKey, testConfig.RTM.SharedSecret)
	if err != nil {
		t.Fatalf("Failed to create RTM test client: %v", err)
	}
	defer rtmClient.Close()

	// Track API requests to respect limits
	startingRequests := rtmClient.GetRequestCount()
	defer func() {
		totalRequests := rtmClient.GetRequestCount() - startingRequests
		t.Logf("Total RTM API requests made: %d", totalRequests)
	}()

	// First, test if we already have a valid token
	if testConfig.RTM.AuthToken != "" {
		rtmClient.SetAuthToken(testConfig.RTM.AuthToken)
		valid, err := rtmClient.CheckToken()
		if err != nil {
			t.Logf("Warning: Error checking token: %v", err)
		} else if valid {
			t.Logf("Using valid token from test configuration")
			// TODO: Find a way to set token directly on server RTM service

			// Try to use reflection to set the token directly in the RTM service
			// This is a bit of a hack, but useful for testing
			if err := setAuthTokenOnServer(s, testConfig.RTM.AuthToken); err != nil {
				t.Logf("Note: %v - will get new token instead", err)
			} else {
				t.Logf("Successfully set authentication token on server")
			}
		} else {
			t.Logf("Token in test configuration is invalid, will authenticate")
		}
	}

	// Test 1: Access auth resource to get authentication URL
	t.Run("AuthResource", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := readResource(ctx, client, "auth://rtm")
		if err != nil {
			t.Fatalf("Failed to read auth resource: %v", err)
		}

		content, ok := resp["content"].(string)
		if !ok || content == "" {
			t.Fatalf("Auth resource returned invalid content")
		}

		mimeType, ok := resp["mime_type"].(string)
		if !ok || mimeType == "" {
			t.Fatalf("Auth resource returned invalid mime_type")
		}

		t.Logf("Auth resource content type: %s", mimeType)

		// Extract auth URL and frob from content
		authURL, frob := extractAuthInfoFromContent(content)
		if authURL == "" || frob == "" {
			t.Logf("Could not extract auth URL and frob from content")
			t.Logf("Content: %s", content)

			// Get frob directly from RTM API for testing
			frob, err = rtmClient.GetFrob()
			if err != nil {
				t.Fatalf("Failed to get frob from RTM API: %v", err)
			}

			authURL = rtmClient.GetAuthURL(frob, "delete")
			t.Logf("Got frob directly from RTM API: %s", frob)
		}

		t.Logf("Authentication URL: %s", authURL)
		t.Logf("Frob: %s", frob)

		// If we have a valid token in the test config, we should try to use it first
		if testConfig.RTM.AuthToken != "" && isServerAuthenticated(ctx, client) {
			t.Logf("Server is already authenticated, skipping authentication flow")
			return
		}

		// To complete the test, we need manual intervention
		// In a real testing scenario, we would either use a pre-authenticated token
		// or implement a headless browser to complete the flow
		fmt.Printf("\n\n")
		fmt.Printf("┌────────────────────────────────────────────────────────────────────┐\n")
		fmt.Printf("│                         AUTHENTICATION REQUIRED                     │\n")
		fmt.Printf("└────────────────────────────────────────────────────────────────────┘\n\n")
		fmt.Printf("To proceed with live testing, please authenticate with Remember The Milk:\n\n")
		fmt.Printf("1. Open this URL in your browser: %s\n\n", authURL)
		fmt.Printf("2. Log in and authorize the application\n\n")
		fmt.Printf("3. After authorizing, enter any key to continue the test\n\n")

		// Wait for user to authenticate
		fmt.Scanln()

		// Now that the user has authenticated, exchange the frob for a token
		token, err := rtmClient.GetToken(frob)
		if err != nil {
			t.Fatalf("Failed to get token: %v", err)
		}

		// Save the token for future tests
		testConfig.SetRTMAuthToken(token)
		if err := helpers.SaveTestConfig(testConfig, ""); err != nil {
			t.Logf("Warning: Failed to save test config: %v", err)
		} else {
			t.Logf("Saved authentication token for future tests")
		}

		// Set token on server if possible
		if err := setAuthTokenOnServer(s, token); err != nil {
			t.Logf("Warning: %v", err)

			// Complete authentication using the call_tool interface
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			result, err := callTool(ctx, client, "authenticate", map[string]interface{}{
				"frob": frob,
			})
			if err != nil {
				t.Fatalf("Failed to call authenticate tool: %v", err)
			}

			// Check if authentication was successful
			t.Logf("Authentication result: %v", result["result"])
		} else {
			t.Logf("Successfully set authentication token on server")
		}
	})

	// Test 2: Test if the server is authenticated
	if !isServerAuthenticated(context.Background(), client) {
		t.Fatal("Server is not authenticated, cannot continue with tests")
	}

	// Test 3: List resources while authenticated
	t.Run("ListResourcesAuthenticated", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resourcesList, err := listResources(ctx, client)
		if err != nil {
			t.Fatalf("Failed to list resources: %v", err)
		}

		// Check that resources includes task resources
		resources, ok := resourcesList["resources"].([]interface{})
		if !ok {
			t.Fatalf("Invalid resources response")
		}

		// Find task resources
		var taskResources []string
		for _, res := range resources {
			resource, ok := res.(map[string]interface{})
			if !ok {
				continue
			}

			name, ok := resource["name"].(string)
			if !ok {
				continue
			}

			if strings.HasPrefix(name, "tasks://") {
				taskResources = append(taskResources, name)
			}
		}

		if len(taskResources) == 0 {
			t.Errorf("No task resources found")
		} else {
			t.Logf("Found task resources: %v", taskResources)
		}
	})

	// Test 4: Access tasks resource
	t.Run("TasksResource", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := readResource(ctx, client, "tasks://all")
		if err != nil {
			t.Fatalf("Failed to read tasks resource: %v", err)
		}

		content, ok := resp["content"].(string)
		if !ok || content == "" {
			t.Fatalf("Tasks resource returned invalid content")
		}

		mimeType, ok := resp["mime_type"].(string)
		if !ok || mimeType == "" {
			t.Fatalf("Tasks resource returned invalid mime_type")
		}

		t.Logf("Tasks resource content type: %s", mimeType)
		t.Logf("Tasks content length: %d characters", len(content))

		if len(content) < 20 {
			t.Logf("Tasks content: %s", content)
		} else {
			// Just show a preview of the content
			t.Logf("Tasks content preview: %s...", content[:20])
		}
	})

	// Test 5: Access lists resource
	t.Run("ListsResource", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := readResource(ctx, client, "lists://all")
		if err != nil {
			t.Fatalf("Failed to read lists resource: %v", err)
		}

		content, ok := resp["content"].(string)
		if !ok || content == "" {
			t.Fatalf("Lists resource returned invalid content")
		}

		mimeType, ok := resp["mime_type"].(string)
		if !ok || mimeType == "" {
			t.Fatalf("Lists resource returned invalid mime_type")
		}

		t.Logf("Lists resource content type: %s", mimeType)
		t.Logf("Lists content length: %d characters", len(content))

		if len(content) < 20 {
			t.Logf("Lists content: %s", content)
		} else {
			// Just show a preview of the content
			t.Logf("Lists content preview: %s...", content[:20])
		}
	})

	// Test 6: Test adding a task
	t.Run("AddTask", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Add a test task
		testTaskName := fmt.Sprintf("MCP Test Task %d", time.Now().Unix())
		result, err := callTool(ctx, client, "add_task", map[string]interface{}{
			"name": testTaskName,
		})
		if err != nil {
			t.Fatalf("Failed to add task: %v", err)
		}

		resultStr, ok := result["result"].(string)
		if !ok || resultStr == "" {
			t.Fatalf("Add task returned invalid result")
		}

		t.Logf("Add task result: %s", resultStr)

		// Verify the task was added by checking the tasks resource
		resp, err := readResource(ctx, client, "tasks://all")
		if err != nil {
			t.Fatalf("Failed to read tasks resource: %v", err)
		}

		content, ok := resp["content"].(string)
		if !ok || content == "" {
			t.Fatalf("Tasks resource returned invalid content")
		}

		if !strings.Contains(content, testTaskName) {
			t.Errorf("Added task not found in tasks resource")
		}
	})
}

// Helper functions

// readResource sends a read_resource request to the MCP server.
func readResource(ctx context.Context, client *helpers.MCPClient, resourceName string) (map[string]interface{}, error) {
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

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d, body: %s",
			resp.StatusCode, string(body))
	}

	// Parse JSON response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	return result, nil
}

// listResources sends a list_resources request to the MCP server.
func listResources(ctx context.Context, client *helpers.MCPClient) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		client.BaseURL+"/mcp/list_resources", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := client.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s",
			resp.StatusCode, string(body))
	}

	// Parse JSON response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	return result, nil
}

// callTool sends a call_tool request to the MCP server.
func callTool(ctx context.Context, client *helpers.MCPClient, name string, args map[string]interface{}) (map[string]interface{}, error) {
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

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s",
			resp.StatusCode, string(body))
	}

	// Parse JSON response
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	return result, nil
}

// extractAuthInfoFromContent tries to extract auth URL and frob from the auth resource content.
func extractAuthInfoFromContent(content string) (string, string) {
	// Try to find auth URL
	authURLIndex := strings.Index(content, "https://www.rememberthemilk.com/services/auth/")
	if authURLIndex == -1 {
		return "", ""
	}

	// Extract URL by finding the end (whitespace, newline, etc.)
	endIndex := authURLIndex
	for i := authURLIndex; i < len(content); i++ {
		if content[i] == ' ' || content[i] == '\n' || content[i] == '\r' || content[i] == ']' || content[i] == ')' {
			endIndex = i
			break
		}
		endIndex = i
	}

	authURL := content[authURLIndex : endIndex+1]

	// Try to find frob in content
	frobIndex := strings.Index(content, "frob ")
	if frobIndex == -1 {
		return authURL, ""
	}

	// Extract frob
	frobStartIndex := frobIndex + 5 // Skip "frob "
	frobEndIndex := frobStartIndex
	for i := frobStartIndex; i < len(content); i++ {
		if content[i] == ' ' || content[i] == '\n' || content[i] == '\r' {
			frobEndIndex = i
			break
		}
		frobEndIndex = i
	}

	frob := content[frobStartIndex : frobEndIndex+1]

	return authURL, frob
}

// isServerAuthenticated checks if the server is authenticated with RTM.
func isServerAuthenticated(ctx context.Context, client *helpers.MCPClient) bool {
	// Try to access an authenticated resource
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		client.BaseURL+"/mcp/read_resource?name="+url.QueryEscape("tasks://all"), nil)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return false
	}

	resp, err := client.Client.Do(req)
	if err != nil {
		log.Printf("Error sending request: %v", err)
		return false
	}
	defer resp.Body.Close()

	// If we can access tasks, the server is authenticated
	return resp.StatusCode == http.StatusOK
}

// setAuthTokenOnServer attempts to set the authentication token directly on the server.
// This uses reflection to access unexported fields, which is generally not recommended
// but is useful for testing purposes.
func setAuthTokenOnServer(s *server.MCPServer, token string) error {
	// Get RTM service
	rtmService := s.GetRTMService()
	if rtmService == nil {
		return fmt.Errorf("cannot get RTM service from server")
	}

	// Use reflection to access the RTM service client
	value := reflect.ValueOf(rtmService).Elem()

	// Try to find the client field
	clientField := value.FieldByName("client")
	if !clientField.IsValid() {
		return fmt.Errorf("RTM service has no client field")
	}

	// Check if client is accessible
	if !clientField.CanInterface() {
		return fmt.Errorf("RTM service client field is not accessible")
	}

	// Get the client
	clientObj := clientField.Interface()

	// Check if the client has a SetAuthToken method
	clientValue := reflect.ValueOf(clientObj)
	setTokenMethod := clientValue.MethodByName("SetAuthToken")
	if !setTokenMethod.IsValid() {
		return fmt.Errorf("RTM client has no SetAuthToken method")
	}

	// Call SetAuthToken with the token
	setTokenMethod.Call([]reflect.Value{reflect.ValueOf(token)})

	// Also try to set the authStatus field to indicate authentication
	authStatusField := value.FieldByName("authStatus")
	if authStatusField.IsValid() && authStatusField.CanSet() {
		// Status 3 is StatusAuthenticated in our RTM package
		authStatusField.SetInt(3)
	}

	// Check if authentication worked
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if isServerAuthenticated(ctx, helpers.NewMCPClient(nil, s)) {
		return nil
	}

	return fmt.Errorf("failed to authenticate server using reflection")
}
