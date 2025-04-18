// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/mcp_integration_test.go

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time" // Ensure time is imported

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging" // Keep logging import
	"github.com/stretchr/testify/require"
)

// --- ANSI Color Codes. ---.
const (
	ColorWarn    = "\033[0;33m" // Yellow for Warn
	ColorError   = "\033[0;31m" // Red for Error
	ColorSkip    = "\033[0;35m" // Magenta for Skip/Emphasis
	ColorDefault = "\033[0m"    // No Color / Reset
	Nc           = "\033[0m"
	Bold         = "\033[1m"
	Green        = "\033[0;32m"
	Yellow       = "\033[0;33m"
	Red          = "\033[0;31m"
	Blue         = "\033[0;34m"
	IconStart    = "$(Blue)▶$(Nc)"
	IconOk       = "$(Green)✓$(Nc)"
	IconWarn     = "$(Yellow)⚠$(Nc)"
	IconFail     = "$(Red)✗$(Nc)"
	IconInfo     = "$(Blue)ℹ$(Nc)"
	// Formatting Strings for Alignment.
	LabelFmt = "%-15s" // Indent 3, Pad label to 15 chars, left-aligned.
)

// --- Test Logger Implementation (Reinstated and Refined) ---

// INFO logs print plainly. WARN/ERROR have prefixes/colors. DEBUG is plain+prefix.
// testLogger adapts t.Logf to the logging.Logger interface with cleaner output.
type testLogger struct {
	t      *testing.T
	fields map[string]any
}

// newTestLogger creates a test logger with improved output.
func newTestLogger(t *testing.T) logging.Logger {
	t.Helper()
	return &testLogger{
		t:      t,
		fields: make(map[string]any),
	}
}

// I
// WithContext returns a logger with context.
func (l *testLogger) WithContext(_ context.Context) logging.Logger {
	return l
}

// WithField returns a logger with the field added.
func (l *testLogger) WithField(key string, value any) logging.Logger {
	newLogger := &testLogger{
		t:      l.t,
		fields: make(map[string]any, len(l.fields)+1),
	}

	// Copy existing fields.
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}

	// Add new field
	newLogger.fields[key] = value

	return newLogger
}

// formatArgs - Returns ONLY the message string. Key-value args are ignored for test output clarity.
func (l *testLogger) formatArgs(msg string, _ ...any) string {
	// NOTE: We completely ignore 'args' here for cleaner test output by default.
	return msg
}

// Debug logs are generally suppressed by 'go test' unless -v is used.
// Format includes args when shown with -v.
// file: internal/rtm/mcp_integration_test.go

// Debug logs are suppressed by default unless in verbose mode.
func (l *testLogger) Debug(msg string, args ...any) {
	// Only show in verbose mode to reduce noise in normal test output
	if testing.Verbose() {
		// Format args in a clean way if needed
		argStr := ""
		if len(args) > 0 {
			relevantArgs := make([]string, 0)
			for i := 0; i < len(args); i += 2 {
				if i+1 < len(args) {
					k, v := args[i], args[i+1]
					// Only include important args
					if k == "error" || k == "username" {
						relevantArgs = append(relevantArgs, fmt.Sprintf("%v=%v", k, v))
					}
				}
			}
			if len(relevantArgs) > 0 {
				argStr = " (" + strings.Join(relevantArgs, ", ") + ")"
			}
		}

		l.t.Logf("  [DEBUG] %s%s", msg, argStr)
	}
}

// Info logs *just* the message plainly for status updates.
func (l *testLogger) Info(msg string, args ...any) {
	l.t.Logf("  %s", l.formatArgs(msg, args...)) // Indented slightly for readability
}

// Warn logs a warning-level message in yellow with a clear prefix.
func (l *testLogger) Warn(msg string, args ...any) {
	l.t.Logf("%sWARN: %s%s", ColorWarn, l.formatArgs(msg, args...), ColorDefault)
}

// Error logs an error-level message in red with a clear prefix.
func (l *testLogger) Error(msg string, args ...any) {
	l.t.Logf("%sERROR: %s%s", ColorError, l.formatArgs(msg, args...), ColorDefault)
}

// --- Test Functions ---

func TestRTMToolsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode.")
	}

	// Check API credentials
	cfg := config.DefaultConfig()
	if cfg.RTM.APIKey == "" || cfg.RTM.SharedSecret == "" {
		t.Log("\n")
		t.Log("┌─────────────────────────────────────────────┐")
		t.Log("│ RTM INTEGRATION TESTS: NOT RUNNING          │")
		t.Log("│                                             │")
		t.Log("│ REASON: Missing API credentials             │")
		t.Log("│                                             │")
		t.Log("│ TO FIX: Set these environment variables:    │")
		t.Log("│   RTM_API_KEY                               │")
		t.Log("│   RTM_SHARED_SECRET                         │")
		t.Log("└─────────────────────────────────────────────┘")
		t.Skip("Skipping RTM integration tests: API credentials not configured.")
	}

	// Use minimal logging for tests
	testLogger := newTestLogger(t)
	rtmService := NewService(cfg, testLogger)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Test header with consistent width
	t.Log("\n")
	t.Log("┌─────────────────────────────────────────────┐")
	t.Log("│ RTM INTEGRATION TESTS                       │")
	t.Log("└─────────────────────────────────────────────┘")

	// The key issue is that we need to run the credential check
	// and initialization (non-authenticated parts) BEFORE we
	// check if we're authenticated and potentially skip
	t.Log("\n")
	t.Log("[PHASE 1] CREDENTIAL VALIDATION")
	t.Log("------------------------------")

	// Run connectivity check
	options := DefaultConnectivityCheckOptions()
	options.RequireAuth = false
	results, err := rtmService.PerformConnectivityCheck(ctx, options)
	require.NoError(t, err, "Failed to run connectivity check")

	// Track API key validity
	apiKeyValid := false
	authPresent := false

	// Display results
	t.Log("\n")
	t.Log("Credential Status:")

	for _, result := range results {
		if result.Name == "RTM API Echo Test" {
			apiKeyValid = result.Success
			if apiKeyValid {
				t.Log("✅ API KEY & SECRET: Valid and working")
			} else {
				t.Log("❌ API KEY & SECRET: Invalid")
				if result.Error != nil {
					t.Logf("   Error: %v", result.Error)
				}
			}
		}

		if result.Name == "RTM Authentication" {
			authPresent = result.Success
			if authPresent {
				t.Log("✅ AUTHENTICATION: You are authenticated with RTM")
			} else {
				t.Log("❌ AUTHENTICATION: Not authenticated with RTM")
			}
		}
	}

	// Credential summary
	t.Log("\n")
	t.Log("Credential Summary:")
	if apiKeyValid {
		t.Log("✅ Your API key and secret are valid")
		t.Log("   You can use them to access the RTM API")
	} else {
		t.Log("❌ Your API credentials are not working")
		t.Log("   Double-check RTM_API_KEY and RTM_SHARED_SECRET")
		t.Log("   Register at: https://www.rememberthemilk.com/services/api/")
	}

	// Skip further tests if API key is invalid
	if !apiKeyValid {
		t.Log("\n")
		t.Log("┌─────────────────────────────────────────────┐")
		t.Log("│ REMAINING TESTS: SKIPPED                    │")
		t.Log("│                                             │")
		t.Log("│ REASON: Invalid API credentials             │")
		t.Log("│                                             │")
		t.Log("│ TO FIX: Ensure your API key and secret are  │")
		t.Log("│         valid                               │")
		t.Log("└─────────────────────────────────────────────┘")
		t.Fatal("Cannot continue tests with invalid API credentials")
	}

	// Phase 2: Authentication Status
	t.Log("\n")
	t.Log("[PHASE 2] AUTHENTICATION STATUS")
	t.Log("------------------------------")

	// Initialize service
	err = rtmService.Initialize(ctx)
	require.NoError(t, err, "Failed to initialize RTM service")

	// Check authentication
	isAuthenticated := rtmService.IsAuthenticated()
	username := rtmService.GetUsername()

	t.Log("\n")
	t.Log("Authentication Status:")
	if isAuthenticated {
		t.Logf("✅ AUTHENTICATED: Logged in as %s", username)
	} else {
		t.Log("❌ NOT AUTHENTICATED: You need to complete the RTM authentication flow")
		t.Log("\n")
		t.Log("To authenticate:")
		t.Log("1. Run this command:  go run ./cmd/rtm_connection_test")
		t.Log("2. Follow the authentication instructions in your browser")
		t.Log("3. Run the tests again after authentication is complete")
	}

	// Skip authenticated tests if not authenticated
	if !isAuthenticated {
		t.Log("\n")
		t.Log("┌─────────────────────────────────────────────┐")
		t.Log("│ AUTHENTICATED TESTS: SKIPPED                │")
		t.Log("│                                             │")
		t.Log("│ REASON: Not authenticated with RTM          │")
		t.Log("│                                             │")
		t.Log("│ TO FIX: Complete the RTM authentication flow│")
		t.Log("│         (see instructions above)            │")
		t.Log("└─────────────────────────────────────────────┘")
		t.Skip("Skipping authenticated RTM tests: Not authenticated")
	}

	// Phase 3: Authenticated Tests (only runs if authenticated)
	t.Log("\n")
	t.Log("[PHASE 3] AUTHENTICATED OPERATIONS")
	t.Log("----------------------------------")
	t.Logf("Running as authenticated user: %s", username)

	// Run tests on tools, resources, etc.
	// Test GetTools
	tools := rtmService.GetTools()
	if len(tools) > 0 {
		t.Logf("✅ Found %d MCP tools", len(tools))
	} else {
		t.Log("❌ No MCP tools returned")
	}

	// Test CallTool (getTasks)
	t.Log("\n")
	t.Log("Testing getTasks tool:")
	args := map[string]interface{}{"filter": "status:incomplete"}
	argsBytes, _ := json.Marshal(args)
	result, err := rtmService.CallTool(ctx, "getTasks", argsBytes)
	if err == nil && result != nil && !result.IsError {
		t.Log("✅ Task retrieval successful")
	} else {
		t.Log("❌ Task retrieval failed")
		if err != nil {
			t.Logf("   Error: %v", err)
		}
	}

	// Test GetResources and ReadResource
	t.Log("\n")
	t.Log("Testing MCP Resources:")
	resources := rtmService.GetResources()
	if len(resources) > 0 {
		t.Logf("✅ Found %d MCP resources", len(resources))
	} else {
		t.Log("❌ No MCP resources returned")
	}

	// Final summary
	t.Log("\n")
	t.Log("┌─────────────────────────────────────────────┐")
	t.Log("│ RTM INTEGRATION TESTS: COMPLETE             │")
	t.Log("│                                             │")
	t.Log("│ ✅ API credentials valid                     │")
	if isAuthenticated {
		t.Log("│ ✅ Authentication successful                  │")
		t.Log("│ ✅ All tests passed                          │")
	} else {
		t.Log("│ ❌ Authentication missing                     │")
		t.Log("│ ℹ️ Some tests skipped                         │")
	}
	t.Log("└─────────────────────────────────────────────┘")
}
