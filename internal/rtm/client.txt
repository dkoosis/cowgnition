// file: internal/rtm/client.go
// Package rtm provides a client for interacting with the Remember The Milk (RTM) API v2.
// It handles request signing, authentication, and making API calls.
// Terminate all comments with a period.
package rtm

import (
	"context"
	"crypto/md5" // #nosec G501 - RTM API specifically requires MD5.
	"encoding/hex"
	"encoding/json" // Needed for parsing CheckToken response.
	"fmt"           // Needed for error formatting.
	"io"
	"log" // Consider replacing with structured logger.
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/cockroachdb/errors"                           // Error handling library.
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

// Client represents an RTM API client.
type Client struct {
	APIKey       string
	SharedSecret string
	AuthToken    string
	HTTPClient   *http.Client
}

// NewClient creates and returns a new RTM API client.
func NewClient(apiKey, sharedSecret string) *Client {
	return &Client{
		APIKey:       apiKey,
		SharedSecret: sharedSecret,
		HTTPClient:   &http.Client{Timeout: DefaultTimeout},
		// AuthToken is initially empty.
	}
}

// SetAuthToken updates the authentication token used by the client.
func (c *Client) SetAuthToken(token string) {
	c.AuthToken = token
}

// Sign generates the required RTM API signature (api_sig).
func (c *Client) Sign(params map[string]string) string {
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(c.SharedSecret)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}
	// #nosec G401 - RTM API requires MD5.
	hash := md5.Sum([]byte(sb.String())) //nolint:gosec
	return hex.EncodeToString(hash[:])
}

// MakeRequest sends a request to the specified RTM API method with parameters.
// Updated signature to accept context.Context.
func (c *Client) MakeRequest(ctx context.Context, method string, params map[string]string) ([]byte, error) {
	reqParams := make(map[string]string)
	for k, v := range params {
		reqParams[k] = v
	}

	reqParams["method"] = method
	reqParams["api_key"] = c.APIKey
	reqParams["format"] = "json"
	if c.AuthToken != "" {
		reqParams["auth_token"] = c.AuthToken
	}
	reqParams["api_sig"] = c.Sign(reqParams)

	values := url.Values{}
	for k, v := range reqParams {
		values.Add(k, v)
	}
	requestURL := APIURL + "?" + values.Encode()
	log.Printf("Making RTM API request: %s.", requestURL) // Added period.

	// Use the passed context for the request.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		// Wrap error for context.
		return nil, cgerr.NewRTMError(
			0, "Failed to create HTTP request for RTM API.", errors.WithStack(err),
			map[string]interface{}{"method": method, "url": requestURL},
		)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		// Check if context error caused the Do failure.
		if ctx.Err() != nil {
			return nil, cgerr.NewRTMError(
				0, fmt.Sprintf("Context ended during RTM API request (%s).", ctx.Err()), errors.WithStack(ctx.Err()),
				map[string]interface{}{"method": method, "url": requestURL},
			)
		}
		// Otherwise, wrap the http client error.
		return nil, cgerr.NewRTMError(
			0, "Failed to execute HTTP request to RTM API.", errors.WithStack(err),
			map[string]interface{}{"method": method, "url": requestURL},
		)
	}
	defer resp.Body.Close() // Ensure body is closed.

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, cgerr.NewRTMError(
			0, "Failed to read response body from RTM API.", errors.WithStack(err),
			map[string]interface{}{"method": method, "url": requestURL, "status_code": resp.StatusCode},
		)
	}

	// Basic check for RTM API level error in response (often indicated by non-200 status).
	// RTM might still return 200 with an error in the JSON, handled by specific method calls.
	if resp.StatusCode != http.StatusOK {
		// Attempt to parse RTM specific error if possible, otherwise return generic HTTP error.
		var rtmErrResp struct {
			Rsp struct {
				Stat string `json:"stat"`
				Err  struct {
					Code int    `json:"code,string"` // RTM codes are strings in JSON.
					Msg  string `json:"msg"`
				} `json:"err"`
			} `json:"rsp"`
		}
		// Try unmarshalling even on non-200, might contain error details.
		_ = json.Unmarshal(body, &rtmErrResp) // Ignore unmarshal error here, focus on HTTP status.

		rtmCode := rtmErrResp.Rsp.Err.Code
		rtmMsg := rtmErrResp.Rsp.Err.Msg
		if rtmMsg == "" {
			rtmMsg = fmt.Sprintf("RTM API request failed with HTTP status %d.", resp.StatusCode)
		}

		return nil, cgerr.NewRTMError(
			rtmCode, rtmMsg, fmt.Errorf("HTTP status %d", resp.StatusCode),
			map[string]interface{}{
				"method":        method,
				"url":           requestURL,
				"status_code":   resp.StatusCode,
				"response_body": string(body), // Include body snippet in error context.
			},
		)
	}

	return body, nil
}

// --- Specific RTM Method Implementations ---

// AuthResponse represents the structure within the "rsp" field for auth-related calls.
type AuthResponse struct {
	Stat string `json:"stat"` // Should be "ok" on success.
	Auth *struct {
		Token string `json:"token"`
		Perms string `json:"perms"`
		User  struct {
			ID       string `json:"id,string"` // RTM uses string IDs.
			Username string `json:"username"`
			Fullname string `json:"fullname"`
		} `json:"user"`
	} `json:"auth,omitempty"` // Optional: present in checkToken, getToken.
	Frob string    `json:"frob,omitempty"` // Optional: present in getFrob.
	Err  *struct { // Optional: present on failure.
		Code int    `json:"code,string"` // RTM error codes are strings.
		Msg  string `json:"msg"`
	} `json:"err,omitempty"`
}

// CheckTokenCtx checks the validity of the client's current AuthToken using context.
// Returns AuthResponse on success, nil otherwise.
func (c *Client) CheckTokenCtx(ctx context.Context) (*AuthResponse, error) {
	if c.AuthToken == "" {
		return nil, cgerr.NewAuthError("Cannot check token: AuthToken is not set.", nil, nil)
	}
	params := map[string]string{} // No extra params needed besides auth_token added by MakeRequest.
	body, err := c.MakeRequest(ctx, "rtm.auth.checkToken", params)
	if err != nil {
		return nil, errors.Wrap(err, "CheckTokenCtx: API request failed.") // Wrap for context.
	}

	var response struct {
		Rsp AuthResponse `json:"rsp"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, cgerr.NewRTMError(
			0, "Failed to parse checkToken response.", errors.WithStack(err),
			map[string]interface{}{"response_body": string(body)},
		)
	}

	// Check RTM API specific error field.
	if response.Rsp.Stat != "ok" || response.Rsp.Err != nil {
		errMsg := "RTM checkToken failed."
		errCode := 0
		if response.Rsp.Err != nil {
			errMsg = response.Rsp.Err.Msg
			errCode = response.Rsp.Err.Code
		}
		return nil, cgerr.NewRTMError(errCode, errMsg, nil, map[string]interface{}{"response_body": string(body)})
	}

	// Check if auth structure is present (it should be on success).
	if response.Rsp.Auth == nil {
		return nil, cgerr.NewRTMError(
			0, "Invalid checkToken response: missing auth details.", nil,
			map[string]interface{}{"response_body": string(body)},
		)
	}

	return &response.Rsp, nil // Return the AuthResponse part.
}

// GetFrobCtx gets a frob required to start the desktop authentication flow, using context.
// Returns the frob string or an error.
func (c *Client) GetFrobCtx(ctx context.Context) (string, error) {
	params := map[string]string{}
	body, err := c.MakeRequest(ctx, "rtm.auth.getFrob", params)
	if err != nil {
		return "", errors.Wrap(err, "GetFrobCtx: API request failed.")
	}

	var response struct {
		Rsp AuthResponse `json:"rsp"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", cgerr.NewRTMError(
			0, "Failed to parse getFrob response.", errors.WithStack(err),
			map[string]interface{}{"response_body": string(body)},
		)
	}

	if response.Rsp.Stat != "ok" || response.Rsp.Err != nil || response.Rsp.Frob == "" {
		errMsg := "RTM getFrob failed or returned empty frob."
		errCode := 0
		if response.Rsp.Err != nil {
			errMsg = response.Rsp.Err.Msg
			errCode = response.Rsp.Err.Code
		}
		return "", cgerr.NewRTMError(errCode, errMsg, nil, map[string]interface{}{"response_body": string(body)})
	}

	return response.Rsp.Frob, nil
}

// GetAuthURL generates the RTM web authentication URL for the user.
func (c *Client) GetAuthURL(frob string, perms string) string {
	params := map[string]string{
		"api_key": c.APIKey,
		"perms":   perms,
		"frob":    frob,
	}
	apiSig := c.Sign(params)

	values := url.Values{}
	values.Add("api_key", c.APIKey)
	values.Add("perms", perms)
	values.Add("frob", frob)
	values.Add("api_sig", apiSig)

	return AuthURL + "?" + values.Encode()
}

// GetTokenCtx exchanges a frob for a permanent authentication token, using context.
// Returns AuthResponse containing the token and user info on success.
func (c *Client) GetTokenCtx(ctx context.Context, frob string) (*AuthResponse, error) {
	params := map[string]string{
		"frob": frob,
	}
	body, err := c.MakeRequest(ctx, "rtm.auth.getToken", params)
	if err != nil {
		return nil, errors.Wrap(err, "GetTokenCtx: API request failed.")
	}

	var response struct {
		Rsp AuthResponse `json:"rsp"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, cgerr.NewRTMError(
			0, "Failed to parse getToken response.", errors.WithStack(err),
			map[string]interface{}{"response_body": string(body), "frob": frob},
		)
	}

	if response.Rsp.Stat != "ok" || response.Rsp.Err != nil || response.Rsp.Auth == nil || response.Rsp.Auth.Token == "" {
		errMsg := "RTM getToken failed or returned invalid auth details."
		errCode := 0
		if response.Rsp.Err != nil {
			errMsg = response.Rsp.Err.Msg
			errCode = response.Rsp.Err.Code
		}
		return nil, cgerr.NewRTMError(errCode, errMsg, nil, map[string]interface{}{"response_body": string(body), "frob": frob})
	}

	return &response.Rsp, nil
}
