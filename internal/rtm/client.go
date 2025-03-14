package rtm

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	// API endpoints
	baseURL      = "https://api.rememberthemilk.com/services/rest/"
	authURL      = "https://www.rememberthemilk.com/services/auth/"
	
	// Response status
	statusOK   = "ok"
	statusFail = "fail"
)

// Client represents an RTM API client
type Client struct {
	apiKey       string
	sharedSecret string
	authToken    string
	httpClient   *http.Client
}

// Response represents a generic RTM API response
type Response struct {
	XMLName xml.Name `xml:"rsp"`
	Status  string   `xml:"stat,attr"`
	Error   *struct {
		Code    string `xml:"code,attr"`
		Message string `xml:"msg,attr"`
	} `xml:"err,omitempty"`
}

// NewClient creates a new RTM API client
func NewClient(apiKey, sharedSecret string) *Client {
	return &Client{
		apiKey:       apiKey,
		sharedSecret: sharedSecret,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetAuthToken sets the authentication token
func (c *Client) SetAuthToken(token string) {
	c.authToken = token
}

// GetAuthToken returns the current authentication token
func (c *Client) GetAuthToken() string {
	return c.authToken
}

// GetAuthURL generates an authentication URL for a desktop application
func (c *Client) GetAuthURL(frob, perms string) string {
	params := url.Values{}
	params.Set("api_key", c.apiKey)
	params.Set("perms", perms)
	
	if frob != "" {
		params.Set("frob", frob)
	}
	
	sig := c.generateSignature(params)
	params.Set("api_sig", sig)
	
	return authURL + "?" + params.Encode()
}

// GetFrob requests a frob from the RTM API
func (c *Client) GetFrob() (string, error) {
	type frobResponse struct {
		Response
		Frob string `xml:"frob"`
	}
	
	params := url.Values{}
	params.Set("method", "rtm.auth.getFrob")
	
	var resp frobResponse
	if err := c.doRequest(params, &resp); err != nil {
		return "", err
	}
	
	return resp.Frob, nil
}

// GetToken exchanges a frob for an auth token
func (c *Client) GetToken(frob string) (string, error) {
	type authResponse struct {
		Response
		Auth struct {
			Token string `xml:"token"`
			Perms string `xml:"perms"`
			User  struct {
				ID       string `xml:"id,attr"`
				Username string `xml:"username,attr"`
				Fullname string `xml:"fullname,attr"`
			} `xml:"user"`
		} `xml:"auth"`
	}
	
	params := url.Values{}
	params.Set("method", "rtm.auth.getToken")
	params.Set("frob", frob)
	
	var resp authResponse
	if err := c.doRequest(params, &resp); err != nil {
		return "", err
	}
	
	// Save the token
	c.authToken = resp.Auth.Token
	
	return resp.Auth.Token, nil
}

// CheckToken checks if a token is valid
func (c *Client) CheckToken() (bool, error) {
	if c.authToken == "" {
		return false, fmt.Errorf("no auth token set")
	}
	
	type authResponse struct {
		Response
		Auth struct {
			Token string `xml:"token"`
			Perms string `xml:"perms"`
			User  struct {
				ID       string `xml:"id,attr"`
				Username string `xml:"username,attr"`
				Fullname string `xml:"fullname,attr"`
			} `xml:"user"`
		} `xml:"auth"`
	}
	
	params := url.Values{}
	params.Set("method", "rtm.auth.checkToken")
	params.Set("auth_token", c.authToken)
	
	var resp authResponse
	if err := c.doRequest(params, &resp); err != nil {
		// If we get an error code 98, the token is invalid
		return false, nil
	}
	
	return true, nil
}

// doRequest performs an API request
func (c *Client) doRequest(params url.Values, v interface{}) error {
	// Add API key to parameters
	params.Set("api_key", c.apiKey)
	
	// Add auth token if set
	if c.authToken != "" && params.Get("auth_token") == "" {
		params.Set("auth_token", c.authToken)
	}
	
	// Generate signature
	sig := c.generateSignature(params)
	params.Set("api_sig", sig)
	
	// Prepare request
	reqURL := baseURL + "?" + params.Encode()
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	
	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response: %w", err)
	}
	
	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned non-200 status: %d %s", resp.StatusCode, resp.Status)
	}
	
	// Parse response
	if err := xml.Unmarshal(body, v); err != nil {
		return fmt.Errorf("error parsing response: %w", err)
	}
	
	// Check API status
	respStatus := v.(interface {
		GetStatus() string
		GetError() (string, string)
	})
	
	if respStatus.GetStatus() != statusOK {
		code, msg := respStatus.GetError()
		return fmt.Errorf("API returned error: %s - %s", code, msg)
	}
	
	return nil
}

// Helper method to extract status
func (r Response) GetStatus() string {
	return r.Status
}

// Helper method to extract error
func (r Response) GetError() (string, string) {
	if r.Error != nil {
		return r.Error.Code, r.Error.Message
	}
	return "", ""
}

// generateSignature generates an API signature
func (c *Client) generateSignature(params url.Values) string {
	// Sort parameters by key
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	// Concatenate parameters
	var sb strings.Builder
	sb.WriteString(c.sharedSecret)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params.Get(k))
	}
	
	// Calculate MD5 hash
	h := md5.New()
	h.Write([]byte(sb.String()))
	return hex.EncodeToString(h.Sum(nil))
}
