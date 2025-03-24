// Package mcp provides test utilities for MCP protocol testing.
// file: test/mcp/special_cases.go
package mcp

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cowgnition/cowgnition/test/mcp/helpers"
)

// SpecialCases tests special edge cases for MCP protocol compliance.
func SpecialCases(t *testing.T, client *helpers.MCPClient) {
	// Test case-sensitivity in resource names
	t.Run("ResourceCaseSensitivity", func(t *testing.T) {
		// MCP URIs should be case-sensitive
		resp, err := client.ReadResource(t, "AUTH://RTM")

		// Should fail for uppercase resource name
		if err == nil {
			t.Error("Uppercase resource name should not be accepted")
		}

		// Or return an error response if err is nil
		if err == nil && resp != nil {
			validateErrorResponse(t, resp)
		}
	})

	// Test for expected headers in responses
	t.Run("ResponseHeaders", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			client.BaseURL+"/mcp/list_resources", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := client.Client.Do(req)
		if err != nil {
			t.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()

		// Check Content-Type header
		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			t.Errorf("Response Content-Type should be application/json, got %s", contentType)
		}
	})
}

// validateErrorResponse validates an error response structure.
// This function will eventually be moved to a shared helpers file
func validateErrorResponse(t *testing.T, response map[string]interface{}) {
	t.Helper()

	// Check for error field or status field (required in MCP error responses)
	if response["error"] == nil && response["status"] == nil {
		t.Error("Error response missing both error and status fields")
	}

	// MCP spec requires status field in standardized error responses
	if status, ok := response["status"].(float64); ok {
		if status < 400 {
			t.Errorf("Error status code should be >= 400, got %v", status)
		}
	}

	// If error field is an object, validate its structure
	if errObj, ok := response["error"].(map[string]interface{}); ok {
		// Check for required error fields
		if code, ok := errObj["code"].(float64); !ok {
			t.Error("Error object missing code field or code is not a number")
		} else if code == 0 {
			t.Error("Error code should not be 0")
		}

		if msg, ok := errObj["message"].(string); !ok || msg == "" {
			t.Error("Error object missing message field or message is empty")
		}
	}

	// Check for timestamp field (recommended in errors)
	if ts, ok := response["timestamp"].(string); ok {
		// Attempt to parse timestamp to validate format
		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("Invalid timestamp format: %s", ts)
		}
	}
}
