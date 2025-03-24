// Package mcp provides a framework for testing MCP protocol compliance.
// file: test/conformance/mcp/test_framework.go
package mcp

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/common/helpers"
	"github.com/cowgnition/cowgnition/test/mcp"
	mcphelpers "github.com/cowgnition/cowgnition/test/mcp/helpers"
	"github.com/cowgnition/cowgnition/test/rtm/mocks"
)

// MCPTestFramework provides a framework for testing MCP protocol compliance.
type MCPTestFramework struct {
	T         *testing.T
	Server    *server.MCPServer
	Client    *mcphelpers.MCPClient
	RTMMock   *mocks.RTMServer
	StartTime time.Time
}

// NewMCPTestFramework creates a new MCP test framework.
func NewMCPTestFramework(t *testing.T) (*MCPTestFramework, error) {
	t.Helper()

	// Create a test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Name: "MCP Test Server",
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

	// Create a mock RTM server
	rtmMock := mocks.NewRTMServer(t)

	// Setup mock responses for all required RTM API endpoints
	mcp.SetupMockRTMResponses(rtmMock)

	// Override RTM API endpoint in client if using a mock
	if err := os.Setenv("RTM_API_ENDPOINT", rtmMock.BaseURL); err != nil {
		rtmMock.Close()
		return nil, fmt.Errorf("failed to set environment variable: %w", err)
	}
	defer os.Unsetenv("RTM_API_ENDPOINT")

	// Create and start the MCP server
	s, err := server.NewServer(cfg)
	if err != nil {
		rtmMock.Close()
		return nil, fmt.Errorf("failed to create server: %w", err)
	}
	s.SetVersion("test-version")

	// Simulate authentication for testing
	if err := helpers.SimulateAuthentication(s); err != nil {
		t.Logf("Warning: Could not simulate authentication: %v", err)
	}

	// Create MCP test client
	client := mcphelpers.NewMCPClient(t, s)

	startTime := time.Now()

	return &MCPTestFramework{
		T:         t,
		Server:    s,
		Client:    client,
		RTMMock:   rtmMock,
		StartTime: startTime,
	}, nil
}

// Close cleans up resources used by the framework.
func (f *MCPTestFramework) Close() {
	duration := time.Since(f.StartTime)
	f.T.Logf("Test ran for %v", duration)

	// Close clients
	f.Client.Close()
	f.RTMMock.Close()
}

// TestInitialization runs tests for the initialization endpoint.
func (f *MCPTestFramework) TestInitialization() {
	f.T.Helper()
	f.T.Run("Initialization", func(t *testing.T) {
		// Tests for Protocol Initialization
		mcp.Initialization(t, f.Client)
	})
}

// TestResources runs tests for the resource endpoints.
func (f *MCPTestFramework) TestResources() {
	f.T.Helper()
	f.T.Run("Resources", func(t *testing.T) {
		// Tests for Resource Management
		mcp.Resources(t, f.Client)
	})
}

// TestTools runs tests for the tool endpoints.
func (f *MCPTestFramework) TestTools() {
	f.T.Helper()
	f.T.Run("Tools", func(t *testing.T) {
		// Tests for Tool Management
		mcp.Tools(t, f.Client)
	})
}

// TestSpecialCases runs tests for special edge cases.
func (f *MCPTestFramework) TestSpecialCases() {
	f.T.Helper()
	f.T.Run("SpecialCases", func(t *testing.T) {
		// Tests for Special Scenarios
		mcp.SpecialCases(t, f.Client)
	})
}

// RunComprehensiveTest runs all MCP protocol conformance tests.
func (f *MCPTestFramework) RunComprehensiveTest() {
	f.T.Helper()

	// Run all tests
	f.TestInitialization()
	f.TestResources()
	f.TestTools()
	f.TestSpecialCases()
}

// RunTest runs a specific test function.
func (f *MCPTestFramework) RunTest(name string, testFn func(t *testing.T)) {
	f.T.Helper()
	f.T.Run(name, testFn)
}
