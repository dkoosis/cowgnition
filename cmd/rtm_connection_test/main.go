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
	"github.com/dkoosis/cowgnition/internal/rtm" // Import rtm package.
)

const (
	internetTestURL = "https://www.google.com"
	// rtmAPIEndpoint is no longer needed here, client gets it from config/defaults.
	testTimeout  = 90 * time.Second // Increased timeout for interactive auth.
	callbackPort = 8090             // Default port for RTM callback.
)

// Flags holds the parsed command-line flags.
// Removed frobArg as AuthManager handles it.
type Flags struct {
	configPath   string
	verbose      bool
	skipInternet bool
	skipNonAuth  bool
	skipAuth     bool
	forceAuth    bool // New flag to force re-authentication.
	clearAuth    bool // New flag to clear existing auth.
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
	logger := logging.GetLogger("rtm_conn_test") // Use named logger.

	// Check API key and shared secret availability.
	if cfg.RTM.APIKey == "" || cfg.RTM.SharedSecret == "" {
		fmt.Println("\n❌ ERROR: RTM API key and shared secret are required.")
		fmt.Println("Set them in the config file or via environment variables:")
		fmt.Println("  - RTM_API_KEY")
		fmt.Println("  - RTM_SHARED_SECRET")
		os.Exit(1)
	} else {
		logger.Info("RTM API credentials found.",
			"api_key_source", determineSource(cfg.RTM.APIKey, os.Getenv("RTM_API_KEY")),
			"secret_source", determineSource(cfg.RTM.SharedSecret, os.Getenv("RTM_SHARED_SECRET")))
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create RTM Service (needed for AuthManager and token storage).
	// Use a factory to ensure consistent setup.
	rtmFactory := rtm.NewServiceFactory(cfg, logger)
	// Create service, handling potential initialization errors (like token storage setup).
	rtmService, err := rtmFactory.CreateService(ctx)
	if err != nil {
		// Log the specific error from service creation.
		logger.Error("Failed to create or initialize RTM Service.", "error", fmt.Sprintf("%+v", err))
		fmt.Printf("\n❌ ERROR: Failed to initialize RTM Service: %v\n", err)
		fmt.Println("Check permissions for token storage (keyring or config directory) and configuration.")
		os.Exit(1)
	}
	// Service created, now run tests.
	results := runConnectionTests(ctx, cfg, flags, logger, rtmService)
	printResultsSummary(results) // Pass results to summary function.
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

// parseFlagsAndSetupLogging parses command-line flags and initializes logging.
func parseFlagsAndSetupLogging() Flags {
	var flags Flags
	flag.StringVar(&flags.configPath, "config", "", "Path to configuration file")
	flag.BoolVar(&flags.verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&flags.skipInternet, "skip-internet", false, "Skip internet connectivity test")
	flag.BoolVar(&flags.skipNonAuth, "skip-nonauth", false, "Skip non-authenticated RTM API test")
	flag.BoolVar(&flags.skipAuth, "skip-auth", false, "Skip authenticated RTM API test")
	flag.BoolVar(&flags.forceAuth, "force-auth", false, "Force interactive authentication even if already authenticated")
	flag.BoolVar(&flags.clearAuth, "clear-auth", false, "Clear existing authentication token before running tests")
	// Removed frob flag.
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
	// Apply environment overrides explicitly if needed, though DefaultConfig does this.
	// rtm.applyEnvironmentOverrides(cfg, logging.GetLogger("config")).
	return cfg
}

// runConnectionTests executes the suite of connectivity and RTM tests.
// Now accepts rtmService.
func runConnectionTests(ctx context.Context, cfg *config.Config, flags Flags, logger logging.Logger, rtmService *rtm.Service) []TestResult {
	results := make([]TestResult, 0)
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Test 1: Internet connectivity.
	if !flags.skipInternet {
		results = append(results, testInternetConnectivity(ctx, httpClient, logger))
	}

	// Test 2: RTM API availability (non-authenticated).
	rtmAPIEndpoint := rtmService.GetClientAPIEndpoint() // Get endpoint from service/client.
	if !flags.skipNonAuth {
		results = append(results, testRTMAvailability(ctx, httpClient, logger, rtmAPIEndpoint))
	}

	// Use the RTM client from the service.
	client := rtmService.GetClient()

	// Test 3: RTM API echo test (non-authenticated method).
	if !flags.skipNonAuth {
		results = append(results, testRTMEcho(ctx, client, logger))
	}

	// Handle --clear-auth flag first.
	if flags.clearAuth {
		logger.Info("Clearing existing authentication token due to --clear-auth flag.")
		err := rtmService.ClearAuth()
		if err != nil {
			results = append(results, TestResult{
				Name:        "Clear Authentication",
				Success:     false,
				Error:       err,
				Description: "Failed to clear existing RTM authentication token",
			})
			// Decide whether to stop or continue based on the error.
			// For now, let's continue to the auth check/attempt.
		} else {
			results = append(results, TestResult{
				Name:        "Clear Authentication",
				Success:     true,
				Description: "Successfully cleared existing RTM authentication token",
			})
		}
	}

	// Test 4: RTM Authentication (using AuthManager).
	if !flags.skipAuth {
		results = append(results, handleAuthenticationTestWithManager(ctx, flags, logger, rtmService)...)
	}

	return results
}

// handleAuthenticationTestWithManager checks auth state and uses AuthManager if needed.
func handleAuthenticationTestWithManager(ctx context.Context, flags Flags, logger logging.Logger, rtmService *rtm.Service) []TestResult {
	results := make([]TestResult, 0)
	start := time.Now()

	// Get initial auth state.
	initialAuthState, err := rtmService.GetAuthState(ctx)
	if err != nil {
		results = append(results, TestResult{
			Name:        "RTM Auth Pre-Check",
			Success:     false,
			Error:       err,
			Description: "Failed to check initial authentication state",
			Duration:    time.Since(start),
		})
		// If we can't even check the state, maybe don't proceed with EnsureAuthenticated?
		// For now, we let EnsureAuthenticated handle it.
	}

	needsAuth := !initialAuthState.IsAuthenticated || flags.forceAuth
	authActionDescription := ""

	if needsAuth {
		authActionDescription = "Authentication required (not authenticated or --force-auth used)."
		logger.Info(authActionDescription)

		// Configure and run AuthManager.
		authOptions := rtm.DefaultAuthManagerOptions()
		authOptions.AutoCompleteAuth = true // Enable automatic completion via callback.
		authOptions.Mode = rtm.AuthModeInteractive
		authOptions.CallbackPort = callbackPort // Use defined port.
		// Let AuthManager use the default timeout from context unless overridden.
		// authOptions.TimeoutDuration = testTimeout

		authManager := rtm.NewAuthManager(rtmService, authOptions, logger)
		defer authManager.Shutdown() // Ensure callback server is stopped.

		authResult, authErr := authManager.EnsureAuthenticated(ctx)

		duration := time.Since(start) // Recalculate duration for the whole process.

		if authErr != nil || !authResult.Success {
			errMsg := "Failed to authenticate using AuthManager."
			finalErr := authErr
			if authResult != nil && authResult.Error != nil {
				errMsg = fmt.Sprintf("AuthManager process failed: %v", authResult.Error)
				finalErr = authResult.Error // Prefer the error from the result struct.
			} else if authErr != nil {
				errMsg = fmt.Sprintf("AuthManager process failed: %v", authErr)
			}

			results = append(results, TestResult{
				Name:        "RTM Authentication Attempt",
				Success:     false,
				Error:       finalErr,
				Description: errMsg,
				Duration:    duration,
			})
			// If auth failed, we cannot run the authenticated API test.
			results = append(results, TestResult{
				Name:        "RTM Authenticated API",
				Success:     false,
				Description: "Skipped due to authentication failure",
			})
			return results // Stop here if auth failed.
		}

		// Auth succeeded via AuthManager.
		results = append(results, TestResult{
			Name:        "RTM Authentication Attempt",
			Success:     true,
			Description: fmt.Sprintf("Successfully authenticated as %s via AuthManager.", authResult.Username),
			Duration:    duration,
		})
		// Now run the authenticated test.
		results = append(results, testRTMAuthenticated(ctx, rtmService.GetClient(), logger, rtmService.GetUsername()))

	} else {
		// Already authenticated and not forced.
		authActionDescription = fmt.Sprintf("Already authenticated as %s. Use --force-auth to re-authenticate.", initialAuthState.Username)
		results = append(results, TestResult{
			Name:        "RTM Authentication Check",
			Success:     true,
			Description: authActionDescription,
			Duration:    time.Since(start),
		})
		// Run the authenticated test.
		results = append(results, testRTMAuthenticated(ctx, rtmService.GetClient(), logger, initialAuthState.Username))
	}

	return results
}

// printResultsSummary prints the final test results.
// No longer needs to print manual frob instructions.
func printResultsSummary(results []TestResult) {
	fmt.Println("\n=== RTM Connection Test Results ===")
	allSuccess := true
	authFailed := false
	authFailureMsg := ""

	for _, result := range results {
		statusMark := "✓"
		if !result.Success {
			statusMark = "✗"
			allSuccess = false
			if strings.Contains(result.Name, "Authentication") {
				authFailed = true
				if result.Error != nil {
					authFailureMsg = result.Error.Error()
				} else {
					authFailureMsg = result.Description
				}
			}
		}

		fmt.Printf("%s %s (%.2fs)\n", statusMark, result.Name, result.Duration.Seconds())
		fmt.Printf("   %s\n", result.Description)

		if result.Error != nil {
			fmt.Printf("   Error: %v\n", result.Error)
		}
	}

	fmt.Println("\n=== Summary ===")
	if allSuccess {
		fmt.Println("✅ All tests passed successfully.")
	} else {
		fmt.Printf("❌ One or more tests failed.")
		if authFailed {
			fmt.Printf(" Authentication Error: %s", authFailureMsg)
		}
		fmt.Println() // Newline after failure message.
		os.Exit(1)
	}
}

// testInternetConnectivity checks if the internet is reachable.
func testInternetConnectivity(ctx context.Context, client *http.Client, logger logging.Logger) TestResult {
	start := time.Now()
	logger.Debug("Testing internet connectivity...")

	req, err := http.NewRequestWithContext(ctx, "HEAD", internetTestURL, nil)
	if err != nil {
		return TestResult{
			Name: "Internet Connectivity", Success: false, Error: errors.Wrap(err, "failed to create request"),
			Description: "Failed to create request", Duration: time.Since(start),
		}
	}

	resp, err := client.Do(req)
	duration := time.Since(start)
	if err != nil {
		logger.Warn("Internet connectivity check failed.", "error", err)
		return TestResult{
			Name: "Internet Connectivity", Success: false, Error: errors.Wrap(err, "HTTP HEAD request failed"),
			Description: "Failed to connect to the internet", Duration: duration,
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		logger.Warn("Internet connectivity check received non-success status.", "status_code", resp.StatusCode)
		return TestResult{
			Name: "Internet Connectivity", Success: false, Error: errors.Errorf("HTTP status code: %d", resp.StatusCode),
			Description: "Received error status code from internet test", Duration: duration,
		}
	}

	logger.Debug("Internet connectivity check passed.", "status_code", resp.StatusCode)
	return TestResult{
		Name: "Internet Connectivity", Success: true, Error: nil,
		Description: fmt.Sprintf("Connected to %s (HTTP %d)", internetTestURL, resp.StatusCode), Duration: duration,
	}
}

// testRTMAvailability checks if the RTM API endpoint is reachable.
// Now accepts apiEndpoint as parameter.
func testRTMAvailability(ctx context.Context, client *http.Client, logger logging.Logger, apiEndpoint string) TestResult {
	start := time.Now()
	logger.Debug("Testing RTM API endpoint availability...", "endpoint", apiEndpoint)

	req, err := http.NewRequestWithContext(ctx, "HEAD", apiEndpoint, nil)
	if err != nil {
		return TestResult{
			Name: "RTM API Availability", Success: false, Error: errors.Wrap(err, "failed to create request"),
			Description: "Failed to create request", Duration: time.Since(start),
		}
	}

	resp, err := client.Do(req)
	duration := time.Since(start)
	if err != nil {
		// RTM endpoint might not support HEAD, so treat network errors as failure, but allow 4xx/5xx status.
		logger.Warn("RTM API availability check failed (network error).", "error", err)
		return TestResult{
			Name: "RTM API Availability", Success: false, Error: errors.Wrap(err, "HTTP HEAD request failed"),
			Description: "Failed to connect to RTM API endpoint", Duration: duration,
		}
	}
	defer func() { _ = resp.Body.Close() }()

	// Log status but consider reachable even if not 200 OK.
	logger.Debug("RTM API availability check completed.", "status_code", resp.StatusCode)
	return TestResult{
		Name: "RTM API Availability", Success: true, Error: nil, // Success means reachable.
		Description: fmt.Sprintf("RTM API endpoint is reachable (HTTP %d)", resp.StatusCode), Duration: duration,
	}
}

// testRTMEcho tests the rtm.test.echo method (doesn't require authentication).
func testRTMEcho(ctx context.Context, client *rtm.Client, logger logging.Logger) TestResult {
	start := time.Now()
	logger.Debug("Testing RTM API echo...")

	params := map[string]string{"test_param": "hello_rtm_conn_test"}
	respBytes, err := client.CallMethod(ctx, "rtm.test.echo", params) // Use client directly.
	duration := time.Since(start)

	if err != nil {
		logger.Warn("RTM API echo test failed.", "error", err)
		return TestResult{
			Name: "RTM API Echo Test", Success: false, Error: err,
			Description: "Failed to call rtm.test.echo method - API key or secret may be invalid", Duration: duration,
		}
	}

	respStr := string(respBytes)
	// RTM echo includes params under rsp key.
	if !strings.Contains(respStr, `"stat":"ok"`) || !strings.Contains(respStr, `"test_param":"hello_rtm_conn_test"`) {
		logger.Warn("RTM API echo test received invalid response.", "response", respStr)
		return TestResult{
			Name: "RTM API Echo Test", Success: false, Error: errors.New("response doesn't contain success status or echoed param"),
			Description: "API returned an invalid response format", Duration: duration,
		}
	}

	logger.Debug("RTM API echo test passed.")
	return TestResult{
		Name: "RTM API Echo Test", Success: true,
		Description: "Successfully verified API key and secret are valid", Duration: duration,
	}
}

// testRTMAuthenticated tests an authenticated RTM API method.
// Now accepts username for description.
func testRTMAuthenticated(ctx context.Context, client *rtm.Client, logger logging.Logger, username string) TestResult {
	if username == "" { // Check if username is valid.
		return TestResult{
			Name: "RTM Authenticated API", Success: false,
			Description: "Cannot run test, not authenticated",
		}
	}

	start := time.Now()
	logger.Debug("Testing authenticated RTM API call (GetLists)...", "user", username)

	lists, err := client.GetLists(ctx) // Try to get lists.
	duration := time.Since(start)

	if err != nil {
		logger.Warn("Authenticated RTM API call failed.", "error", err)
		return TestResult{
			Name: "RTM Authenticated API", Success: false, Error: err,
			Description: "Failed to call authenticated API method (rtm.lists.getList)", Duration: duration,
		}
	}

	description := fmt.Sprintf("Successfully retrieved %d lists", len(lists))
	logger.Debug("Authenticated RTM API call successful.", "list_count", len(lists))

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
		Name: "RTM Authenticated API", Success: true, Error: nil,
		Description: description, Duration: duration,
	}
}

// Removed startRTMAuth and completeRTMAuth as AuthManager handles the flow now.
