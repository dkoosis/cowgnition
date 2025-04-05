// Package rtm provides a client for interacting with the Remember The Milk (RTM) API v2.
// It handles request signing, authentication, and making API calls.
package rtm

import (
	"context"
	"crypto/md5" // #nosec G501 - RTM API specifically requires MD5 for request signing, so weak crypto is unavoidable here.
	"encoding/hex"
	"io"
	"log" // Consider replacing with a structured logger if available package-wide.
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors" // Project's MCP error types.
)

const (
	// APIURL is the base endpoint for RTM REST API calls.
	APIURL = "https://api.rememberthemilk.com/services/rest/"
	// AuthURL is the base endpoint for the RTM web-based authentication flow.
	AuthURL = "https://www.rememberthemilk.com/services/auth/"

	// DefaultTimeout specifies the default timeout duration for HTTP requests made to the RTM API.
	DefaultTimeout = 30 * time.Second
)

// Client represents an RTM API client, holding credentials and the HTTP client used for requests.
type Client struct {
	// APIKey is the key assigned by RTM for application identification.
	APIKey string
	// SharedSecret is the secret key assigned by RTM, used for signing API requests.
	SharedSecret string
	// AuthToken is the user-specific token obtained after authentication, required for most API calls.
	AuthToken string
	// HTTPClient is the client used to make HTTP requests to the RTM API.
	HTTPClient *http.Client
}

// NewClient creates and returns a new RTM API client initialized with the
// provided API key and shared secret. It uses a default HTTP client with a timeout.
func NewClient(apiKey, sharedSecret string) *Client {
	return &Client{
		APIKey:       apiKey,
		SharedSecret: sharedSecret,
		HTTPClient:   &http.Client{Timeout: DefaultTimeout},
	}
}

// SetAuthToken updates the authentication token used by the client for subsequent requests.
func (c *Client) SetAuthToken(token string) {
	c.AuthToken = token
}

// Sign generates the required API signature (api_sig) for a map of request parameters.
// It follows the RTM API's specific signing process:
// 1. Sort parameters alphabetically by key.
// 2. Concatenate the shared secret followed by all key-value pairs (without separators).
// 3. Calculate the MD5 hash of the resulting string.
// Note: RTM explicitly requires MD5 for signing, despite its known weaknesses.
func (c *Client) Sign(params map[string]string) string {
	// Step 1: Get parameter keys and sort them alphabetically.
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Step 2: Concatenate the shared secret and the sorted key-value pairs.
	var sb strings.Builder
	sb.WriteString(c.SharedSecret)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}

	// Step 3: Calculate the MD5 hash of the concatenated string.
	// #nosec G401 - RTM API specifically requires MD5 for request signing. This is a documented requirement.
	hash := md5.Sum([]byte(sb.String())) //nolint:gosec // Required by RTM API.
	return hex.EncodeToString(hash[:])
}

// MakeRequest sends a request to the specified RTM API method with the given parameters.
// It automatically adds the api_key, format (json), auth_token (if available),
// and generates the api_sig before making the GET request.
// It returns the raw response body as a byte slice or an error if the request fails at any stage.
func (c *Client) MakeRequest(method string, params map[string]string) ([]byte, error) {
	// Clone the parameters map to avoid modifying the caller's original map.
	reqParams := make(map[string]string)
	for k, v := range params {
		reqParams[k] = v
	}

	// Add required common RTM API parameters.
	reqParams["method"] = method
	reqParams["api_key"] = c.APIKey
	reqParams["format"] = "json" // Request JSON responses.

	// Include the authentication token if it has been set for the client.
	if c.AuthToken != "" {
		reqParams["auth_token"] = c.AuthToken
	}

	// Sign the request parameters using the RTM-required MD5 method.
	reqParams["api_sig"] = c.Sign(reqParams)

	// Build the final request URL with encoded query parameters.
	values := url.Values{}
	for k, v := range reqParams {
		values.Add(k, v)
	}
	requestURL := APIURL + "?" + values.Encode()
	// Consider replacing log.Printf with a structured logger if appropriate for the project.
	log.Printf("Making RTM API request: %s.", requestURL) // Added period.

	// Create the HTTP GET request with a background context.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, requestURL, nil)
	// Handle potential errors during request creation.
	if err != nil {
		return nil, cgerr.NewRTMError(
			0, // RTM error code not applicable here.
			"Failed to create HTTP request for RTM API.",
			err,
			map[string]interface{}{
				"method": method,
				"url":    requestURL,
			},
		)
	}

	// Execute the HTTP request using the configured client.
	resp, err := c.HTTPClient.Do(req)
	// Handle potential errors during HTTP request execution (e.g., network issues, timeouts).
	if err != nil {
		return nil, cgerr.NewRTMError(
			0, // RTM error code not applicable here.
			"Failed to execute HTTP request to RTM API.",
			err,
			map[string]interface{}{
				"method": method,
				"url":    requestURL,
			},
		)
	}
	// Ensure the response body is closed after processing.
	defer resp.Body.Close()

	// Read the entire response body.
	body, err := io.ReadAll(resp.Body)
	// Handle potential errors during response body reading.
	if err != nil {
		return nil, cgerr.NewRTMError(
			0, // RTM error code not applicable here.
			"Failed to read response body from RTM API.",
			err,
			map[string]interface{}{
				"method":      method,
				"url":         requestURL,
				"status_code": resp.StatusCode,
			},
		)
	}

	// Return the raw response body. Further processing (e.g., JSON parsing, error checking) is left to the caller.
	return body, nil
}
