// Package mcp provides test utilities for MCP protocol testing.
package mcp

import (
	"os"
	"testing"

	"github.com/cowgnition/cowgnition/internal/config"
	"github.com/cowgnition/cowgnition/internal/server"
	"github.com/cowgnition/cowgnition/test/helpers"
	"github.com/cowgnition/cowgnition/test/mocks"
)

// TestComprehensiveConformance provides a comprehensive test suite for
// validating conformance with the MCP protocol specification.
func TestComprehensiveConformance(t *testing.T) {
	// Create a test configuration
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

	// Create a mock RTM server
	rtmMock := mocks.NewRTMServer(t)
	defer rtmMock.Close()

	// Setup mock responses for all required RTM API endpoints
	SetupMockRTMResponses(rtmMock)

	// Override RTM API endpoint in client if using a mock
	if err := os.Setenv("RTM_API_ENDPOINT", rtmMock.BaseURL); err != nil {
		t.Fatalf("Failed to set environment variable: %v", err)
	}
	defer os.Unsetenv("RTM_API_ENDPOINT")

	// Create and start the MCP server
	s, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}
	s.SetVersion("conformance-test-version")

	// Simulate authentication for testing
	if err := helpers.SimulateAuthentication(s); err != nil {
		t.Logf("Warning: Could not simulate authentication: %v", err)
	}

	// Create MCP test client
	client := helpers.NewMCPClient(t, s)
	defer client.Close()

	// Tests for Protocol Initialization
	t.Run("Initialization", func(t *testing.T) {
		Initialization(t, client)
	})

	// Tests for Resource Management
	t.Run("Resources", func(t *testing.T) {
		Resources(t, client)
	})

	// Tests for Tool Management
	t.Run("Tools", func(t *testing.T) {
		Tools(t, client)
	})

	// Tests for Special Scenarios
	t.Run("SpecialScenarios", func(t *testing.T) {
		SpecialCases(t, client)
	})
}
