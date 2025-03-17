// Package conformance provides tests to verify MCP protocol compliance.
package conformance

import (
	"os"
	"strings"
	"testing"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/helpers"
	"github.com/cowgnition/cowgnition/test/mocks"
)

// TestMCPConformance is the main entry point for comprehensive MCP protocol conformance testing.
// It runs a full suite of tests to verify the server's compliance with the MCP specification.
func TestMCPConformance(t *testing.T) {
	// Create a test configuration.
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name: "Conformance Test Server",
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

	// Setup mock responses for all required RTM API endpoints.
	setupMockRTMResponses(rtmMock)

	// Override RTM API endpoint in client.
	if err := os.Setenv("RTM_API_ENDPOINT", rtmMock.BaseURL); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer os.Unsetenv("RTM_API_ENDPOINT")

	// Create the MCP server.
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	s.SetVersion("conformance-test-version")

	// Simulate authentication for testing
	if err := helpers.SimulateAuthentication(s); err != nil {
		t.Logf("Warning: Could not simulate authentication: %v", err)
	}

	// Create a protocol tester.
	tester := NewMCPProtocolTester(t, s)
	defer tester.Close()

	// Run comprehensive protocol tests.
	t.Run("ProtocolConformance", func(t *testing.T) {
		tester.RunComprehensiveTest()
	})

	// Run additional tests for authentication scenarios.
	t.Run("AuthenticationScenarios", func(t *testing.T) {
		testAuthenticationScenarios(t, s) // Removed rtmMock
	})

	// Run additional tests for error handling.
	t.Run("ErrorHandling", func(t *testing.T) {
		testErrorHandlingScenarios(t, s) // Removed rtmMock
	})

	// Run resource-specific tests.
	t.Run("ResourceTests", func(t *testing.T) {
		testResourceImplementations(t, s) // Removed rtmMock
	})

	// Run tool-specific tests.
	t.Run("ToolTests", func(t *testing.T) {
		testToolImplementations(t, s) // Removed rtmMock
	})
}

// setupMockRTMResponses configures the mock RTM server with all necessary responses.
func setupMockRTMResponses(rtmMock *mocks.RTMServer) {
	// Authentication-related responses.
	rtmMock.AddResponse("rtm.auth.getFrob", `<rsp stat="ok"><frob>test_frob_12345</frob></rsp>`)
	rtmMock.AddResponse("rtm.auth.getToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)
	rtmMock.AddResponse("rtm.auth.checkToken", `<rsp stat="ok"><auth><token>test_token_abc123</token><perms>delete</perms><user id="123" username="test_user" fullname="Test User" /></auth></rsp>`)

	// Timeline-related responses.
	rtmMock.AddResponse("rtm.timelines.create", `<rsp stat="ok"><timeline>timeline_12345</timeline></rsp>`)

	// List-related responses.
	rtmMock.AddResponse("rtm.lists.getList", `<rsp stat="ok"><lists>
        <list id="1" name="Inbox" deleted="0" locked="1" archived="0" position="-1" smart="0" />
        <list id="2" name="Work" deleted="0" locked="0" archived="0" position="0" smart="0" />
        <list id="3" name="Personal" deleted="0" locked="0" archived="0" position="1" smart="0" />
        <list id="4" name="High Priority" deleted="0" locked="0" archived="0" position="2" smart="1">
            <filter>(priority:1)</filter>
        </list>
    </lists></rsp>`)

	// Task-related responses.
	rtmMock.AddResponse("rtm.tasks.getList", `<rsp stat="ok"><tasks>
        <list id="1">
            <taskseries id="101" created="2025-03-15T12:00:00Z" modified="2025-03-15T12:00:00Z" name="Buy groceries" source="api">
                <tags><tag>shopping</tag></tags>
                <participants/>
                <notes/>
                <task id="1001" due="2025-03-17T00:00:00Z" has_due_time="0" added="2025-03-15T12:00:00Z" completed="" deleted="" priority="1" postponed="0" estimate=""/>
            </taskseries>
            <taskseries id="102" created="2025-03-15T12:00:00Z" modified="2025-03-15T12:00:00Z" name="Finish report" source="api">
                <tags><tag>work</tag></tags>
                <participants/>
                <notes>
                    <note id="201" created="2025-03-15T12:00:00Z" modified="2025-03-15T12:00:00Z" title="">Remember to include Q1 data</note>
                </notes>
                <task id="1002" due="2025-03-16T00:00:00Z" has_due_time="0" added="2025-03-15T12:00:00Z" completed="" deleted="" priority="2" postponed="0" estimate=""/>
            </taskseries>
        </list>
    </tasks></rsp>`)

	// Tool-related responses.
	rtmMock.AddResponse("rtm.tasks.add", `<rsp stat="ok">
        <transaction id="12345" undoable="1" />
        <list id="1">
            <taskseries id="103" created="2025-03-16T12:00:00Z" modified="2025-03-16T12:00:00Z" name="New Task" source="api">
                <tags/>
                <participants/>
                <notes/>
                <task id="1003" due="" has_due_time="0" added="2025-03-16T12:00:00Z" completed="" deleted="" priority="N" postponed="0" estimate="" />
            </taskseries>
        </list>
    </rsp>`)

	rtmMock.AddResponse("rtm.tasks.complete", `<rsp stat="ok">
        <transaction id="12346" undoable="1" />
        <list id="1">
            <taskseries id="101" created="2025-03-15T12:00:00Z" modified="2025-03-16T12:00:00Z" name="Buy groceries" source="api">
                <tags><tag>shopping</tag></tags>
                <participants/>
                <notes/>
                <task id="1001" due="2025-03-17T00:00:00Z" has_due_time="0" added="2025-03-15T12:00:00Z" completed="2025-03-16T12:00:00Z" deleted="" priority="1" postponed="0" estimate=""/>
            </taskseries>
        </list>
    </rsp>`)

	// Add error responses for testing error scenarios.
	rtmMock.AddResponse("rtm.error.invalidKey", `<rsp stat="fail"><err code="100" msg="Invalid API Key" /></rsp>`)
	rtmMock.AddResponse("rtm.error.notFound", `<rsp stat="fail"><err code="112" msg="Method not found" /></rsp>`)
	rtmMock.AddResponse("rtm.error.invalidAuth", `<rsp stat="fail"><err code="98" msg="Login failed / Invalid auth token" /></rsp>`)
}

// testAuthenticationScenarios tests authentication-related scenarios.
func testAuthenticationScenarios(t *testing.T, s *server.MCPServer) { // Removed rtmMock
	t.Helper()

	// Create a protocol tester.
	tester := NewMCPProtocolTester(t, s)
	defer tester.Close()

	// Test 1: Check auth resource is available.
	content, mimeType := tester.TestReadResource("auth://rtm")
	if content == "" || mimeType == "" {
		t.Error("auth://rtm resource should be available without authentication")
	}

	// More authentication scenarios could be added here.
	// These might include testing the flow of:
	// 1. Getting auth URL
	// 2. Authenticating with frob
	// 3. Verifying authenticated state
	// 4. Testing resource/tool availability changes post-authentication
}

// testErrorHandlingScenarios tests various error scenarios to ensure proper handling.
func testErrorHandlingScenarios(t *testing.T, s *server.MCPServer) { // Removed rtmMock
	t.Helper()

	// Create a protocol tester.
	tester := NewMCPProtocolTester(t, s)
	defer tester.Close()

	// Test 1: Nonexistent resource.
	content, mimeType := tester.TestReadResource("nonexistent://resource")
	if content != "" || mimeType != "" {
		t.Error("Nonexistent resource should return empty content and mime type")
	}

	// Test 2: Nonexistent tool.
	result := tester.TestCallTool("nonexistent_tool", map[string]interface{}{})
	if result != "" {
		t.Error("Nonexistent tool should return empty result")
	}

	// More error scenarios could be added here.
}

// testResourceImplementations tests specific resource implementations.
func testResourceImplementations(t *testing.T, s *server.MCPServer) { // Removed rtmMock
	t.Helper()

	// Create a protocol tester.
	tester := NewMCPProtocolTester(t, s)
	defer tester.Close()

	// Test specific resource implementations.
	// This could include testing for content correctness, not just structure.
	resources := tester.TestListResources()
	if resources == nil {
		t.Fatal("Failed to list resources")
	}

	// Example: Test auth resource content for expected instructions.
	content, mimeType := tester.TestReadResource("auth://rtm")
	if content == "" || mimeType == "" {
		t.Error("auth://rtm resource should be available")
	} else if mimeType != "text/markdown" && mimeType != "text/plain" {
		t.Errorf("auth://rtm resource should have text MIME type, got %s", mimeType)
	} else if !strings.Contains(content, "Authentication") {
		t.Error("auth://rtm resource should contain authentication instructions")
	}
}

// testToolImplementations tests specific tool implementations.
func testToolImplementations(t *testing.T, s *server.MCPServer) { // Removed rtmMock
	t.Helper()

	// Create a protocol tester.
	tester := NewMCPProtocolTester(t, s)
	defer tester.Close()

	// Test specific tool implementations.
	// This could include testing for result correctness, not just structure.
	tools := tester.TestListTools()
	if tools == nil {
		t.Fatal("Failed to list tools")
	}

	// Example: Test authenticate tool with invalid frob.
	result := tester.TestCallTool("authenticate", map[string]interface{}{
		"frob": "invalid_frob",
	})
	if result != "" {
		t.Error("authenticate tool with invalid frob should fail")
	}
}
