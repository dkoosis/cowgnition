// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
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
	Name        string        // Name of the test.
	Success     bool          // Whether the test was successful.
	Error       error         // Error if the test failed.
	Description string        // Human-readable description of the result.
	Duration    time.Duration // How long the test took to execute.
}

// ConnectivityCheckOptions defines which diagnostic checks to perform.
type ConnectivityCheckOptions struct {
	CheckInternet   bool   // Whether to check internet connectivity.
	CheckRTMAPI     bool   // Whether to check RTM API availability.
	CheckAPIKey     bool   // Whether to validate API key via echo test.
	CheckAuth       bool   // Whether to check authentication status.
	RequireAuth     bool   // Whether authentication is required.
	InternetTestURL string // URL to use for internet connectivity test.
}

// DefaultConnectivityCheckOptions returns default options for connectivity checks.
func DefaultConnectivityCheckOptions() ConnectivityCheckOptions {
	return ConnectivityCheckOptions{
		CheckInternet:   true,
		CheckRTMAPI:     true,
		CheckAPIKey:     true,
		CheckAuth:       true,
		RequireAuth:     false,
		InternetTestURL: "https://www.google.com", // Default test URL.
	}
}

// PerformConnectivityCheck runs a series of diagnostic tests to verify RTM connectivity.
// It returns the results of each test and an error if any critical test fails.
func (s *Service) PerformConnectivityCheck(ctx context.Context, options ConnectivityCheckOptions) ([]DiagnosticResult, error) {
	results := make([]DiagnosticResult, 0, 4) // Pre-allocate slice.
	var criticalError error

	httpClient := &http.Client{Timeout: 10 * time.Second}

	// Step 1: Check Internet connectivity.
	if options.CheckInternet {
		result := s.checkInternetConnectivity(ctx, httpClient, options.InternetTestURL)
		results = append(results, result)
		if !result.Success && criticalError == nil {
			criticalError = errors.Wrap(result.Error, "internet connectivity check failed")
		}
	}

	// Step 2: Check RTM API availability.
	if options.CheckRTMAPI && (criticalError == nil || !options.CheckInternet) {
		result := s.checkRTMAvailability(ctx, httpClient)
		results = append(results, result)
		if !result.Success && criticalError == nil {
			criticalError = errors.Wrap(result.Error, "RTM API availability check failed")
		}
	}

	// Step 3: Check API key validity via echo test.
	if options.CheckAPIKey && (criticalError == nil || (!options.CheckInternet && !options.CheckRTMAPI)) {
		result := s.checkRTMEcho(ctx)
		results = append(results, result)
		if !result.Success && criticalError == nil {
			criticalError = errors.Wrap(result.Error, "RTM API echo test failed (API key/secret may be invalid)")
		}
	}

	// Step 4: Check authentication.
	if options.CheckAuth && (criticalError == nil || (!options.CheckInternet && !options.CheckRTMAPI && !options.CheckAPIKey)) {
		authResult := s.checkRTMAuth(ctx)
		results = append(results, authResult)
		if options.RequireAuth && !authResult.Success && criticalError == nil {
			criticalError = errors.New("authentication required but not authenticated with RTM")
		}
	}

	return results, criticalError
}

// formatDiagnosticResult formats a diagnostic result string.
func formatDiagnosticResult(result DiagnosticResult) string {
	status := "PASS"
	icon := "✅"
	detail := result.Description

	if !result.Success {
		status = "FAIL"
		icon = "❌"
		// Optionally include simplified error in description for FAIL status.
		if result.Error != nil {
			// Limit error length in summary if desired.
			errorStr := fmt.Sprintf("%v", result.Error)
			maxErrLen := 50
			if len(errorStr) > maxErrLen {
				errorStr = errorStr[:maxErrLen] + "..."
			}
			detail = fmt.Sprintf("%s - %s", result.Description, errorStr)
		}
	}
	// Format: Icon Name... STATUS (Description) - Adjusted padding.
	return fmt.Sprintf("%s %-28s %s (%s)", icon, result.Name+"...", status, detail)
}

// checkInternetConnectivity tests if the internet is reachable.
func (s *Service) checkInternetConnectivity(ctx context.Context, client *http.Client, testURL string) DiagnosticResult {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "HEAD", testURL, nil)
	if err != nil {
		return DiagnosticResult{
			Name: "Internet Connectivity", Success: false, Error: errors.Wrap(err, "failed to create request"),
			Description: "Internal error creating request", Duration: time.Since(start),
		}
	}

	resp, err := client.Do(req)
	duration := time.Since(start)
	if err != nil {
		return DiagnosticResult{
			Name: "Internet Connectivity", Success: false, Error: errors.Wrap(err, "HTTP HEAD request failed"),
			Description: "Failed to connect to internet test URL", Duration: duration,
		}
	}
	defer func() { _ = resp.Body.Close() }() // Simplified close.

	if resp.StatusCode >= 400 {
		return DiagnosticResult{
			Name: "Internet Connectivity", Success: false, Error: errors.Errorf("HTTP status code: %d", resp.StatusCode),
			Description: fmt.Sprintf("Received error status (%d) from test URL", resp.StatusCode), Duration: duration,
		}
	}

	return DiagnosticResult{
		Name: "Internet Connectivity", Success: true, Error: nil,
		Description: fmt.Sprintf("Connected to %s", testURL), Duration: duration,
	}
}

// checkRTMAvailability tests if the RTM API endpoint is reachable.
func (s *Service) checkRTMAvailability(ctx context.Context, client *http.Client) DiagnosticResult {
	start := time.Now()
	apiEndpoint := s.client.GetAPIEndpoint()

	req, err := http.NewRequestWithContext(ctx, "HEAD", apiEndpoint, nil)
	if err != nil {
		return DiagnosticResult{
			Name: "RTM API Availability", Success: false, Error: errors.Wrap(err, "failed to create request"),
			Description: "Internal error creating request", Duration: time.Since(start),
		}
	}

	resp, err := client.Do(req)
	duration := time.Since(start)
	if err != nil {
		return DiagnosticResult{
			Name: "RTM API Availability", Success: false, Error: errors.Wrap(err, "HTTP HEAD request failed"),
			Description: "Failed to connect to RTM API endpoint", Duration: duration,
		}
	}
	defer func() { _ = resp.Body.Close() }() // Simplified close.

	// Note: RTM returns 404 for HEAD on the REST endpoint, which is okay for reachability.
	return DiagnosticResult{
		Name: "RTM API Availability", Success: true, Error: nil,
		Description: fmt.Sprintf("Endpoint reachable (HTTP %d)", resp.StatusCode), Duration: duration,
	}
}

// checkRTMEcho tests the rtm.test.echo method (validates API key/secret).
func (s *Service) checkRTMEcho(ctx context.Context) DiagnosticResult {
	start := time.Now()

	params := map[string]string{"test_param": "cowgnition_echo"}
	respBytes, err := s.client.CallMethod(ctx, "rtm.test.echo", params)
	duration := time.Since(start)

	if err != nil {
		return DiagnosticResult{ // Return specific failure description.
			Name: "RTM API Echo Test", Success: false, Error: err,
			Description: "API Key/Secret likely invalid", Duration: duration,
		}
	}

	// Check if the echoed parameter is present in the response.
	respStr := string(respBytes)
	if !strings.Contains(respStr, `"test_param":"cowgnition_echo"`) { // Check for exact match.
		return DiagnosticResult{
			Name: "RTM API Echo Test", Success: false, Error: errors.New("echoed parameter mismatch"),
			Description: "Unexpected echo response format", Duration: duration,
		}
	}

	return DiagnosticResult{
		Name: "RTM API Echo Test", Success: true, Error: nil,
		Description: "API Key/Secret Valid", Duration: duration,
	}
}

// checkRTMAuth tests the authentication status using the currently loaded token.
func (s *Service) checkRTMAuth(ctx context.Context) DiagnosticResult {
	start := time.Now()

	authState, err := s.GetAuthState(ctx) // Use service's GetAuthState.
	duration := time.Since(start)

	if err != nil {
		return DiagnosticResult{
			Name: "RTM Authentication", Success: false, Error: err,
			Description: "Failed to check authentication state", Duration: duration,
		}
	}

	if !authState.IsAuthenticated {
		return DiagnosticResult{
			Name: "RTM Authentication", Success: false, Error: nil,
			Description: "Not currently authenticated", Duration: duration,
		}
	}

	return DiagnosticResult{
		Name: "RTM Authentication", Success: true, Error: nil,
		Description: fmt.Sprintf("Authenticated as %q", authState.Username), Duration: duration,
	}
}

// NOTE: truncateString function REMOVED from this file. It now resides in helpers.go.
