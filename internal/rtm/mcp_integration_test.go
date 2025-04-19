// file: internal/rtm/mcp_integration_test.go

package rtm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/mcp"
	"github.com/stretchr/testify/require"
)

// --- Test Logger Implementation ---.
type testLogger struct {
	t *testing.T
}

// newTestLogger creates a new test logger that wraps testing.T.
func newTestLogger(t *testing.T) logging.Logger {
	t.Helper()
	return &testLogger{t: t}
}

// WithContext implements Logger interface but returns the same logger.
func (l *testLogger) WithContext(_ context.Context) logging.Logger { return l }

// WithField implements Logger interface but returns the same logger.
func (l *testLogger) WithField(_ string, _ any) logging.Logger { return l }

// Debug logs a debug message when in verbose mode.
func (l *testLogger) Debug(msg string, _ ...any) {
	if testing.Verbose() {
		l.t.Logf("  [DEBUG] %s", msg)
	}
}

// Info logs an info-level message.
func (l *testLogger) Info(msg string, _ ...any) { l.t.Logf("  INFO: %s", msg) }

// Warn logs a warning-level message.
func (l *testLogger) Warn(msg string, _ ...any) { l.t.Logf("  WARN: %s", msg) }

// Error logs an error-level message.
func (l *testLogger) Error(msg string, _ ...any) { l.t.Logf("  ERROR: %s", msg) }

// --- Test Functions ---

// TestRTMToolsIntegration tests the integration of RTM service with MCP tools.
// nolint:gocyclo // Integration test involves multiple sequential steps & checks
func TestRTMToolsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode.")
	}

	// --- Test State Variables ---
	var apiKeyValid bool
	var isAuthenticated bool
	var authTestsSkipped bool
	var username string

	// --- Banner and Setup ---
	printTestHeader(t, "RTM Integration Test")

	// Check API credentials.
	cfg := config.DefaultConfig()
	if cfg.RTM.APIKey == "" || cfg.RTM.SharedSecret == "" {
		printTestResult(t, "CREDENTIALS CHECK", "MISSING", "Environment variables RTM_API_KEY or RTM_SHARED_SECRET not found.")
		printTestFooter(t, "SKIPPED", "Missing RTM credentials in environment variables.")
		t.Skip("Skipping RTM integration tests: Required credentials not found in environment variables.")
		return
	}

	printTestResult(t, "CREDENTIALS CHECK", "FOUND", fmt.Sprintf("API Key: %s... (from environment).", truncateCredential(cfg.RTM.APIKey)))

	testLogger := newTestLogger(t)
	rtmService := NewService(cfg, testLogger)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// --- Step 1: Validate Credentials ---
	printSectionHeader(t, "Credential Validation")
	options := DefaultConnectivityCheckOptions()
	options.RequireAuth = false // Don't require auth for this initial check.
	diagResults, err := rtmService.PerformConnectivityCheck(ctx, options)
	require.NoError(t, err, "Connectivity check shouldn't cause fatal error here.")

	for _, result := range diagResults {
		t.Logf("  %s", formatDiagnosticResult(result))
		if result.Name == "RTM API Echo Test" {
			apiKeyValid = result.Success
		}
	}

	if !apiKeyValid {
		printTestResult(t, "CREDENTIAL VALIDATION", "FAILED", "API Key/Secret rejected by RTM API.")
		printTestFooter(t, "FAILED", "Invalid RTM credentials - Test cannot continue.")
		t.Fatal("Cannot continue tests with invalid API credentials.")
		return
	}

	printTestResult(t, "CREDENTIAL VALIDATION", "PASSED", "API Key and Secret are accepted by RTM.")

	// --- Step 2: Check Authentication Status ---
	printSectionHeader(t, "Authentication Status")

	// Initialize service fully now (loads token, verifies final state).
	err = rtmService.Initialize(ctx)
	require.NoError(t, err, "Failed to initialize RTM service.")

	isAuthenticated = rtmService.IsAuthenticated()
	username = rtmService.GetUsername()
	if username == "" {
		username = "N/A"
	}

	if isAuthenticated {
		printTestResult(t, "AUTHENTICATION STATUS", "AUTHENTICATED",
			fmt.Sprintf("User: %s.", username))
	} else {
		authTestsSkipped = true
		printTestResult(t, "AUTHENTICATION STATUS", "NOT AUTHENTICATED",
			"No valid auth token found in system keyring or token storage.")

		// Give clear instructions for authentication.
		t.Log("")
		t.Log("  ╔════════════════════════════════════════════════════════════════╗")
		t.Log("  ║  AUTHENTICATION REQUIRED                                       ║")
		t.Log("  ╠════════════════════════════════════════════════════════════════╣")
		t.Log("  ║  To authenticate with Remember The Milk:                       ║")
		t.Log("  ║                                                                ║")
		t.Log("  ║  1. Run this command:                                          ║")
		t.Log("  ║     go run ./cmd/rtm_connection_test                           ║")
		t.Log("  ║                                                                ║")
		t.Log("  ║  2. Follow the browser instructions to authorize the app       ║")
		t.Log("  ║                                                                ║")
		t.Log("  ║  3. Re-run the tests after authorization is complete           ║")
		t.Log("  ╚════════════════════════════════════════════════════════════════╝")
		t.Log("")
	}

	// --- Step 3: Run Authenticated Operations ---
	printSectionHeader(t, "Authenticated Operations")

	if authTestsSkipped {
		printTestResult(t, "AUTHENTICATED TESTS", "SKIPPED",
			"Cannot run authenticated tests without valid auth token.")
	} else {
		runAuthenticatedTests(ctx, t, rtmService)
	}

	// --- Final Test Result ---
	var finalResult, finalReason string
	if !apiKeyValid {
		finalResult = "FAILED"
		finalReason = "Invalid API credentials (Key/Secret)."
	} else if authTestsSkipped {
		finalResult = "INCOMPLETE"
		finalReason = "Authentication required."
		// Mark as failure for the test runner.
		t.Errorf("Test incomplete: Authentication with RTM required.")
	} else if t.Failed() {
		finalResult = "FAILED"
		finalReason = "One or more authenticated operations failed."
	} else {
		finalResult = "PASSED"
		finalReason = "All credential validation and authenticated operations successful."
	}

	printTestFooter(t, finalResult, finalReason)
}

// runAuthenticatedTests executes test operations that require authentication.
// Context is passed as the first parameter per Go best practices.
func runAuthenticatedTests(ctx context.Context, t *testing.T, rtmService *Service) {
	t.Helper()

	// Test GetTools.
	tools := rtmService.GetTools()
	if len(tools) > 0 {
		printTestResult(t, "GetTools()", "PASSED", fmt.Sprintf("%d tools found.", len(tools)))
	} else {
		printTestResult(t, "GetTools()", "FAILED", "No tools returned.")
		t.Fail()
	}

	// Test CallTool (getTasks).
	args := map[string]interface{}{"filter": "status:incomplete"}
	argsBytes, _ := json.Marshal(args)
	result, callErr := rtmService.CallTool(ctx, "getTasks", argsBytes)
	if callErr == nil && result != nil && !result.IsError {
		printTestResult(t, "CallTool(getTasks)", "PASSED", "Successfully retrieved tasks.")
	} else {
		errorDetail := "Unknown tool error."
		if callErr != nil {
			errorDetail = fmt.Sprintf("Internal Error: %v.", callErr)
		} else if result != nil && result.IsError && len(result.Content) > 0 {
			if tc, ok := result.Content[0].(mcp.TextContent); ok {
				errorDetail = fmt.Sprintf("Tool Error: %s.", tc.Text)
			}
		}
		printTestResult(t, "CallTool(getTasks)", "FAILED", errorDetail)
		t.Fail()
	}

	// Test GetResources.
	resources := rtmService.GetResources()
	if len(resources) > 0 {
		printTestResult(t, "GetResources()", "PASSED", fmt.Sprintf("%d resources found.", len(resources)))
	} else {
		printTestResult(t, "GetResources()", "FAILED", "No resources returned.")
		t.Fail()
	}
}

// --- Test Output Formatting Helpers ---

// printTestHeader prints a nicely formatted test header.
func printTestHeader(t *testing.T, title string) {
	t.Helper()
	t.Log("")
	t.Log("╔═══════════════════════════════════════════════════════════════════════════╗")
	t.Logf("║ %s%s ║", title, strings.Repeat(" ", 65-len(title)))
	t.Log("╚═══════════════════════════════════════════════════════════════════════════╝")
	t.Log("")
}

// printSectionHeader prints a nicely formatted section header.
func printSectionHeader(t *testing.T, section string) {
	t.Helper()
	t.Log("")
	t.Logf("┌─── %s %s┐", section, strings.Repeat("─", 70-len(section)))
	t.Log("")
}

// printTestResult prints a clearly formatted test result.
func printTestResult(t *testing.T, test, status, details string) {
	t.Helper()
	var icon, colorCode string

	switch status {
	case "PASSED", "AUTHENTICATED", "FOUND":
		icon = "✓"
		colorCode = "\033[0;32m" // Green
	case "FAILED", "NOT AUTHENTICATED", "MISSING":
		icon = "✗"
		colorCode = "\033[0;31m" // Red
	case "SKIPPED", "INCOMPLETE":
		icon = "!"
		colorCode = "\033[0;33m" // Yellow
	default:
		icon = "•"
		colorCode = "\033[0;34m" // Blue
	}

	resetCode := "\033[0m"
	t.Logf("  %s%s %s: %s%s", colorCode, icon, test, status, resetCode)
	t.Logf("     → %s", details)
}

// printTestFooter prints a nicely formatted test footer.
func printTestFooter(t *testing.T, result, reason string) {
	t.Helper()
	var colorCode string

	switch result {
	case "PASSED":
		colorCode = "\033[0;32m" // Green
	case "FAILED":
		colorCode = "\033[0;31m" // Red
	case "SKIPPED", "INCOMPLETE":
		colorCode = "\033[0;33m" // Yellow
	default:
		colorCode = "\033[0m" // Reset
	}

	resetCode := "\033[0m"

	t.Log("")
	t.Log("└────────────────────────────────────────────────────────────────────────┘")
	t.Log("")
	t.Logf("%sTEST RESULT: %s%s", colorCode, result, resetCode)
	t.Logf("REASON: %s", reason)
	t.Log("")
}

// truncateCredential safely truncates a credential for display.
func truncateCredential(cred string) string {
	if len(cred) <= 5 {
		return "****"
	}
	return cred[:5] + "****"
}
