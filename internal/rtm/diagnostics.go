// file: internal/rtm/diagnostics.go
package rtm

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// DiagnosticResult represents the outcome of a diagnostic test.
type DiagnosticResult struct {
	Name        string        // Name of the test
	Success     bool          // Whether the test was successful
	Error       error         // Error if the test failed
	Description string        // Human-readable description of the result
	Duration    time.Duration // How long the test took to execute
}

// ConnectivityCheckOptions defines which diagnostic checks to perform.
type ConnectivityCheckOptions struct {
	CheckInternet   bool   // Whether to check internet connectivity
	CheckRTMAPI     bool   // Whether to check RTM API availability
	CheckAPIKey     bool   // Whether to validate API key via echo test
	CheckAuth       bool   // Whether to check authentication status
	RequireAuth     bool   // Whether authentication is required
	InternetTestURL string // URL to use for internet connectivity test
}

// DefaultConnectivityCheckOptions returns default options for connectivity checks.
func DefaultConnectivityCheckOptions() ConnectivityCheckOptions {
	return ConnectivityCheckOptions{
		CheckInternet:   true,
		CheckRTMAPI:     true,
		CheckAPIKey:     true,
		CheckAuth:       true,
		RequireAuth:     false,
		InternetTestURL: "https://www.google.com",
	}
}

// PerformConnectivityCheck runs a series of diagnostic tests to verify RTM connectivity.
// It returns the results of each test and an error if any critical test fails.
func (s *Service) PerformConnectivityCheck(ctx context.Context, options ConnectivityCheckOptions) ([]DiagnosticResult, error) {
	results := make([]DiagnosticResult, 0)
	var criticalError error

	// Create an HTTP client for tests
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Step 1: Check Internet connectivity if requested
	if options.CheckInternet {
		result := s.checkInternetConnectivity(ctx, httpClient, options.InternetTestURL)
		results = append(results, result)
		if !result.Success && criticalError == nil {
			criticalError = errors.Wrap(result.Error, "internet connectivity check failed")
		}
	}

	// Step 2: Check RTM API availability if requested and previous checks passed
	if options.CheckRTMAPI && (criticalError == nil || !options.CheckInternet) {
		result := s.checkRTMAvailability(ctx, httpClient)
		results = append(results, result)
		if !result.Success && criticalError == nil {
			criticalError = errors.Wrap(result.Error, "RTM API availability check failed")
		}
	}

	// Step 3: Check API key validity via echo test if requested and previous checks passed
	if options.CheckAPIKey && (criticalError == nil || (!options.CheckInternet && !options.CheckRTMAPI)) {
		result := s.checkRTMEcho(ctx)
		results = append(results, result)
		if !result.Success && criticalError == nil {
			criticalError = errors.Wrap(result.Error, "RTM API echo test failed (API key/secret may be invalid)")
		}
	}

	// Step 4: Check authentication if requested and previous checks passed
	if options.CheckAuth && (criticalError == nil || (!options.CheckInternet && !options.CheckRTMAPI && !options.CheckAPIKey)) {
		authResult := s.checkRTMAuth(ctx)
		results = append(results, authResult)

		// If authentication is required but we're not authenticated, treat as critical error
		if options.RequireAuth && !authResult.Success && criticalError == nil {
			criticalError = errors.New("authentication required but not authenticated with RTM")
		}
	}

	return results, criticalError
}

// checkInternetConnectivity tests if the internet is reachable.
func (s *Service) checkInternetConnectivity(ctx context.Context, client *http.Client, testURL string) DiagnosticResult {
	start := time.Now()
	s.logger.Debug("Testing internet connectivity...", "url", testURL)

	req, err := http.NewRequestWithContext(ctx, "HEAD", testURL, nil)
	if err != nil {
		return DiagnosticResult{
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
		return DiagnosticResult{
			Name:        "Internet Connectivity",
			Success:     false,
			Error:       errors.Wrap(err, "HTTP HEAD request failed"),
			Description: "Failed to connect to the internet",
			Duration:    duration,
		}
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			s.logger.Warn("Error closing response body", "error", closeErr)
		}
	}()

	if resp.StatusCode >= 400 {
		return DiagnosticResult{
			Name:        "Internet Connectivity",
			Success:     false,
			Error:       errors.Errorf("HTTP status code: %d", resp.StatusCode),
			Description: "Received error status code from internet test",
			Duration:    duration,
		}
	}

	s.logger.Debug("Internet connectivity test successful",
		"status", resp.Status,
		"duration", duration)
	return DiagnosticResult{
		Name:        "Internet Connectivity",
		Success:     true,
		Error:       nil,
		Description: fmt.Sprintf("Connected to %s (HTTP %d)", testURL, resp.StatusCode),
		Duration:    duration,
	}
}

// checkRTMAvailability tests if the RTM API endpoint is reachable.
func (s *Service) checkRTMAvailability(ctx context.Context, client *http.Client) DiagnosticResult {
	start := time.Now()
	apiEndpoint := s.client.GetAPIEndpoint()
	s.logger.Debug("Testing RTM API endpoint availability...", "url", apiEndpoint)

	req, err := http.NewRequestWithContext(ctx, "HEAD", apiEndpoint, nil)
	if err != nil {
		return DiagnosticResult{
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
		return DiagnosticResult{
			Name:        "RTM API Availability",
			Success:     false,
			Error:       errors.Wrap(err, "HTTP HEAD request failed"),
			Description: "Failed to connect to RTM API endpoint",
			Duration:    duration,
		}
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			s.logger.Warn("Error closing response body", "error", closeErr)
		}
	}()

	s.logger.Debug("RTM API endpoint is reachable",
		"status", resp.Status,
		"duration", duration)
	return DiagnosticResult{
		Name:        "RTM API Availability",
		Success:     true,
		Error:       nil,
		Description: fmt.Sprintf("RTM API endpoint is reachable (HTTP %d)", resp.StatusCode),
		Duration:    duration,
	}
}

// checkRTMEcho tests the rtm.test.echo method (doesn't require authentication).
func (s *Service) checkRTMEcho(ctx context.Context) DiagnosticResult {
	start := time.Now()
	s.logger.Debug("Testing RTM API with non-authenticated method (rtm.test.echo)...")

	params := map[string]string{"test_param": "hello_rtm"}
	respBytes, err := s.client.CallMethod(ctx, "rtm.test.echo", params)
	duration := time.Since(start)

	if err != nil {
		return DiagnosticResult{
			Name:        "RTM API Echo Test",
			Success:     false,
			Error:       err, // Assumes client.CallMethod wraps errors appropriately
			Description: "Failed to call rtm.test.echo method",
			Duration:    duration,
		}
	}

	respStr := string(respBytes)
	if !strings.Contains(respStr, `"test_param": "hello_rtm"`) {
		return DiagnosticResult{
			Name:        "RTM API Echo Test",
			Success:     false,
			Error:       errors.New("response doesn't contain expected key-value pair"),
			Description: fmt.Sprintf("Unexpected response content: %s", truncateString(respStr, 100)),
			Duration:    duration,
		}
	}

	s.logger.Debug("RTM API echo test successful",
		"duration", duration,
		"response_preview", truncateString(respStr, 100))
	return DiagnosticResult{
		Name:        "RTM API Echo Test",
		Success:     true,
		Error:       nil,
		Description: "Successfully called rtm.test.echo method (API key valid)",
		Duration:    duration,
	}
}

// checkRTMAuth tests the authentication status.
func (s *Service) checkRTMAuth(ctx context.Context) DiagnosticResult {
	start := time.Now()
	s.logger.Debug("Checking RTM authentication status...")

	authState, err := s.GetAuthState(ctx)
	duration := time.Since(start)

	if err != nil {
		return DiagnosticResult{
			Name:        "RTM Authentication",
			Success:     false,
			Error:       err,
			Description: "Failed to get authentication state",
			Duration:    duration,
		}
	}

	if !authState.IsAuthenticated {
		return DiagnosticResult{
			Name:        "RTM Authentication",
			Success:     false,
			Error:       nil, // Not an error, just not authenticated
			Description: "Not authenticated with RTM",
			Duration:    duration,
		}
	}

	s.logger.Debug("RTM authentication check successful",
		"username", authState.Username,
		"duration", duration)
	return DiagnosticResult{
		Name:        "RTM Authentication",
		Success:     true,
		Error:       nil,
		Description: fmt.Sprintf("Authenticated as %s", authState.Username),
		Duration:    duration,
	}
}

// truncateString truncates a string to a maximum length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
