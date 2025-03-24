// Package helpers provides testing utilities for the CowGnition MCP server.
package common

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/cowgnition/cowgnition/internal/rtm"
)

// RTMTestClient provides a wrapper around the RTM client for testing,
// with added features for rate limiting, error reporting, and test control.
type RTMTestClient struct {
	client       *rtm.Client
	rateLimiter  *time.Ticker
	lastRequest  time.Time
	requestCount int
	mu           sync.Mutex
	debug        bool
}

// NewRTMTestClient creates a new RTM test client.
// It uses provided credentials or attempts to load them from environment variables.
func NewRTMTestClient(apiKey, sharedSecret string) (*RTMTestClient, error) {
	// Try to get credentials from environment variables if not provided
	if apiKey == "" {
		apiKey = os.Getenv("RTM_API_KEY")
	}
	if sharedSecret == "" {
		sharedSecret = os.Getenv("RTM_SHARED_SECRET")
	}

	// Validate credentials
	if apiKey == "" || sharedSecret == "" {
		return nil, fmt.Errorf("RTM credentials not provided and not found in environment variables")
	}

	// Create RTM client
	client := rtm.NewClient(apiKey, sharedSecret)

	// Create rate limiter (1 request per second as per RTM API guidelines)
	ticker := time.NewTicker(time.Second)

	return &RTMTestClient{
		client:      client,
		rateLimiter: ticker,
		debug:       os.Getenv("RTM_TEST_DEBUG") != "",
	}, nil
}

// Close releases resources used by the client.
func (c *RTMTestClient) Close() {
	c.rateLimiter.Stop()
}

// ShouldSkipLiveTests returns true if live RTM API tests should be skipped.
// This can be controlled via environment variables.
func ShouldSkipLiveTests() bool {
	return os.Getenv("RTM_SKIP_LIVE_TESTS") == "true"
}

// waitForRateLimit waits for the rate limiter to allow a request.
// This ensures we respect RTM's rate limits.
func (c *RTMTestClient) waitForRateLimit() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check time since last request to implement proper backoff
	elapsed := time.Since(c.lastRequest)
	if elapsed < time.Second && c.lastRequest.Unix() > 0 {
		// If we're making requests too quickly, wait for the rate limiter
		<-c.rateLimiter.C
	}

	// Update last request time
	c.lastRequest = time.Now()
	c.requestCount++

	// Log request if debug mode is enabled
	if c.debug {
		log.Printf("RTM API Request #%d at %s", c.requestCount, c.lastRequest.Format(time.RFC3339))
	}
}

// GetFrob gets a frob from the RTM API with proper rate limiting and error handling.
func (c *RTMTestClient) GetFrob() (string, error) {
	c.waitForRateLimit()

	frob, err := c.client.GetFrob()
	if err != nil {
		return "", fmt.Errorf("RTM API GetFrob error: %w", err)
	}

	if c.debug {
		log.Printf("Got frob: %s", frob)
	}

	return frob, nil
}

// GetAuthURL generates an authentication URL for the given frob and permission level.
func (c *RTMTestClient) GetAuthURL(frob string, permission string) string {
	return c.client.GetAuthURL(frob, permission)
}

// GetToken exchanges a frob for an authentication token with proper rate limiting.
func (c *RTMTestClient) GetToken(frob string) (string, error) {
	c.waitForRateLimit()

	token, err := c.client.GetToken(frob)
	if err != nil {
		return "", fmt.Errorf("RTM API GetToken error: %w", err)
	}

	if c.debug {
		log.Printf("Got token: %s", token)
	}

	return token, nil
}

// SetAuthToken directly sets the authentication token on the client.
func (c *RTMTestClient) SetAuthToken(token string) {
	c.client.SetAuthToken(token)
}

// CheckToken verifies if the current token is valid with proper rate limiting.
func (c *RTMTestClient) CheckToken() (bool, error) {
	c.waitForRateLimit()

	valid, err := c.client.CheckToken()
	if err != nil {
		return false, fmt.Errorf("RTM API CheckToken error: %w", err)
	}

	if c.debug {
		log.Printf("Token check result: %t", valid)
	}

	return valid, nil
}

// GetRequestCount returns the number of requests made to the RTM API.
func (c *RTMTestClient) GetRequestCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requestCount
}

// GetClient returns the underlying RTM client for advanced usage.
// Use with caution as this bypasses rate limiting.
func (c *RTMTestClient) GetClient() *rtm.Client {
	return c.client
}
