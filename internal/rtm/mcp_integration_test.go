// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ANSI Color Codes. ---.
const (
	ColorWarn    = "\033[0;33m" // Yellow for Warn
	ColorError   = "\033[0;31m" // Red for Error
	ColorSkip    = "\033[0;35m" // Magenta for Skip/Emphasis
	ColorDefault = "\033[0m"    // No Color / Reset
)

// --- Test Logger Implementation (Reinstated and Refined) ---

// testLogger adapts t.Logf to the logging.Logger interface.
// INFO logs print plainly. WARN/ERROR have prefixes/colors. DEBUG is plain+prefix.
type testLogger struct {
	t      *testing.T
	fields []any // Fields are kept for internal context propagation if needed.
}

// newTestLogger creates the refined test logger.
func newTestLogger(t *testing.T) logging.Logger {
	t.Helper() // Mark this function as a test helper.
	return &testLogger{t: t}
}

// formatArgs - Returns ONLY the message string. Key-value args are ignored for test output clarity.
func (l *testLogger) formatArgs(msg string, _ ...any) string {
	// NOTE: We completely ignore 'args' here for cleaner test output by default.
	return msg
}

// Debug logs are generally suppressed by 'go test' unless -v is used.
// Format includes args when shown with -v.
func (l *testLogger) Debug(msg string, args ...any) {
	fullMsg := msg // Start with base message
	combinedArgs := append(l.fields, args...)
	for i := 0; i < len(combinedArgs); i += 2 {
		if i+1 < len(combinedArgs) {
			fullMsg += fmt.Sprintf(" %v=%v", combinedArgs[i], combinedArgs[i+1])
		} else {
			fullMsg += fmt.Sprintf(" %v=MISSING_VALUE", combinedArgs[i])
		}
	}
	l.t.Logf("  debug: %s", fullMsg) // Plain prefix, includes args when verbose
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

// WithContext remains a no-op for the test logger itself.
func (l *testLogger) WithContext(_ context.Context) logging.Logger { return l }

// WithField creates a new logger, preserving fields internally but not adding them to test output.
func (l *testLogger) WithField(key string, value any) logging.Logger {
	newFields := make([]any, len(l.fields)+2)
	copy(newFields, l.fields)
	newFields[len(l.fields)] = key
	newFields[len(l.fields)+1] = value
	return &testLogger{t: l.t, fields: newFields}
}

// --- Test Functions ---

func TestRTMToolsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode.")
	}

	cfg := config.DefaultConfig()
	if cfg.RTM.APIKey == "" || cfg.RTM.SharedSecret == "" {
		t.Logf("%sSkipping RTM integration tests: RTM_API_KEY or RTM_SHARED_SECRET not configured.%s", ColorSkip, ColorDefault)
		t.Skip("Skipping RTM integration tests: RTM_API_KEY or RTM_SHARED_SECRET not configured.")
	}

	// Pass the refined testLogger to the service
	testSpecificLogger := newTestLogger(t)
	rtmService := NewService(cfg, testSpecificLogger)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// === Run Diagnostics Phase ===
	t.Run("Phase: Pre-Initialization Diagnostics", func(t *testing.T) {
		phaseLogger := newTestLogger(t)
		phaseLogger.Info("Running connectivity diagnostics...")

		options := DefaultConnectivityCheckOptions()
		options.RequireAuth = false
		results, checkErr := rtmService.PerformConnectivityCheck(ctx, options)

		require.NoError(t, checkErr, "Connectivity check command failed")
		require.NotEmpty(t, results, "Connectivity check returned no results")

		phaseLogger.Info("Diagnostic Results:")
		allOk := true
		for _, result := range results {
			if !result.Success {
				allOk = false
				phaseLogger.Warn(fmt.Sprintf("Diagnostic Failed: %-25s - %s", result.Name, result.Description))
				if result.Error != nil {
					phaseLogger.Warn(fmt.Sprintf("  └─ Reason: %v", result.Error))
				}
				if result.Name == "RTM API Echo Test" {
					t.Errorf("Critical diagnostic failed: %s", result.Name)
				}
			}
		}

		if !allOk {
			phaseLogger.Warn("-> Some diagnostic checks failed.")
		} else {
			phaseLogger.Info("-> All diagnostic checks passed.")
		}
	}) // End Diagnostics Phase

	// === Initialize Service Phase ===
	var initErr error
	var isAuthenticated bool
	var finalUsername string
	t.Run("Phase: Initialize RTM Service", func(t *testing.T) {
		initErr = rtmService.Initialize(ctx)
		require.NoError(t, initErr, "Service initialization failed")

		isAuthenticated = rtmService.IsAuthenticated()
		finalUsername = rtmService.GetUsername()
	}) // End Initialize Phase

	// === Check Authentication and Skip if Necessary ===
	if initErr != nil {
		t.Fatalf("%sSkipping authenticated tests due to initialization error: %v%s", ColorSkip, initErr, ColorDefault)
	}
	if !isAuthenticated {
		t.Logf("%sSkipping authenticated RTM tests: Service not authenticated after initialization.%s", ColorSkip, ColorDefault)
		t.Skip("Skipping authenticated RTM tests: Service not authenticated after initialization.")
		return
	}

	// === Authenticated Tests Phase ===
	t.Run("Phase: Authenticated Operations", func(t *testing.T) {
		authLogger := newTestLogger(t)
		authLogger.Info(fmt.Sprintf("Running tests for authenticated user: %q", finalUsername))

		t.Run("Sub: GetTools", func(t *testing.T) {
			tools := rtmService.GetTools()
			assert.NotEmpty(t, tools)
			// ... other assertions ...
		})

		t.Run("Sub: CallTool_GetTasks", func(t *testing.T) {
			args := map[string]interface{}{"filter": "status:incomplete"}
			argsBytes, err := json.Marshal(args)
			require.NoError(t, err)
			result, err := rtmService.CallTool(ctx, "getTasks", argsBytes)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.False(t, result.IsError)
			assert.NotEmpty(t, result.Content)
		})

		t.Run("Sub: GetResources", func(t *testing.T) {
			resources := rtmService.GetResources()
			assert.NotEmpty(t, resources)
			// ... other assertions ...
		})

		t.Run("Sub: ReadResource_Auth", func(t *testing.T) {
			content, err := rtmService.ReadResource(ctx, "rtm://auth")
			require.NoError(t, err)
			require.NotEmpty(t, content)
			// ... other assertions ...
		})

		t.Run("Sub: ReadResource_Lists", func(t *testing.T) {
			content, err := rtmService.ReadResource(ctx, "rtm://lists")
			require.NoError(t, err)
			require.NotEmpty(t, content)
			// ... other assertions ...
		})
		// Add more authenticated sub-tests here
	}) // End Authenticated Operations Phase
}
