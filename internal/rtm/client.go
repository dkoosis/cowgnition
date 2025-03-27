// internal/rtm/client.go
package rtm

import (
	"context"
	"crypto/md5" // #nosec G501 - RTM API specifically requires MD5 for request signing
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

const (
	// API endpoints.
	APIURL  = "https://api.rememberthemilk.com/services/rest/"
	AuthURL = "https://www.rememberthemilk.com/services/auth/"

	// Timeout for API requests.
	DefaultTimeout = 30 * time.Second
)

// Client represents an RTM API client.
type Client struct {
	APIKey       string
	SharedSecret string
	AuthToken    string
	HTTPClient   *http.Client
}

// NewClient creates a new RTM API client.
func NewClient(apiKey, sharedSecret string) *Client {
	return &Client{
		APIKey:       apiKey,
		SharedSecret: sharedSecret,
		HTTPClient:   &http.Client{Timeout: DefaultTimeout},
	}
}

// SetAuthToken sets the authentication token for the client.
func (c *Client) SetAuthToken(token string) {
	c.AuthToken = token
}

// Sign generates an API signature for the given parameters.
// Uses MD5 as specifically required by the RTM API.
func (c *Client) Sign(params map[string]string) string {
	// Step 1: Sort parameters by key
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Step 2: Concatenate shared secret with sorted key-value pairs.
	var sb strings.Builder
	sb.WriteString(c.SharedSecret)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}

	// Step 3: Calculate MD5 hash
	// #nosec G401 - RTM API specifically requires MD5 for request signing.
	hash := md5.Sum([]byte(sb.String()))
	return hex.EncodeToString(hash[:])
}

// MakeRequest performs an API request to RTM.
func (c *Client) MakeRequest(method string, params map[string]string) ([]byte, error) {
	// Clone parameters map to avoid modifying the original
	reqParams := make(map[string]string)
	for k, v := range params {
		reqParams[k] = v
	}

	// Add common parameters.
	reqParams["method"] = method
	reqParams["api_key"] = c.APIKey
	reqParams["format"] = "json" // Use JSON format

	// Add auth token if set.
	if c.AuthToken != "" {
		reqParams["auth_token"] = c.AuthToken
	}

	// Sign the request.
	reqParams["api_sig"] = c.Sign(reqParams)

	// Build query string.
	values := url.Values{}
	for k, v := range reqParams {
		values.Add(k, v)
	}

	requestURL := APIURL + "?" + values.Encode()
	log.Printf("Making RTM API request: %s", requestURL)

	// Make HTTP request
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, cgerr.NewRTMError(
			0,
			"Failed to create HTTP request",
			err,
			map[string]interface{}{
				"method": method,
				"url":    requestURL,
			},
		)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, cgerr.NewRTMError(
			0,
			"Failed to make HTTP request",
			err,
			map[string]interface{}{
				"method": method,
				"url":    requestURL,
			},
		)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, cgerr.NewRTMError(
			0,
			"Failed to read response body",
			err,
			map[string]interface{}{
				"method":      method,
				"url":         requestURL,
				"status_code": resp.StatusCode,
			},
		)
	}

	return body, nil
}
