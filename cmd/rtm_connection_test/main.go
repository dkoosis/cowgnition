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

	// Completely disable default logger to silence those messages.
	logger := logging.GetNoopLogger()
	logging.SetDefaultLogger(logger)

	// Check API key and shared secret availability.
	if cfg.RTM.APIKey == "" || cfg.RTM.SharedSecret == "" {
		fmt.Println("\n❌ ERROR: RTM API key and shared secret are required")
		fmt.Println("Set them in the config file or via environment variables:")
		fmt.Println("  - RTM_API_KEY")
		fmt.Println("  - RTM_SHARED_SECRET")
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

	// Initialize silent logging.
	logLevel := "info"
	if flags.verbose {
		logLevel = "debug"
	}
	logging.SetupDefaultLogger(logLevel)

	return flags
}

// loadConfiguration loads the application configuration.
func loadConfiguration(configPath string) *config.Config {
	var cfg *config.Config
	var err error

	if configPath != "" {
		cfg, err = config.LoadFromFile(configPath)
		if err != nil {
			fmt.Printf("Failed to load configuration: %s\n", err)
			os.Exit(1)
		}
	} else {
		cfg = config.DefaultConfig()
	}
	return cfg
}

// runConnectionTests executes the suite of connectivity and RTM tests.
func runConnectionTests(ctx context.Context, cfg *config.Config, flags Flags, logger logging.Logger) []TestResult {
	results := make([]TestResult, 0)
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Test 1: Internet connectivity.
	if !flags.skipInternet {
		results = append(results, testInternetConnectivity(ctx, httpClient, logger))
	}

	// Test 2: RTM API availability (non-authenticated).
	if !flags.skipNonAuth {
		results = append(results, testRTMAvailability(ctx, httpClient, logger))
	}

	// Create RTM client for API tests.
	factory := rtm.NewServiceFactory(cfg, logger)
	client := factory.CreateClient()

	// Test 3: RTM API echo test (non-authenticated method).
	if !flags.skipNonAuth {
		results = append(results, testRTMEcho(ctx, client, logger))
	}

	// Test 4: RTM Authentication.
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
		results = append(results, TestResult{
			Name:        "RTM Auth Check",
			Success:     false,
			Error:       err,
			Description: "Failed to check authentication state",
		})
		return results
	}

	if authState.IsAuthenticated {
		results = append(results, testRTMAuthenticated(ctx, client, logger, authState))
	} else {
		if frobArg != "" {
			authCompletionResult := completeRTMAuth(ctx, client, frobArg, logger)
			results = append(results, authCompletionResult)
			if authCompletionResult.Success {
				// Re-check state after successful completion.
				newAuthState, _ := client.GetAuthState(ctx)
				results = append(results, testRTMAuthenticated(ctx, client, logger, newAuthState))
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

	var authURL string
	var frob string
	authNeeded := false

	for _, result := range results {
		statusMark := "✓"
		if !result.Success {
			statusMark = "✗"
			allSuccess = false
		}

		fmt.Printf("%s %s (%.2fs)\n", statusMark, result.Name, result.Duration.Seconds())
		fmt.Printf("   %s\n", result.Description)

		if result.Error != nil {
			fmt.Printf("   Error: %v\n", result.Error)
		}

		// Extract auth URL and frob if available.
		if result.Name == "RTM Auth Flow Start" && result.Success {
			authNeeded = true
			desc := result.Description
			if strings.Contains(desc, "https://") {
				urlStart := strings.Index(desc, "https://")
				urlEnd := strings.Index(desc[urlStart:], " ")
				if urlEnd == -1 {
					authURL = desc[urlStart:]
				} else {
					authURL = desc[urlStart : urlStart+urlEnd]
				}

				if strings.Contains(authURL, "frob=") {
					frobStart := strings.Index(authURL, "frob=") + 5
					frobEnd := strings.Index(authURL[frobStart:], "&")
					if frobEnd == -1 {
						frob = authURL[frobStart:]
					} else {
						frob = authURL[frobStart : frobStart+frobEnd]
					}
				}
			}
		}
	}

	fmt.Println("\n=== Summary ===")
	if !allSuccess && authNeeded {
		fmt.Println("⚠️  Authentication needed")
		fmt.Println("\nTo complete authentication:")
		if authURL != "" {
			fmt.Println("1. Open this URL in your browser:")
			fmt.Println("   " + authURL)
		}
		fmt.Println("2. Authorize CowGnition in your RTM account")
		if frob != "" {
			fmt.Println("3. Run this command:")
			fmt.Println("   go run ./cmd/rtm_connection_test --frob=" + frob)
		}
	} else if allSuccess {
		fmt.Println("✅ All tests passed successfully.")
	} else {
		fmt.Println("❌ One or more tests failed.")
		os.Exit(1)
	}
}

// testInternetConnectivity checks if the internet is reachable.
func testInternetConnectivity(ctx context.Context, client *http.Client, _ logging.Logger) TestResult {
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
	// Ignore error from resp.Body.Close().
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		return TestResult{
			Name:        "Internet Connectivity",
			Success:     false,
			Error:       errors.Errorf("HTTP status code: %d", resp.StatusCode),
			Description: "Received error status code from internet test",
			Duration:    duration,
		}
	}

	return TestResult{
		Name:        "Internet Connectivity",
		Success:     true,
		Error:       nil,
		Description: fmt.Sprintf("Connected to %s (HTTP %d)", internetTestURL, resp.StatusCode),
		Duration:    duration,
	}
}

// testRTMAvailability checks if the RTM API endpoint is reachable.
func testRTMAvailability(ctx context.Context, client *http.Client, _ logging.Logger) TestResult {
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
	// Ignore error from resp.Body.Close().
	defer func() {
		_ = resp.Body.Close()
	}()

	return TestResult{
		Name:        "RTM API Availability",
		Success:     true,
		Error:       nil,
		Description: fmt.Sprintf("RTM API endpoint is reachable (HTTP %d)", resp.StatusCode),
		Duration:    duration,
	}
}

// testRTMEcho tests the rtm.test.echo method (doesn't require authentication).
func testRTMEcho(ctx context.Context, client *rtm.Client, _ logging.Logger) TestResult {
	start := time.Now()

	// Use a simple test parameter - the API will echo it back.
	params := map[string]string{"test_param": "hello_rtm"}
	respBytes, err := client.CallMethod(ctx, "rtm.test.echo", params)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Name:        "RTM API Echo Test",
			Success:     false,
			Error:       err,
			Description: "Failed to call rtm.test.echo method - API key or secret may be invalid",
			Duration:    duration,
		}
	}

	// Check that we received a valid response with "stat":"ok".
	respStr := string(respBytes)
	if !strings.Contains(respStr, `"stat":"ok"`) {
		return TestResult{
			Name:        "RTM API Echo Test",
			Success:     false,
			Error:       errors.New("response doesn't contain success status"),
			Description: "API returned an invalid response format",
			Duration:    duration,
		}
	}

	// Don't look for specific parameters, as the API might format them differently.
	// Just confirm it's a valid OK response.
	return TestResult{
		Name:        "RTM API Echo Test",
		Success:     true,
		Description: "Successfully verified API key and secret are valid",
		Duration:    duration,
	}
}

// startRTMAuth starts the RTM authentication flow.
func startRTMAuth(ctx context.Context, client *rtm.Client, _ logging.Logger) TestResult {
	start := time.Now()

	authURL, _, err := client.StartAuthFlow(ctx)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Name:        "RTM Auth Flow Start",
			Success:     false,
			Error:       err,
			Description: "Failed to start authentication flow",
			Duration:    duration,
		}
	}

	description := fmt.Sprintf("Auth flow started. Please visit this URL to authorize: %s", authURL)

	return TestResult{
		Name:        "RTM Auth Flow Start",
		Success:     true,
		Error:       nil,
		Description: description,
		Duration:    duration,
	}
}

// completeRTMAuth completes the RTM authentication flow with a frob.
func completeRTMAuth(ctx context.Context, client *rtm.Client, frob string, _ logging.Logger) TestResult {
	start := time.Now()

	_, err := client.CompleteAuthFlow(ctx, frob)
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Name:        "RTM Auth Flow Completion",
			Success:     false,
			Error:       err,
			Description: "Failed to complete authentication flow",
			Duration:    duration,
		}
	}

	// Re-verify auth state immediately after completion.
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

	description := fmt.Sprintf("Successfully authenticated as %s", authState.Username)

	return TestResult{
		Name:        "RTM Auth Flow Completion",
		Success:     true,
		Error:       nil,
		Description: description,
		Duration:    duration,
	}
}

// testRTMAuthenticated tests an authenticated RTM API method.
func testRTMAuthenticated(ctx context.Context, client *rtm.Client, _ logging.Logger, authState *rtm.AuthState) TestResult {
	if authState == nil || !authState.IsAuthenticated {
		return TestResult{
			Name:        "RTM Authenticated API",
			Success:     false,
			Description: "Cannot run test, not authenticated",
		}
	}

	start := time.Now()

	lists, err := client.GetLists(ctx) // Try to get lists.
	duration := time.Since(start)

	if err != nil {
		return TestResult{
			Name:        "RTM Authenticated API",
			Success:     false,
			Error:       err,
			Description: "Failed to call authenticated API method (rtm.lists.getList)",
			Duration:    duration,
		}
	}

	description := fmt.Sprintf("Successfully retrieved %d lists", len(lists))

	// Log the first few lists for verification.
	maxListsToLog := 3
	listInfo := ""
	for i, list := range lists {
		if i >= maxListsToLog {
			listInfo += fmt.Sprintf("... and %d more lists.", len(lists)-maxListsToLog)
			break
		}
		if i > 0 {
			listInfo += ", "
		}
		listInfo += list.Name
	}
	if listInfo != "" {
		description += ": " + listInfo
	}

	return TestResult{
		Name:        "RTM Authenticated API",
		Success:     true,
		Error:       nil,
		Description: description,
		Duration:    duration,
	}
}
