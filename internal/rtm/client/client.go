// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import (
	"net/http"
	"time"
)

// Client represents an RTM API client.
type Client struct {
	APIKey       string
	SharedSecret string
	AuthToken    string
	HTTPClient   *http.Client
	APIURL       string
	usePOST      bool // Controls whether to use POST or GET for requests.
	rateLimiter  *RateLimiter
}

// NewClient creates a new RTM client with the provided API key and shared secret.
// It initializes a default HTTP client with reasonable timeout and a rate limiter
// configured for RTM's API limits.
func NewClient(apiKey, sharedSecret string) *Client {
	return &Client{
		APIKey:       apiKey,
		SharedSecret: sharedSecret,
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
		APIURL:       "https://api.rememberthemilk.com/services/rest/",
		usePOST:      false,                  // Default to GET requests.
		rateLimiter:  NewRateLimiter(1.0, 3), // 1 req/sec with burst of 3
	}
}

// SetAuthToken sets the authentication token for the client.
// This token is required for most API operations and is obtained
// through the authentication flow.
func (c *Client) SetAuthToken(token string) {
	c.AuthToken = token
}

// SetUsePOST sets whether to use POST for API requests.
// By default, the client uses GET requests, but some operations
// may require POST, especially those with large payloads.
func (c *Client) SetUsePOST(usePOST bool) {
	c.usePOST = usePOST
}

// ConfigureRateLimit allows adjusting the rate limiting parameters.
// This can be useful for different environments or based on API requirements changes.
// The rate is specified in requests per second, and burstLimit is the maximum
// number of requests that can be made concurrently before being limited.
func (c *Client) ConfigureRateLimit(rate float64, burstLimit int) {
	if c.rateLimiter != nil {
		c.rateLimiter.SetRateLimit(rate, burstLimit)
	} else {
		c.rateLimiter = NewRateLimiter(rate, burstLimit)
	}
}
