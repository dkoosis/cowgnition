package rtm

// file: internal/rtm/mcp_integration_test.go

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time" // Ensure time is imported

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging" // Keep logging import
	"github.com/dkoosis/cowgnition/internal/mcp"     // Added for mcp.TextContent
	"github.com/stretchr/testify/require"
)

// --- Test Logger Implementation (Simplified - less relevant now) ---.
type testLogger struct {
	t *testing.T
}

func newTestLogger(t *testing.T) logging.Logger {
	t.Helper()
	return &testLogger{t: t}
}
func (l *testLogger) WithContext(_ context.Context) logging.Logger { return l }
func (l *testLogger) WithField(_ string, _ any) logging.Logger     { return l }
func (l *testLogger) Debug(msg string, _ ...any) {
	if testing.Verbose() {
		l.t.Logf("  [DEBUG] %s", msg)
	}
}
func (l *testLogger) Info(msg string, _ ...any)  { l.t.Logf("  INFO: %s", msg) } // Keep Info for potential service logs
func (l *testLogger) Warn(msg string, _ ...any)  { l.t.Logf("  WARN: %s", msg) }
func (l *testLogger) Error(msg string, _ ...any) { l.t.Logf("  ERROR: %s", msg) }

// --- Test Functions ---

//nolint:gocyclo // Integration test involves multiple sequential steps & checks
func TestRTMToolsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode.")
	}

	var apiKeyValid bool
	var isAuthenticated bool
	var authTestsSkipped bool
	var username string

	// Check API credentials
	cfg := config.DefaultConfig()
	if cfg.RTM.APIKey == "" || cfg.RTM.SharedSecret == "" {
		t.Log("--------------------------------------------------")
		t.Log("TEST: RTM Integration")
		t.Log("--------------------------------------------------")
		t.Log("RESULT: SKIP (Reason: Missing RTM_API_KEY or RTM_SHARED_SECRET environment variables)")
		t.Log("--------------------------------------------------")
		t.Skip("Skipping RTM integration tests: API credentials not configured.")
		return
	}

	testLogger := newTestLogger(t) // Logger for the service
	rtmService := NewService(cfg, testLogger)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	t.Log("--------------------------------------------------")
	t.Log("TEST: RTM Integration")
	t.Log("--------------------------------------------------")

	// --- Step 1: Validate Credentials ---
	t.Logf("\033[0;34m▶  %s...\033[0m", "Validating Credentials")
	options := DefaultConnectivityCheckOptions()
	options.RequireAuth = false // Don't require auth for this initial check
	diagResults, err := rtmService.PerformConnectivityCheck(ctx, options)
	require.NoError(t, err, "Connectivity check shouldn't cause fatal error here")

	//authCheckSuccessful := false
	for _, result := range diagResults {
		// Use the new formatDiagnosticResult helper (defined above or in diagnostics.go)
		t.Logf("  %s", formatDiagnosticResult(result)) // Log each check result cleanly
		if result.Name == "RTM API Echo Test" {
			apiKeyValid = result.Success
		}
	}

	if !apiKeyValid {
		t.Log("RESULT: FAIL (Reason: RTM API Key/Secret invalid - Echo test failed)")
		t.Log("--------------------------------------------------")
		t.Fatal("Cannot continue tests with invalid API credentials")
		return
	}
	t.Log("  Credential Check: PASS (API Key/Secret Valid)")

	// --- Step 2: Check Authentication Status ---
	t.Logf("\033[0;34m▶  %s...\033[0m", "Checking Authentication Status")
	// Initialize service fully now (loads token, verifies final state)
	err = rtmService.Initialize(ctx)
	require.NoError(t, err, "Failed to initialize RTM service")

	isAuthenticated = rtmService.IsAuthenticated()
	username = rtmService.GetUsername()
	if username == "" {
		username = "N/A"
	}

	if isAuthenticated {
		t.Logf("  RESULT: AUTHENTICATED (User: %s)", username)
	} else {
		authTestsSkipped = true
		t.Log("  RESULT: NOT AUTHENTICATED")
		t.Log("  INFO: RTM Authentication required for full test.")
		t.Log("  INFO: To authenticate:")
		t.Log("  INFO: 1. Run: go run ./cmd/rtm_connection_test")
		t.Log("  INFO: 2. Follow browser instructions.")
		t.Log("  INFO: 3. Re-run tests.")
	}

	// --- Step 3: Run Authenticated Operations ---
	t.Logf("\033[0;34m▶  %s...\033[0m", "Running Authenticated Operations")
	if authTestsSkipped {
		t.Log("  RESULT: SKIP (Reason: Not authenticated with RTM)")
	} else {
		// Test GetTools
		tools := rtmService.GetTools()
		if len(tools) > 0 {
			t.Logf("  CHECK: GetTools... PASS (%d tools found)", len(tools))
		} else {
			t.Log("  CHECK: GetTools... FAIL (No tools returned)")
			t.Fail()
		}

		// Test CallTool (getTasks)
		args := map[string]interface{}{"filter": "status:incomplete"}
		argsBytes, _ := json.Marshal(args)
		result, callErr := rtmService.CallTool(ctx, "getTasks", argsBytes)
		if callErr == nil && result != nil && !result.IsError {
			t.Log("  CHECK: CallTool(getTasks)... PASS")
		} else {
			errorDetail := "Unknown tool error"
			if callErr != nil {
				errorDetail = fmt.Sprintf("Internal Error: %v", callErr)
			} else if result != nil && result.IsError && len(result.Content) > 0 {
				if tc, ok := result.Content[0].(mcp.TextContent); ok {
					errorDetail = fmt.Sprintf("Tool Error: %s", tc.Text)
				}
			}
			t.Logf("  CHECK: CallTool(getTasks)... FAIL (%s)", errorDetail)
			t.Fail()
		}

		// Test GetResources
		resources := rtmService.GetResources()
		if len(resources) > 0 {
			t.Logf("  CHECK: GetResources... PASS (%d resources found)", len(resources))
		} else {
			t.Log("  CHECK: GetResources... FAIL (No resources returned)")
			t.Fail()
		}
		// Add more authenticated checks here...
	}

	// --- Final Result ---
	t.Log("--------------------------------------------------")
	finalResult := "PASS"
	finalReason := ""

	if authTestsSkipped {
		// Mark as failure if auth was required but skipped
		finalResult = "FAIL"
		finalReason = " (Reason: Authentication check failed or was skipped)"
		// Ensure test is marked as failed if we skipped auth tests
		t.Errorf("Test failed due to missing RTM authentication")
	} else if t.Failed() {
		// If any t.Fail() was called during authenticated ops
		finalResult = "FAIL"
		finalReason = " (Reason: One or more authenticated checks failed)"
	}

	t.Logf("OVERALL: %s%s", finalResult, finalReason)
	t.Log("--------------------------------------------------")
}

// Helper function to get working directory (optional, for debugging paths)
// func getwd(t *testing.T) string {
// 	t.Helper()
// 	wd, err := os.Getwd()
// 	if err != nil {
// 		t.Logf("Warning: Could not get working directory: %v", err)
// 		return "[unknown]"
// 	}
// 	return wd
// }
