// file: cmd/rtm_connection_test/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/rtm"
)

const (
	internetTestURL = "https://www.google.com"
	rtmAPIEndpoint  = "https://api.rememberthemilk.com/services/rest/"
	testTimeout     = 30 * time.Second
)

// Flags holds the parsed command-line flags.
type Flags struct {
	configPath   string
	verbose      bool
	skipInternet bool
	skipNonAuth  bool
	skipAuth     bool
	frobArg      string
}

// TestResult represents the outcome of a test.
type TestResult struct {
	Name        string
	Success     bool
	Error       error
	Description string
	Duration    time.Duration
}

func main() {
	flags := parseFlagsAndSetupLogging()
	cfg := loadConfiguration(flags.configPath)
	logger := logging.GetLogger("rtm_connection_test") // Get logger after setup

	// Check API key and shared secret availability
	if cfg.RTM.APIKey == "" || cfg.RTM.SharedSecret == "" {
		logger.Error("RTM API key and shared secret are required")
		logger.Info("Set them in the config file or via environment variables (RTM_API_KEY, RTM_SHARED_SECRET)")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	results := runConnectionTests(ctx, cfg, flags, logger)
	printResultsSummary(results)
}

// parseFlagsAndSetupLogging parses command-line flags and initializes logging.
func parseFlagsAndSetupLogging() Flags {
	var flags Flags
	flag.StringVar(&flags.configPath, "config", "", "Path to configuration file")
	flag.BoolVar(&flags.verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&flags.skipInternet, "skip-internet", false, "Skip internet connectivity test")
	flag.BoolVar(&flags.skipNonAuth, "skip-nonauth", false, "Skip non-authenticated RTM API test")
	flag.BoolVar(&flags.skipAuth, "skip-auth", false, "Skip authenticated RTM API test")
	flag.StringVar(&flags.frobArg, "frob", "", "Frob to use for authentication completion")
	flag.Parse()

	logLevel := "info"
	if flags.verbose {
		logLevel = "debug"
	}
	logging.SetupDefaultLogger(logLevel)

	return flags
}

// loadConfiguration loads the application configuration.
func loadConfiguration(configPath string) *config.Config {
	logger := logging.GetLogger("config_loader") // Use a specific logger scope
	var cfg *config.Config
	var err error

	if configPath != "" {
		logger.Info("Loading configuration", "config_path", configPath)
		cfg, err = config.LoadFromFile(configPath)
		if err != nil {
			logger.Error("Failed to load configuration", "error", err)
			os.Exit(1)
		}
	} else {
		logger.Info("Using default configuration")
		cfg = config.DefaultConfig()
	}
	return cfg
}

// runConnectionTests executes the suite of connectivity and RTM tests.
func runConnectionTests(ctx context.Context, cfg *config.Config, flags Flags, logger logging.Logger) []TestResult {
	results := make([]TestResult, 0)
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Test 1: Internet connectivity
	if !flags.skipInternet {
		results = append(results, testInternetConnectivity(ctx, httpClient, logger))
	}

	// Test 2: RTM API availability (non-authenticated)
	if !flags.skipNonAuth {
		results = append(results, testRTMAvailability(ctx, httpClient, logger))
	}

	// Create RTM client for API tests
	factory := rtm.NewServiceFactory(cfg, logger)
	client := factory.CreateClient()
	logger.Info("Created RTM client", "api_key_length", len(cfg.RTM.APIKey))

	// Test 3: RTM API echo test (non-authenticated method)
	if !flags.skipNonAuth {
		results = append(results, testRTMEcho(ctx, client, logger))
	}

	// Test 4: RTM Authentication
	if !flags.skipAuth {
		results = append(results, handleAuthenticationTest(ctx, client, flags.frobArg, logger)...)
	}

	return results
}

// handleAuthenticationTest checks auth state and runs appropriate auth tests.
func handleAuthenticationTest(ctx context.Context, client *rtm.Client, frobArg string, logger logging.Logger) []TestResult {
	results := make([]TestResult, 0)
	authState, err := client.GetAuthState(ctx)

	if err != nil {
		logger.Error("Failed to check authentication state", "error", err)
		results = append(results, TestResult{
			Name:        "RTM Auth Check",
			Success:     false,
			Error:       err,
			Description: "Failed to check authentication state",
		})
		return results
	}

	if authState.IsAuthenticated {
		logger.Info("Already authenticated with RTM.")
		results = append(results, testRTMAuthenticated(ctx, client, logger, authState))
	} else {
		logger.Info("Not currently authenticated with RTM.")
		if frobArg != "" {
			logger.Info("Attempting to complete authentication with provided frob.")
			authCompletionResult := completeRTMAuth(ctx, client, frobArg, logger)
			results = append(results, authCompletionResult)
			if authCompletionResult.Success {
				// Re-check state after successful completion
				authState, _ := client.GetAuthState(ctx)
				results = append(results, testRTMAuthenticated(ctx, client, logger, authState))
			}
		} else {
			results = append(results, startRTMAuth(ctx, client, logger))
		}
	}
	return results
}

// printResultsSummary prints the final test results.
func printResultsSummary(results []TestResult) {
	fmt.Println("\n=== RTM Connection Test Results ===")
	allSuccess := true
	for _, result := range results {
		statusMark := "✓"
		if !result.Success {
			statusMark = "✗"
			allSuccess = false
		}
		fmt.Printf("%s %s (%.2fs)\n", statusMark, result.Name, result.Duration.Seconds())
		if result.Description != "" {
			fmt.Printf("   %s\n", result.Description)
		}
		if result.Error != nil {
			fmt.Printf("   Error: %+v\n", result.Error) // Use %+v for potentially wrapped errors
		}
	}

	fmt.Println("\n=== Summary ===")
	if allSuccess {
		fmt.Println("✅ All tests passed successfully.")
	} else {
		fmt.Println("❌ One or more tests failed.")
		os.Exit(1)
	}
}

// testInternetConnectivity checks if the internet is reachable.
func testInternetConnectivity(ctx context.Context, client *http.Client, logger logging.Logger) TestResult {
	logger.Info("Testing internet connectivity...", "url", internetTestURL)
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "HEAD", internetTestURL, nil)
	if err != nil {
		return TestResult{
			Name:        "Internet Connectivity",
			Success:     false,
			Error:       errors.Wrap(err, "failed to create request"),
			Description: "Failed to create request",
			Duration:    time.Since(start),
		}
	}

	resp, err := client.Do(req)
	duration := time.Since(start)
	if err != nil {
		return TestResult{
			Name:        "Internet Connectivity",
			Success:     false,
			Error:       errors.Wrap(err, "HTTP HEAD request failed"),
			Description: "Failed to connect to the internet",
			Duration:    duration,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return TestResult{
			Name:        "Internet Connectivity",
			Success:     false,
			Error:       errors.Errorf("HTTP status code: %d", resp.StatusCode),
			Description: "Received error status code from internet test",
			Duration:    duration,
		}
	}

	logger.Info("Internet connectivity test successful", "status", resp.Status, "duration", duration)
	return TestResult{
		Name:        "Internet Connectivity",
		Success:     true,
		Error:       nil,
		Description: fmt.Sprintf("Connected to %s (HTTP %d)", internetTestURL, resp.StatusCode),
		Duration:    duration,
	}
}

// testRTMAvailability checks if the RTM API endpoint is reachable.
func testRTMAvailability(ctx context.Context, client *http.Client, logger logging.Logger) TestResult {
	logger.Info("Testing RTM API endpoint availability...", "url", rtmAPIEndpoint)
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "HEAD", rtmAPIEndpoint, nil)
	if err != nil {
		return TestResult{
			Name:        "RTM API Availability",
			Success:     false,
			Error:       errors.Wrap(err, "failed to create request"),
			Description: "Failed to create request",
			Duration:    time.Since(start),
		}
	}

	resp, err := client.Do(req)
	duration := time.Since(start)
	if err != nil {
		return TestResult{
			Name:        "RTM API Availability",
			Success:     false,
			Error:       errors.Wrap(err, "HTTP HEAD request failed"),
			Description: "Failed to connect to RTM API endpoint",
			Duration:    duration,
		}
	}
	defer resp.Body.Close()

	logger.Info("RTM API endpoint is reachable", "status", resp.Status, "duration", duration)
	return TestResult{
		Name:        "RTM API Availability",
		Success:     true,
		Error:       nil,
		Description: fmt.Sprintf("RTM API endpoint is reachable (HTTP %d)", resp.StatusCode),
		Duration:    duration,
	}
}

// testRTMEcho tests the rtm.test.echo method (doesn't require authentication).
func testRTMEcho(ctx context.Context, client *rtm.Client, logger logging.Logger) TestResult {
	logger.Info("Testing RTM API with non-authenticated method (rtm.test.echo)...")
	start := time.Now()
	params := map[string]string{"test_param": "hello_rtm"}
	respBytes, err := client.CallMethod(ctx, "rtm.test.echo", params)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Name:        "RTM API Echo Test",
			Success:     false,
			Error:       err, // Assumes client.CallMethod wraps errors appropriately
			Description: "Failed to call rtm.test.echo method",
			Duration:    duration,
		}
	}

	respStr := string(respBytes)
	if !strings.Contains(respStr, `"test_param": "hello_rtm"`) { // More specific check
		return TestResult{
			Name:        "RTM API Echo Test",
			Success:     false,
			Error:       errors.New("response doesn't contain expected key-value pair"),
			Description: fmt.Sprintf("Unexpected response content: %s", truncateString(respStr, 100)),
			Duration:    duration,
		}
	}

	logger.Info("RTM API echo test successful", "duration", duration, "response_preview", truncateString(respStr, 100))
	return TestResult{
		Name:        "RTM API Echo Test",
		Success:     true,
		Description: "Successfully called rtm.test.echo method",
		Duration:    duration,
	}
}

// startRTMAuth starts the RTM authentication flow.
func startRTMAuth(ctx context.Context, client *rtm.Client, logger logging.Logger) TestResult {
	logger.Info("Starting RTM authentication flow...")
	start := time.Now()
	authURL, frob, err := client.StartAuthFlow(ctx)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Name:        "RTM Auth Flow Start",
			Success:     false,
			Error:       err, // Assumes StartAuthFlow wraps errors
			Description: "Failed to start authentication flow",
			Duration:    duration,
		}
	}

	description := fmt.Sprintf("Auth flow started. Please visit this URL to authorize: %s", authURL)
	logger.Info(description)
	logger.Info("After authorizing, run this program again with:", "command", fmt.Sprintf("--frob=%s", frob))

	return TestResult{
		Name:        "RTM Auth Flow Start",
		Success:     true,
		Description: description,
		Duration:    duration,
	}
}

// completeRTMAuth completes the RTM authentication flow with a frob.
func completeRTMAuth(ctx context.Context, client *rtm.Client, frob string, logger logging.Logger) TestResult {
	logger.Info("Completing RTM authentication flow...", "frob_provided", frob != "")
	start := time.Now()
	token, err := client.CompleteAuthFlow(ctx, frob)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Name:        "RTM Auth Flow Completion",
			Success:     false,
			Error:       err, // Assumes CompleteAuthFlow wraps errors
			Description: "Failed to complete authentication flow",
			Duration:    duration,
		}
	}

	// Re-verify auth state immediately after completion
	authState, verifyErr := client.GetAuthState(ctx)
	if verifyErr != nil || !authState.IsAuthenticated {
		return TestResult{
			Name:        "RTM Auth Flow Completion",
			Success:     false,
			Error:       verifyErr,
			Description: "Authentication completed, but verification failed",
			Duration:    duration,
		}
	}

	description := fmt.Sprintf("Successfully authenticated as %s.", authState.Username)
	logger.Info(description, "token_obtained", token != "")

	return TestResult{
		Name:        "RTM Auth Flow Completion",
		Success:     true,
		Description: description,
		Duration:    duration,
	}
}

// testRTMAuthenticated tests an authenticated RTM API method.
func testRTMAuthenticated(ctx context.Context, client *rtm.Client, logger logging.Logger, authState *rtm.AuthState) TestResult {
	if authState == nil || !authState.IsAuthenticated {
		return TestResult{Name: "RTM Authenticated API", Success: false, Description: "Cannot run test, not authenticated"}
	}
	logger.Info("Testing authenticated RTM API access...", "username", authState.Username)
	start := time.Now()
	lists, err := client.GetLists(ctx) // Try to get lists
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Name:        "RTM Authenticated API",
			Success:     false,
			Error:       err, // Assumes GetLists wraps errors
			Description: "Failed to call authenticated API method (rtm.lists.getList)",
			Duration:    duration,
		}
	}

	description := fmt.Sprintf("Successfully retrieved %d lists.", len(lists))
	logger.Info(description)

	// Log the first few lists for verification
	maxListsToLog := 3
	for i, list := range lists {
		if i >= maxListsToLog {
			logger.Info(fmt.Sprintf("... and %d more lists.", len(lists)-maxListsToLog))
			break
		}
		logger.Info(fmt.Sprintf("List %d:", i+1), "id", list.ID, "name", list.Name)
	}

	return TestResult{
		Name:        "RTM Authenticated API",
		Success:     true,
		Description: description,
		Duration:    duration,
	}
}

// truncateString truncates a string to a maximum length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Ensure maxLen is not negative before slicing
	if maxLen < 0 {
		maxLen = 0
	}
	return s[:maxLen] + "..."
}
