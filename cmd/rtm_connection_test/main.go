// file: cmd/rtm_connection_test/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url" // Import url package
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/rtm" // Import rtm package.
)

const (
	internetTestURL = "https://www.google.com"
	// rtmAPIEndpoint is no longer needed here, client gets it from config/defaults.
	testTimeout = 30 * time.Second
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
	// Add extra fields for token storage information
	StorageMethod string
	StoragePath   string
}

func main() {
	flags := parseFlagsAndSetupLogging()
	cfg := loadConfiguration(flags.configPath)

	// Setup logging based on flags BEFORE using the logger.
	logLevel := "info"
	if flags.verbose {
		logLevel = "debug"
	}
	logging.SetupDefaultLogger(logLevel)
	logger := logging.GetLogger("rtm_conn_test") // Get logger after setup.

	// Check API key and shared secret availability.
	if cfg.RTM.APIKey == "" || cfg.RTM.SharedSecret == "" {
		fmt.Println("\n❌ ERROR: RTM API key and shared secret are required.")
		fmt.Println("Set them in the config file or via environment variables:")
		fmt.Println("  - RTM_API_KEY")
		fmt.Println("  - RTM_SHARED_SECRET")
		os.Exit(1)
	}
	// Use determineSource here.
	logger.Info("RTM API credentials found.",
		"api_key_source", determineSource(cfg.RTM.APIKey, os.Getenv("RTM_API_KEY")),
		"secret_source", determineSource(cfg.RTM.SharedSecret, os.Getenv("RTM_SHARED_SECRET")))

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Pass logger to runConnectionTests.
	results := runConnectionTests(ctx, cfg, flags, logger)
	printResultsSummary(results)
}

// determineSource helps log where credentials came from.
func determineSource(configVal, envVal string) string {
	if envVal != "" {
		return "environment variable"
	}
	if configVal != "" {
		return "config file"
	}
	return "not found"
}

// parseFlagsAndSetupLogging parses command-line flags. Logging setup moved to main.
func parseFlagsAndSetupLogging() Flags {
	var flags Flags
	flag.StringVar(&flags.configPath, "config", "", "Path to configuration file")
	flag.BoolVar(&flags.verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&flags.skipInternet, "skip-internet", false, "Skip internet connectivity test")
	flag.BoolVar(&flags.skipNonAuth, "skip-nonauth", false, "Skip non-authenticated RTM API test")
	flag.BoolVar(&flags.skipAuth, "skip-auth", false, "Skip authenticated RTM API test")
	flag.StringVar(&flags.frobArg, "frob", "", "Frob to use for authentication completion")
	flag.Parse()
	// Logging setup moved to main()
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
	// Assuming DefaultConfig or LoadFromFile handles env overrides sufficiently.
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

	// Create RTM client for API tests.
	// Pass logger to factory.
	factory := rtm.NewServiceFactory(cfg, logger) // Use ServiceFactory to ensure consistent client creation.
	rtmService, err := factory.CreateService(ctx) // Create and initialize the service instead of just the client
	if err != nil {
		logger.Error("Failed to create RTM service.", "error", err)
		results = append(results, TestResult{
			Name:        "RTM Service Initialization",
			Success:     false,
			Error:       err,
			Description: "Failed to create and initialize RTM service",
			Duration:    0,
		})
		return results
	}

	// Get token storage information
	storageMethod, storagePath, storageAvailable := rtmService.GetTokenStorageInfo()
	results = append(results, TestResult{
		Name:          "RTM Token Storage",
		Success:       storageAvailable,
		Error:         nil,
		Description:   fmt.Sprintf("Using %s storage: %s", storageMethod, storagePath),
		Duration:      0,
		StorageMethod: storageMethod,
		StoragePath:   storagePath,
	})

	// Test 2: RTM API availability (non-authenticated).
	if !flags.skipNonAuth {
		results = append(results, testRTMAvailability(ctx, httpClient, rtmService.GetClient(), logger)) // Pass client for endpoint.
	}

	// Test 3: RTM API echo test (non-authenticated method).
	if !flags.skipNonAuth {
		results = append(results, testRTMEcho(ctx, rtmService.GetClient(), logger))
	}

	// Test 4: RTM Authentication.
	if !flags.skipAuth {
		// Get current authentication state
		initialAuthState, _ := rtmService.GetAuthState(ctx)
		isAuthenticated := initialAuthState != nil && initialAuthState.IsAuthenticated

		// Add auth status test result
		authStatusResult := TestResult{
			Name:     "RTM Initial Auth Check",
			Success:  isAuthenticated,
			Duration: 0,
		}

		if isAuthenticated {
			authStatusResult.Description = fmt.Sprintf(
				"Already authenticated as %s", initialAuthState.Username)
		} else {
			authStatusResult.Description = "Not authenticated"
		}

		results = append(results, authStatusResult)

		// If not authenticated and have frob, attempt to complete auth
		if !isAuthenticated && flags.frobArg != "" {
			authCompletionResult := completeRTMAuth(ctx, rtmService, flags.frobArg, logger)
			results = append(results, authCompletionResult)

			// Check if auth succeeded
			if authCompletionResult.Success {
				// Run authenticated test
				newAuthState, _ := rtmService.GetAuthState(ctx)
				results = append(results, testRTMAuthenticated(ctx, rtmService, logger, newAuthState))
			}
		} else if !isAuthenticated {
			// Start auth flow for manual completion
			results = append(results, startRTMAuth(ctx, rtmService.GetClient(), logger))
		} else {
			// Already authenticated, run authenticated test
			results = append(results, testRTMAuthenticated(ctx, rtmService, logger, initialAuthState))
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
	var storageMethod, storagePath string

	for _, result := range results {
		statusMark := "✓"
		if !result.Success {
			statusMark = "✗"
			allSuccess = false
		}

		fmt.Printf("%s %s (%.2fs)\n", statusMark, result.Name, result.Duration.Seconds())
		fmt.Printf("   %s\n", result.Description)

		if result.Error != nil {
			// Use %+v for detailed error with stack trace if available
			fmt.Printf("   Error: %+v\n", result.Error)
		}

		// Extract auth URL and frob if available.
		if result.Name == "RTM Auth Flow Start" && result.Success {
			authNeeded = true
			desc := result.Description
			// Improved URL extraction logic
			urlPrefix := "https://"
			if urlStart := strings.Index(desc, urlPrefix); urlStart != -1 {
				// Find the end of the URL (space or end of string)
				urlEnd := strings.Index(desc[urlStart:], " ")
				if urlEnd == -1 {
					authURL = desc[urlStart:]
				} else {
					authURL = desc[urlStart : urlStart+urlEnd]
				}

				// Improved frob extraction logic
				if strings.Contains(authURL, "frob=") {
					parsedURL, err := url.Parse(authURL)
					if err == nil {
						frob = parsedURL.Query().Get("frob")
					}
				}
			}
		}

		// Record token storage info
		if result.Name == "RTM Token Storage" {
			storageMethod = result.StorageMethod
			storagePath = result.StoragePath
		}
	}

	fmt.Println("\n=== Storage Information ===")
	if storageMethod != "" {
		fmt.Printf("Token Storage Method: %s\n", storageMethod)
		if storagePath != "" {
			fmt.Printf("Token Storage Location: %s\n", storagePath)
		}

		switch storageMethod {
		case "secure":
			fmt.Println("Using OS keychain/credential manager for secure token storage.")
		case "file":
			fmt.Printf("Using file storage at: %s\n", storagePath)
			fmt.Println("Note: File storage is less secure than OS keychain.")
		case "none":
			fmt.Println("Warning: No token storage available. Authentication will not persist between sessions.")
		}
	} else {
		fmt.Println("Token storage information not available.")
	}

	fmt.Println("\n=== Summary ===")
	// Check if auth failed specifically by looking for "RTM Auth Flow Completion" failure
	authFailed := false
	authFailureMsg := ""
	for _, result := range results {
		if result.Name == "RTM Auth Flow Completion" && !result.Success {
			authFailed = true
			if result.Error != nil {
				authFailureMsg = result.Error.Error()
			} else {
				authFailureMsg = result.Description
			}
			break
		}
	}

	if !allSuccess && authNeeded && !authFailed {
		// Only show instructions if overall tests failed AND auth was needed AND auth completion didn't fail
		fmt.Println("⚠️  Authentication needed")
		fmt.Println("\nTo complete authentication:")
		if authURL != "" {
			fmt.Println("1. Open this URL in your browser:")
			fmt.Println("   " + authURL)
		} else {
			fmt.Println("1. (Could not extract Auth URL)")
		}
		fmt.Println("2. Authorize CowGnition in your RTM account")
		if frob != "" {
			fmt.Println("3. Run this command:")
			fmt.Println("   go run ./cmd/rtm_connection_test --frob=" + frob)
		} else {
			fmt.Println("3. (Could not extract Frob for command)")
		}
	} else if allSuccess {
		fmt.Println("✅ All tests passed successfully.")
	} else {
		fmt.Printf("❌ One or more tests failed.")
		if authFailed {
			fmt.Printf(" Authentication Error: %s", authFailureMsg)
		}
		fmt.Println() // Newline after failure message
		os.Exit(1)
	}
}

// testInternetConnectivity checks if the internet is reachable.
func testInternetConnectivity(ctx context.Context, client *http.Client, logger logging.Logger) TestResult {
	start := time.Now()
	logger.Debug("Testing internet connectivity...")

	// Ensure ctx is passed
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
		logger.Warn("Internet connectivity check failed.", "error", err)
		return TestResult{
			Name:        "Internet Connectivity",
			Success:     false,
			Error:       errors.Wrap(err, "HTTP HEAD request failed"),
			Description: "Failed to connect to the internet",
			Duration:    duration,
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		logger.Warn("Internet connectivity check received non-success status.", "status_code", resp.StatusCode)
		return TestResult{
			Name:        "Internet Connectivity",
			Success:     false,
			Error:       errors.Errorf("HTTP status code: %d", resp.StatusCode),
			Description: "Received error status code from internet test",
			Duration:    duration,
		}
	}

	logger.Debug("Internet connectivity check passed.", "status_code", resp.StatusCode)
	return TestResult{
		Name:        "Internet Connectivity",
		Success:     true,
		Error:       nil,
		Description: fmt.Sprintf("Connected to %s (HTTP %d)", internetTestURL, resp.StatusCode),
		Duration:    duration,
	}
}

// testRTMAvailability checks if the RTM API endpoint is reachable.
func testRTMAvailability(ctx context.Context, httpClient *http.Client, rtmClient *rtm.Client, logger logging.Logger) TestResult {
	start := time.Now()
	apiEndpoint := rtmClient.GetAPIEndpoint() // Get endpoint from the client
	logger.Debug("Testing RTM API endpoint availability...", "endpoint", apiEndpoint)

	// Ensure ctx is passed
	req, err := http.NewRequestWithContext(ctx, "HEAD", apiEndpoint, nil)
	if err != nil {
		return TestResult{
			Name:        "RTM API Availability",
			Success:     false,
			Error:       errors.Wrap(err, "failed to create request"),
			Description: "Failed to create request",
			Duration:    time.Since(start),
		}
	}

	resp, err := httpClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		logger.Warn("RTM API availability check failed (network error).", "error", err)
		return TestResult{
			Name:        "RTM API Availability",
			Success:     false,
			Error:       errors.Wrap(err, "HTTP HEAD request failed"),
			Description: "Failed to connect to RTM API endpoint",
			Duration:    duration,
		}
	}
	defer func() { _ = resp.Body.Close() }()

	logger.Debug("RTM API availability check completed.", "status_code", resp.StatusCode)
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
	start := time.Now()
	logger.Debug("Testing RTM API echo...")

	params := map[string]string{"test_param": "hello_rtm_conn_test"}
	// Ensure ctx is passed
	respBytes, err := client.CallMethod(ctx, "rtm.test.echo", params)
	duration := time.Since(start)

	if err != nil {
		logger.Warn("RTM API echo test failed.", "error", err)
		return TestResult{
			Name:        "RTM API Echo Test",
			Success:     false,
			Error:       err,
			Description: "Failed to call rtm.test.echo method - API key or secret may be invalid",
			Duration:    duration,
		}
	}

	respStr := string(respBytes)
	if !strings.Contains(respStr, `"stat":"ok"`) || !strings.Contains(respStr, `"test_param":"hello_rtm_conn_test"`) {
		logger.Warn("RTM API echo test received invalid response.", "response", respStr)
		return TestResult{
			Name:        "RTM API Echo Test",
			Success:     false,
			Error:       errors.New("response doesn't contain success status or echoed param"),
			Description: "API returned an invalid response format",
			Duration:    duration,
		}
	}

	logger.Debug("RTM API echo test passed.")
	return TestResult{
		Name:        "RTM API Echo Test",
		Success:     true,
		Description: "Successfully verified API key and secret are valid",
		Duration:    duration,
	}
}

// startRTMAuth starts the RTM authentication flow.
func startRTMAuth(ctx context.Context, client *rtm.Client, logger logging.Logger) TestResult {
	start := time.Now()
	logger.Debug("Starting RTM auth flow...")

	// Ensure ctx is passed
	authURL, _, err := client.StartAuthFlow(ctx)
	duration := time.Since(start)

	if err != nil {
		logger.Warn("Starting RTM auth flow failed.", "error", err)
		return TestResult{
			Name:        "RTM Auth Flow Start",
			Success:     false,
			Error:       err,
			Description: "Failed to start authentication flow",
			Duration:    duration,
		}
	}

	description := fmt.Sprintf("Auth flow started. Please visit this URL to authorize: %s", authURL)
	logger.Debug("RTM Auth flow started successfully.")
	return TestResult{
		Name:        "RTM Auth Flow Start",
		Success:     true,
		Error:       nil,
		Description: description,
		Duration:    duration,
	}
}

// completeRTMAuth completes the RTM authentication flow with a frob.
func completeRTMAuth(ctx context.Context, service *rtm.Service, frob string, logger logging.Logger) TestResult {
	start := time.Now()
	logger.Debug("Completing RTM auth flow...", "frob", frob) // Log frob for debugging

	// Use service.CompleteAuth instead of client.CompleteAuthFlow
	err := service.CompleteAuth(ctx, frob)
	duration := time.Since(start)

	if err != nil {
		logger.Warn("Completing RTM auth flow failed.", "error", err)
		return TestResult{
			Name:        "RTM Auth Flow Completion",
			Success:     false,
			Error:       err,
			Description: "Failed to complete authentication flow",
			Duration:    duration,
		}
	}

	// Re-verify auth state immediately after completion.
	// Use service.GetAuthState instead of client.GetAuthState
	authState, verifyErr := service.GetAuthState(ctx)
	if verifyErr != nil || authState == nil || !authState.IsAuthenticated { // Check authState for nil
		logger.Warn("RTM auth verification failed after completion.", "verifyError", verifyErr, "isAuthenticated", authState != nil && authState.IsAuthenticated)
		return TestResult{
			Name:        "RTM Auth Flow Completion",
			Success:     false,
			Error:       verifyErr, // Return the verification error if any
			Description: "Authentication completed, but verification failed",
			Duration:    duration,
		}
	}

	// Get storage information
	storageMethod, storagePath, _ := service.GetTokenStorageInfo()
	storageInfo := ""
	switch storageMethod {
	case "secure":
		storageInfo = " (Token saved to OS keychain)"
	case "file":
		storageInfo = fmt.Sprintf(" (Token saved to file: %s)", storagePath)
	default:
		storageInfo = ""
	}

	description := fmt.Sprintf("Successfully authenticated as %s%s", authState.Username, storageInfo)
	logger.Debug("RTM Auth flow completed and verified successfully.", "user", authState.Username)
	return TestResult{
		Name:          "RTM Auth Flow Completion",
		Success:       true,
		Error:         nil,
		Description:   description,
		Duration:      duration,
		StorageMethod: storageMethod,
		StoragePath:   storagePath,
	}
}

// testRTMAuthenticated tests an authenticated RTM API method.
func testRTMAuthenticated(ctx context.Context, service *rtm.Service, logger logging.Logger, authState *rtm.AuthState) TestResult {
	if authState == nil || !authState.IsAuthenticated {
		return TestResult{
			Name:        "RTM Authenticated API",
			Success:     false,
			Description: "Cannot run test, not authenticated",
		}
	}

	start := time.Now()
	logger.Debug("Testing authenticated RTM API call (GetLists)...", "user", authState.Username)

	// Use client from service
	lists, err := service.GetClient().GetLists(ctx)
	duration := time.Since(start)

	if err != nil {
		logger.Warn("Authenticated RTM API call (GetLists) failed.", "error", err)
		return TestResult{
			Name:        "RTM Authenticated API",
			Success:     false,
			Error:       err,
			Description: "Failed to call authenticated API method (rtm.lists.getList)",
			Duration:    duration,
		}
	}

	description := fmt.Sprintf("Successfully retrieved %d lists", len(lists))
	logger.Debug("Authenticated RTM API call successful.", "list_count", len(lists))

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

	// Add storage information
	storageMethod, storagePath, _ := service.GetTokenStorageInfo()

	return TestResult{
		Name:          "RTM Authenticated API",
		Success:       true,
		Error:         nil,
		Description:   description,
		Duration:      duration,
		StorageMethod: storageMethod,
		StoragePath:   storagePath,
	}
}
