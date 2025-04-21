// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/mcp_integration_test.go

import (
	"context"
	"encoding/json"
	"fmt"
	"os" // Added for CI environment check.
	"strings"
	"testing"
	"time"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/mcp"
	"github.com/stretchr/testify/require"
	// Removed testify/assert import as it wasn't used directly here.
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
func (l *testLogger) Debug(msg string, args ...any) {
	if testing.Verbose() {
		logMsg := fmt.Sprintf(msg, args...) // Format message using provided args.
		l.t.Logf("  [DEBUG] %s", logMsg)
	}
}

// Info logs an info-level message.
func (l *testLogger) Info(msg string, args ...any) {
	logMsg := fmt.Sprintf(msg, args...)
	l.t.Logf("  INFO: %s", logMsg)
}

// Warn logs a warning-level message.
func (l *testLogger) Warn(msg string, args ...any) {
	logMsg := fmt.Sprintf(msg, args...)
	l.t.Logf("  WARN: %s", logMsg)
}

// Error logs an error-level message.
func (l *testLogger) Error(msg string, args ...any) {
	logMsg := fmt.Sprintf(msg, args...)
	l.t.Logf("  ERROR: %s", logMsg)
}

// --- Test Functions ---

// TestRTMService_HandlesToolCallsAndResourceReads_When_Authenticated tests the integration of RTM service with MCP tools.
// Renamed function to follow ADR-008 convention.
// NOTE: Keeping this name as it accurately describes a broad integration test.
// The internal steps and checks align with testing behavior based on state (credentials, auth).
// nolint:gocyclo // Integration test involves multiple sequential steps & checks.
func TestRTMService_HandlesToolCallsAndResourceReads_When_Authenticated(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode.")
	}

	// --- Test State Variables ---
	var apiKeyValid bool
	var isAuthenticated bool
	var authTestsSkipped bool // We'll determine this based on the outcome of the auth step
	var username string

	// --- Banner and Setup ---
	printTestHeader(t, "RTM Integration Test")

	// Check API credentials from environment.
	cfg := config.DefaultConfig() // DefaultConfig reads from env vars.
	if cfg.RTM.APIKey == "" || cfg.RTM.SharedSecret == "" {
		printTestResult(t, "CREDENTIALS CHECK", "MISSING", "Environment variables RTM_API_KEY or RTM_SHARED_SECRET not found.")
		printTestFooter(t, "SKIPPED", "Missing RTM credentials in environment variables.")
		t.Skip("Skipping RTM integration tests: Required credentials not found in environment variables.")
		return // Explicit return after skip.
	}

	printTestResult(t, "CREDENTIALS CHECK", "FOUND", fmt.Sprintf("API Key: %s... (from environment).", truncateCredential(cfg.RTM.APIKey)))

	testLogger := newTestLogger(t) // Use our test logger.
	rtmService := NewService(cfg, testLogger)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Slightly longer timeout for auth manager.
	defer cancel()

	// --- Step 1: Validate Credentials ---
	printSectionHeader(t, "Credential Validation")
	options := DefaultConnectivityCheckOptions()
	options.RequireAuth = false // Don't require auth for this initial check.
	// Explicitly enable checks needed for validation.
	options.CheckInternet = true
	options.CheckRTMAPI = true
	options.CheckAPIKey = true
	diagResults, err := rtmService.PerformConnectivityCheck(ctx, options)
	// Check for fatal error during the connectivity check itself.
	require.NoError(t, err, "Connectivity check shouldn't cause fatal error here (check internet/endpoint reachability).")

	// Evaluate results of individual checks.
	for _, result := range diagResults {
		t.Logf("  %s", formatDiagnosticResult(result)) // Use helper from diagnostics.go.
		if result.Name == "RTM API Echo Test" {
			apiKeyValid = result.Success
		}
	}

	if !apiKeyValid {
		printTestResult(t, "CREDENTIAL VALIDATION", "FAILED", "API Key/Secret rejected by RTM API (rtm.test.echo failed).")
		printTestFooter(t, "FAILED", "Invalid RTM credentials - Test cannot continue.")
		// Use t.Fatal to stop immediately if credentials are bad.
		t.Fatal("Cannot continue tests with invalid API credentials.")
		return // Explicit return.
	}

	printTestResult(t, "CREDENTIAL VALIDATION", "PASSED", "API Key and Secret are accepted by RTM.")

	// --- Step 2: Check Authentication Status ---
	printSectionHeader(t, "Authentication Status")

	// Instead of direct Initialize, use AuthManager to handle the flow.
	authOptions := AuthManagerOptions{
		Mode:             AuthModeTest, // Use test mode.
		AutoCompleteAuth: true,         // Attempt auto-completion if possible.
		TimeoutDuration:  30 * time.Second,
	}

	authManager := NewAuthManager(rtmService, authOptions, testLogger)
	// FIX: Assign both return values from EnsureAuthenticated
	_, authErr := authManager.EnsureAuthenticated(ctx) // Assign to _, authErr

	// Check if authentication failed.
	if authErr != nil {
		// FIX: Removed ineffectual assignment
		// authTestsSkipped = true // Marked tests as skipped.
		printTestResult(t, "AUTHENTICATION STATUS", "FAILED", fmt.Sprintf("AuthManager failed: %v.", authErr))

		// If running in CI, skip the test gracefully.
		if os.Getenv("CI") != "" {
			printTestFooter(t, "SKIPPED", "Skipping authentication-dependent test in CI environment.")
			t.Skip("Skipping authentication-dependent test in CI environment.")
			return
		}

		// Otherwise, show clear instructions and fail fatally.
		t.Log("")
		t.Log("  ╔════════════════════════════════════════════════════════════════╗")
		t.Log("  ║  AUTHENTICATION REQUIRED - ACTION NEEDED                       ║")
		t.Log("  ╠════════════════════════════════════════════════════════════════╣")
		t.Log("  ║  Authentication failed. To manually authenticate:              ║")
		t.Log("  ║  1. Run this command:                                          ║")
		t.Log("  ║     go run ./cmd/rtm_connection_test                           ║")
		t.Log("  ║  2. Follow the browser authorization steps.                    ║")
		t.Log("  ║  3. Re-run the tests.                                          ║")
		t.Log("  ╚════════════════════════════════════════════════════════════════╝")
		t.Log("")
		printTestFooter(t, "FAILED", fmt.Sprintf("Authentication required but failed: %v.", authErr))
		t.Fatalf("Authentication required but failed: %v.", authErr) // Use Fatalf to include error.
		return                                                       // Explicit return.
	}

	// Authentication succeeded, proceed.
	isAuthenticated = rtmService.IsAuthenticated() // Check final state.
	username = rtmService.GetUsername()
	if username == "" {
		username = "N/A" // Should not happen if EnsureAuthenticated succeeded.
	}

	if isAuthenticated {
		printTestResult(t, "AUTHENTICATION STATUS", "AUTHENTICATED",
			fmt.Sprintf("User: %s.", username))
		authTestsSkipped = false // Authentication succeeded
	} else {
		// This block should ideally not be reached if EnsureAuthenticated succeeded without error.
		// FIX: Removed ineffectual assignment
		// authTestsSkipped = true // Authentication somehow failed despite no error
		printTestResult(t, "AUTHENTICATION STATUS", "INCONSISTENT",
			"AuthManager reported success but service state is not authenticated.")
		printTestFooter(t, "FAILED", "Inconsistent authentication state after AuthManager.")
		t.Fatal("Inconsistent authentication state.")
		return
	}

	// --- Step 3: Run Authenticated Operations ---
	printSectionHeader(t, "Authenticated Operations")

	if authTestsSkipped {
		// This block should also ideally not be reached if logic above is correct.
		printTestResult(t, "AUTHENTICATED TESTS", "SKIPPED",
			"Cannot run authenticated tests without valid auth token.")
	} else {
		// Run the sub-tests requiring authentication.
		runAuthenticatedTests(ctx, t, rtmService)
	}

	// --- Final Test Result ---
	// Determine overall outcome based on previous steps.
	var finalResult, finalReason string
	if !apiKeyValid {
		// This case should be caught by t.Fatal earlier, but included for completeness.
		finalResult = "FAILED"
		finalReason = "Invalid API credentials (Key/Secret)."
	} else if authTestsSkipped { // Check if auth was skipped *because* it failed earlier
		finalResult = "INCOMPLETE"
		finalReason = "Authentication required to run all operations, but failed."
		// Mark test as failed if auth was skipped but expected implicitly by integration nature.
		t.Errorf("Test incomplete: Authentication with RTM required but failed.")
	} else if t.Failed() {
		finalResult = "FAILED"
		finalReason = "One or more authenticated operations failed (check logs above)."
	} else {
		finalResult = "PASSED"
		finalReason = "All credential validation and authenticated operations successful."
	}

	printTestFooter(t, finalResult, finalReason)
}

// runAuthenticatedTests executes test operations that require authentication.
// Context is passed as the first parameter per Go best practices.
func runAuthenticatedTests(ctx context.Context, t *testing.T, rtmService *Service) {
	t.Helper() // Mark this as a helper function.

	// Test GetTools returns a non-empty list.
	tools := rtmService.GetTools()
	if len(tools) > 0 {
		printTestResult(t, "GetTools()", "PASSED", fmt.Sprintf("%d tools found.", len(tools)))
	} else {
		printTestResult(t, "GetTools()", "FAILED", "No tools returned.")
		t.Fail() // Mark test as failed but continue other checks.
	}

	// Test CallTool (getTasks) - Basic check for success.
	// More specific task content checks could be added if needed.
	args := map[string]interface{}{"filter": "status:incomplete"} // Example filter.
	argsBytes, err := json.Marshal(args)
	require.NoError(t, err, "Failed to marshal args for getTasks.") // Use require for setup errors.

	result, callErr := rtmService.CallTool(ctx, "getTasks", argsBytes)
	if callErr == nil && result != nil && !result.IsError {
		printTestResult(t, "CallTool(getTasks)", "PASSED", "Successfully retrieved tasks.")
		// Optionally log content preview using the helper from helpers.go.
		if len(result.Content) > 0 {
			// Use mcp.TextContent type defined in internal/mcp/types.go.
			if tc, ok := result.Content[0].(mcp.TextContent); ok {
				t.Logf("     → Tasks Result Preview: %s...", truncateString(tc.Text, 80)) // Uses helper from helpers.go.
			}
		}
	} else {
		errorDetail := "Unknown tool error."
		if callErr != nil {
			errorDetail = fmt.Sprintf("Internal Error: %v.", callErr)
		} else if result != nil && result.IsError && len(result.Content) > 0 {
			// Use mcp.TextContent type defined in internal/mcp/types.go.
			if tc, ok := result.Content[0].(mcp.TextContent); ok {
				errorDetail = fmt.Sprintf("Tool Error: %s.", tc.Text)
			}
		}
		printTestResult(t, "CallTool(getTasks)", "FAILED", errorDetail)
		t.Fail() // Mark test as failed.
	}

	// Test GetResources returns a non-empty list.
	resources := rtmService.GetResources()
	if len(resources) > 0 {
		printTestResult(t, "GetResources()", "PASSED", fmt.Sprintf("%d resources found.", len(resources)))
	} else {
		printTestResult(t, "GetResources()", "FAILED", "No resources returned.")
		t.Fail() // Mark test as failed.
	}

	// Test ReadResource (rtm://auth).
	resourceContents, readErr := rtmService.ReadResource(ctx, "rtm://auth")
	if readErr == nil && len(resourceContents) > 0 {
		printTestResult(t, "ReadResource(rtm://auth)", "PASSED", "Successfully read auth resource.")
		// Optionally log content preview using the helper from helpers.go.
		// Use mcp.TextResourceContents type defined in internal/mcp/types.go.
		if tc, ok := resourceContents[0].(mcp.TextResourceContents); ok {
			t.Logf("     → Auth Resource Preview: %s...", truncateString(tc.Text, 80)) // Uses helper from helpers.go.
		}
	} else {
		printTestResult(t, "ReadResource(rtm://auth)", "FAILED", fmt.Sprintf("Error reading resource: %v.", readErr))
		t.Fail()
	}

	// Add more calls to other authenticated tools/resources as needed.
	// e.g., Test CallTool(createTask), ReadResource(rtm://lists) etc.
}

// --- Test Output Formatting Helpers ---

// printTestHeader prints a nicely formatted test header.
func printTestHeader(t *testing.T, title string) {
	t.Helper()
	t.Log("")
	t.Log("╔═══════════════════════════════════════════════════════════════════════════╗")
	t.Logf("║ \033[0;34m▶  Running %s...\033[0m%s ║", title, strings.Repeat(" ", 59-len(title)))
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

// printTestResult prints a clearly formatted test result with color codes.
func printTestResult(t *testing.T, test, status, details string) {
	t.Helper()
	var icon, colorCode string

	// ANSI color codes.
	colorReset := "\033[0m"
	colorGreen := "\033[0;32m"
	colorRed := "\033[0;31m"
	colorYellow := "\033[0;33m"
	colorBlue := "\033[0;34m"

	switch status {
	case "PASSED", "AUTHENTICATED", "FOUND":
		icon = "✓"
		colorCode = colorGreen
	case "FAILED", "NOT AUTHENTICATED", "MISSING", "INCONSISTENT": // Added INCONSISTENT.
		icon = "✗"
		colorCode = colorRed
	case "SKIPPED", "INCOMPLETE":
		icon = "!" // Using ! for skipped/incomplete.
		colorCode = colorYellow
	default:
		icon = "•" // Default bullet for other statuses.
		colorCode = colorBlue
	}

	// Print formatted log lines.
	t.Logf("  %s%s %s: %s%s", colorCode, icon, test, status, colorReset)
	t.Logf("     → %s", details) // Details are not colored.
}

// printTestFooter prints a nicely formatted test footer with color.
func printTestFooter(t *testing.T, result, reason string) {
	t.Helper()
	var colorCode string

	// ANSI color codes defined again for clarity within this function scope.
	colorReset := "\033[0m"
	colorGreen := "\033[0;32m"
	colorRed := "\033[0;31m"
	colorYellow := "\033[0;33m"

	switch result {
	case "PASSED":
		colorCode = colorGreen
	case "FAILED":
		colorCode = colorRed
	case "SKIPPED", "INCOMPLETE":
		colorCode = colorYellow
	default:
		colorCode = colorReset // Default to no color.
	}

	t.Log("")
	t.Log("└────────────────────────────────────────────────────────────────────────┘")
	t.Log("")
	t.Logf("%sTEST RESULT: %s%s", colorCode, result, colorReset)
	t.Logf("     REASON: %s", reason)
	t.Log("")
}

// truncateCredential safely truncates a credential string for display.
func truncateCredential(cred string) string {
	maxLength := 5 // Show first 5 chars.
	if len(cred) <= maxLength {
		return strings.Repeat("*", len(cred)) // Mask short credentials entirely.
	}
	return cred[:maxLength] + strings.Repeat("*", len(cred)-maxLength) // Show prefix, mask rest.
}

// Helper for formatting diagnostic results (if needed, otherwise remove).
// func formatDiagnosticResult(result DiagnosticResult) string { ... }
