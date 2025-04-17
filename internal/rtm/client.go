// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/client.go

import (
	"context"
	"crypto/md5" //nolint:gosec // Required by RTM API
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time" // Keep time import

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
)

// Client is a Remember The Milk API client.
type Client struct {
	config Config
	logger logging.Logger
}

// NewClient creates a new RTM client with the given configuration.
func NewClient(config Config, logger logging.Logger) *Client {
	if config.APIEndpoint == "" {
		config.APIEndpoint = defaultAPIEndpoint
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	return &Client{
		config: config,
		logger: logger.WithField("component", "rtm_client"),
	}
}

// SetAuthToken manually sets an authentication token.
func (c *Client) SetAuthToken(token string) {
	c.config.AuthToken = token
}

// GetAuthToken returns the current authentication token.
func (c *Client) GetAuthToken() string {
	return c.config.AuthToken
}

// callMethod makes a call to the RTM API.
// It handles parameter preparation, signing, HTTP request execution,
// and basic response/error checking.
func (c *Client) callMethod(ctx context.Context, method string, params map[string]string) (json.RawMessage, error) {
	// --- 1. Prepare Request ---
	fullParams := c.prepareParameters(method, params)
	apiURL, err := c.buildRequestURL(fullParams)
	if err != nil {
		return nil, err // Error already wrapped by buildRequestURL
	}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", "CowGnition/0.1.0") // TODO: Version from build flags

	// --- 2. Execute Request ---
	c.logger.Debug("Making RTM API call.", "method", method, "url", apiURL.String())
	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		// Wrap HTTP client errors
		return nil, errors.Wrapf(err, "RTM HTTP request failed (method: %s)", method)
	}
	defer func() {
		// Check and log error from closing RTM API response body.
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Use the client's logger instance.
			c.logger.Warn("Error closing RTM API response body.", "error", closeErr)
		}
	}()
	// --- 3. Read Response Body ---
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read RTM response body")
	}

	// --- 4. Check HTTP Status ---
	if resp.StatusCode != http.StatusOK {
		return nil, c.handleHTTPError(resp.StatusCode, resp.Status, body, method)
	}

	// --- 5. Check RTM API Status ---
	// Unmarshal only the base structure to check 'stat' and 'err' fields.
	var baseResult struct {
		Rsp baseRsp `json:"rsp"`
	}
	if err := json.Unmarshal(body, &baseResult); err != nil {
		return nil, errors.Wrap(err, "failed to parse base RTM API response structure")
	}

	if baseResult.Rsp.Stat != "ok" {
		return nil, c.handleRTMError(baseResult.Rsp, method)
	}

	// --- 6. Return Raw Body on Success ---
	// If HTTP status is OK and RTM status is "ok", return the raw body
	// for the calling method to parse its specific structure.
	return body, nil
}

// prepareParameters adds standard params and generates the signature.
func (c *Client) prepareParameters(method string, params map[string]string) map[string]string {
	fullParams := make(map[string]string)
	for k, v := range params {
		fullParams[k] = v
	}
	fullParams["method"] = method
	fullParams["api_key"] = c.config.APIKey
	fullParams["format"] = responseFormat
	if c.config.AuthToken != "" && method != methodGetFrob && method != methodGetToken {
		fullParams["auth_token"] = c.config.AuthToken
	}
	fullParams["api_sig"] = c.generateSignature(fullParams)
	return fullParams
}

// buildRequestURL constructs the final URL with query parameters.
func (c *Client) buildRequestURL(fullParams map[string]string) (*url.URL, error) {
	apiURL, err := url.Parse(c.config.APIEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse API endpoint URL")
	}
	q := apiURL.Query()
	for k, v := range fullParams {
		q.Set(k, v)
	}
	apiURL.RawQuery = q.Encode()
	return apiURL, nil
}

// handleHTTPError creates an RTMError for non-200 HTTP responses.
func (c *Client) handleHTTPError(statusCode int, status string, body []byte, method string) error {
	errCtx := map[string]interface{}{
		"statusCode": statusCode,
		"status":     status,
		"body":       string(body),
		"rtm_method": method,
	}
	return mcperrors.NewRTMError(mcperrors.ErrRTMAPIFailure,
		fmt.Sprintf("API returned non-200 status: %d", statusCode),
		nil, // No underlying Go error
		errCtx)
}

// handleRTMError creates an RTMError or AuthError for RTM API errors (stat != "ok").
func (c *Client) handleRTMError(rsp baseRsp, method string) error {
	// Base context
	errCtx := map[string]interface{}{
		"rtm_method": method,
	}
	if rsp.Err != nil {
		errCtx["rtmErrorCode"] = rsp.Err.Code
		errCtx["rtmErrorMessage"] = rsp.Err.Msg
	} else {
		errCtx["rtmStatus"] = rsp.Stat
	}

	if rsp.Err != nil {
		rtmErrCode := 0
		_, scanErr := fmt.Sscan(rsp.Err.Code, &rtmErrCode) // Check Sscan error
		if scanErr != nil {
			c.logger.Warn("Failed to parse RTM error code", "rawCode", rsp.Err.Code, "scanError", scanErr)
			errCtx["codeScanError"] = scanErr.Error()
			// Return generic invalid response error
			return mcperrors.NewRTMError(mcperrors.ErrRTMInvalidResponse,
				fmt.Sprintf("RTM API Error: %s (failed to parse error code '%s')", rsp.Err.Msg, rsp.Err.Code),
				scanErr, // Include scan error
				errCtx)
		}

		// Handle specific mapped errors
		if rtmErrCode == rtmErrCodeInvalidAuthToken { // Use constant
			return mcperrors.NewAuthError(
				fmt.Sprintf("RTM API Error: %s", rsp.Err.Msg),
				nil,
				errCtx,
			)
		}

		// Return generic RTM API failure
		return mcperrors.NewRTMError(mcperrors.ErrRTMAPIFailure,
			fmt.Sprintf("RTM API Error: %s", rsp.Err.Msg),
			nil,
			errCtx,
		)
	}

	// If stat != "ok" but no <err> block
	return mcperrors.NewRTMError(mcperrors.ErrRTMInvalidResponse,
		fmt.Sprintf("RTM API returned non-ok status '%s' without error details", rsp.Stat),
		nil,
		errCtx,
	)
}

// generateSignature generates an API signature using MD5.
func (c *Client) generateSignature(params map[string]string) string {
	// Step 1: Extract and sort keys alphabetically
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Step 2: Build the raw string to hash (secret + sorted params)
	var builder strings.Builder
	builder.WriteString(c.config.SharedSecret)

	// Add each key and value in sorted order
	for _, k := range keys {
		builder.WriteString(k)
		builder.WriteString(params[k])
	}
	rawString := builder.String()

	// Step 3: Calculate MD5 hash - avoid logging the full raw string to protect secrets
	c.logger.Debug("Generating API signature",
		"rawStringLength", len(rawString),
		"paramCount", len(params),
		"paramKeys", strings.Join(keys, ","))

	// Step 4: Calculate MD5 hash
	hasher := md5.New() // nolint:gosec // required by RTM API
	hasher.Write([]byte(rawString))
	hashBytes := hasher.Sum(nil)

	// Step 5: Convert to hex string
	signature := hex.EncodeToString(hashBytes)

	c.logger.Debug("Generated API signature", "signature", signature)

	return signature
}

// CallMethod makes a direct call to the RTM API.
// This is a public wrapper around the internal callMethod implementation.
// It handles parameter preparation, signing, HTTP request execution, and basic response checking.
func (c *Client) CallMethod(ctx context.Context, method string, params map[string]string) (json.RawMessage, error) {
	return c.callMethod(ctx, method, params)
}
