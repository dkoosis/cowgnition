// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import (
	"bytes"
	"context"
	"crypto/md5" // #nosec G501 - Required by RTM API for signature generation
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
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
func (c *Client) SetAuthToken(token string) {
	c.AuthToken = token
}

// GetAuthURL generates an authentication URL for the given frob and permission level.
func (c *Client) GetAuthURL(frob, perms string) string {
	params := url.Values{}
	params.Set("api_key", c.APIKey)
	params.Set("perms", perms)
	params.Set("frob", frob)

	apiSig := c.generateSignature(params)
	params.Set("api_sig", apiSig)

	return fmt.Sprintf("https://www.rememberthemilk.com/services/auth/?%s", params.Encode())
}

// generateSignature creates an API signature for the given parameters.
// RTM API specifically requires MD5 for signature generation per their authentication docs.
func (c *Client) generateSignature(params url.Values) string {
	// Extract keys and sort them
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Create signature string
	var sb strings.Builder
	sb.WriteString(c.SharedSecret)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params.Get(k))
	}

	// Calculate MD5 hash - required by RTM API
	hash := md5.Sum([]byte(sb.String())) // #nosec G401 - Required by RTM API specs
	return fmt.Sprintf("%x", hash)
}

// callMethod calls an RTM API method with the provided parameters.
func (c *Client) callMethod(ctx context.Context, method string, params url.Values) ([]byte, error) {
	// Apply rate limiting
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("callMethod: rate limit error: %w", err)
	}

	// Add required parameters
	if params == nil {
		params = url.Values{}
	}
	params.Set("method", method)
	params.Set("api_key", c.APIKey)
	params.Set("format", "rest")

	// Add authentication token if available
	if c.AuthToken != "" {
		params.Set("auth_token", c.AuthToken)
	}

	// Generate signature
	apiSig := c.generateSignature(params)
	params.Set("api_sig", apiSig)

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIURL, bytes.NewBufferString(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("callMethod: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("callMethod: failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limit errors (HTTP 503)
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("callMethod: service temporarily unavailable due to rate limiting")
	}

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("callMethod: failed to read response body: %w", err)
	}

	// Check for API errors
	if err := c.checkResponseForError(body); err != nil {
		return nil, err
	}

	return body, nil
}

// checkResponseForError checks if the RTM API response contains an error.
func (c *Client) checkResponseForError(response []byte) error {
	var respStruct struct {
		Stat string `xml:"stat,attr"`
		Err  struct {
			Code string `xml:"code,attr"`
			Msg  string `xml:"msg,attr"`
		} `xml:"err"`
	}

	if err := xml.Unmarshal(response, &respStruct); err != nil {
		return fmt.Errorf("checkResponseForError: failed to parse response: %w", err)
	}

	if respStruct.Stat == "fail" {
		return fmt.Errorf("checkResponseForError: RTM API error %s: %s", respStruct.Err.Code, respStruct.Err.Msg)
	}

	return nil
}

// GetFrob gets a frob from the RTM API for authentication.
func (c *Client) GetFrob() (string, error) {
	ctx := context.Background()
	resp, err := c.callMethod(ctx, "rtm.auth.getFrob", nil)
	if err != nil {
		return "", err
	}

	var result struct {
		Frob string `xml:"frob"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("GetFrob: failed to parse frob response: %w", err)
	}

	return result.Frob, nil
}

// GetToken exchanges a frob for an authentication token.
func (c *Client) GetToken(frob string) (string, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("frob", frob)

	resp, err := c.callMethod(ctx, "rtm.auth.getToken", params)
	if err != nil {
		return "", err
	}

	var result struct {
		Auth struct {
			Token string `xml:"token"`
		} `xml:"auth"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("GetToken: failed to parse token response: %w", err)
	}

	// Set the token on the client
	c.AuthToken = result.Auth.Token
	return result.Auth.Token, nil
}

// CheckToken checks if the current authentication token is valid.
func (c *Client) CheckToken() (bool, error) {
	if c.AuthToken == "" {
		return false, nil
	}

	ctx := context.Background()
	_, err := c.callMethod(ctx, "rtm.auth.checkToken", nil)
	if err != nil {
		if strings.Contains(err.Error(), "Invalid auth token") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// CreateTimeline creates a new timeline for making changes to tasks.
func (c *Client) CreateTimeline() (string, error) {
	ctx := context.Background()
	resp, err := c.callMethod(ctx, "rtm.timelines.create", nil)
	if err != nil {
		return "", err
	}

	var result struct {
		Timeline string `xml:"timeline"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("CreateTimeline: failed to parse timeline response: %w", err)
	}

	return result.Timeline, nil
}

// GetLists gets all lists from the RTM API.
func (c *Client) GetLists() ([]byte, error) {
	ctx := context.Background()
	return c.callMethod(ctx, "rtm.lists.getList", nil)
}

// GetTasks gets tasks from the RTM API with optional filtering.
func (c *Client) GetTasks(filter string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	if filter != "" {
		params.Set("filter", filter)
	}

	return c.callMethod(ctx, "rtm.tasks.getList", params)
}

// AddTask adds a new task to the specified list.
func (c *Client) AddTask(timeline, name, listID string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("name", name)
	if listID != "" {
		params.Set("list_id", listID)
	}

	return c.callMethod(ctx, "rtm.tasks.add", params)
}

// CompleteTask marks a task as complete.
func (c *Client) CompleteTask(timeline, listID, taskseriesID, taskID string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)

	return c.callMethod(ctx, "rtm.tasks.complete", params)
}

// DeleteTask deletes a task.
func (c *Client) DeleteTask(timeline, listID, taskseriesID, taskID string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)

	return c.callMethod(ctx, "rtm.tasks.delete", params)
}

// SetTaskDueDate sets the due date for a task.
func (c *Client) SetTaskDueDate(timeline, listID, taskseriesID, taskID, due string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("due", due)

	return c.callMethod(ctx, "rtm.tasks.setDueDate", params)
}

// SetTaskPriority sets the priority for a task.
func (c *Client) SetTaskPriority(timeline, listID, taskseriesID, taskID, priority string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("priority", priority)

	return c.callMethod(ctx, "rtm.tasks.setPriority", params)
}

// AddTags adds tags to a task.
func (c *Client) AddTags(timeline, listID, taskseriesID, taskID, tags string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("tags", tags)

	return c.callMethod(ctx, "rtm.tasks.addTags", params)
}

// RemoveTags removes tags from a task.
func (c *Client) RemoveTags(timeline, listID, taskseriesID, taskID, tags string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("tags", tags)

	return c.callMethod(ctx, "rtm.tasks.removeTags", params)
}

// GetTags gets all tags from the RTM API.
func (c *Client) GetTags() ([]byte, error) {
	ctx := context.Background()
	return c.callMethod(ctx, "rtm.tags.getList", nil)
}

// Response represents a generic RTM API response.
type Response struct {
	Status string `xml:"stat,attr"`
	Error  *struct {
		Code    string `xml:"code,attr"`
		Message string `xml:"msg,attr"`
	} `xml:"err"`
}

// GetError returns error code and message from a response.
func (r Response) GetError() (string, string) {
	if r.Error == nil {
		return "", ""
	}
	return r.Error.Code, r.Error.Message
}

// Constants for response status.
const (
	statusOK   = "ok"
	statusFail = "fail"
)

// APIError represents an error returned by the RTM API.
type APIError struct {
	Code    int
	Message string
}

// Error implements the error interface for APIError.
func (e APIError) Error() string {
	return fmt.Sprintf("RTM API error %d: %s", e.Code, e.Message)
}

// Do executes an API request with the given parameters and unmarshals the result.
func (c *Client) Do(params url.Values, result interface{}) ([]byte, error) {
	ctx := context.Background()
	return c.DoWithContext(ctx, params, result)
}

// DoWithContext executes an API request with the given context, parameters and unmarshals the result.
func (c *Client) DoWithContext(ctx context.Context, params url.Values, result interface{}) ([]byte, error) {
	// Apply rate limiting
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("DoWithContext: rate limit error: %w", err)
	}

	// Add required parameters
	if params == nil {
		params = url.Values{}
	}

	// Add API key and format
	params.Set("api_key", c.APIKey)
	params.Set("format", "rest")

	// Add authentication token if available
	if c.AuthToken != "" {
		params.Set("auth_token", c.AuthToken)
	}

	// Generate signature
	apiSig := c.generateSignature(params)
	params.Set("api_sig", apiSig)

	var req *http.Request
	var err error

	// Create appropriate request based on method
	if c.usePOST {
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, c.APIURL,
			strings.NewReader(params.Encode()))
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet,
			c.APIURL+"?"+params.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
	}

	// Send request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limit errors (HTTP 503)
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, APIError{
			Code:    503,
			Message: "Service temporarily unavailable due to rate limiting",
		}
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: status code %d (HTTP status: %d)",
			resp.StatusCode, resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	// Parse response
	var respStruct Response
	if err := xml.Unmarshal(body, &respStruct); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	// Check for API errors
	if respStruct.Status == statusFail && respStruct.Error != nil {
		code, err := strconv.Atoi(respStruct.Error.Code)
		if err != nil {
			code = 0
		}
		return nil, APIError{
			Code:    code,
			Message: respStruct.Error.Message,
		}
	}

	// Unmarshal into result if provided
	if result != nil {
		if err := xml.Unmarshal(body, result); err != nil {
			return nil, fmt.Errorf("error unmarshaling response: %w", err)
		}
	}

	return body, nil
}

// SetUsePOST sets whether to use POST for API requests.
func (c *Client) SetUsePOST(usePOST bool) {
	c.usePOST = usePOST
}

// createMultipartForm creates a multipart form with file content.
func createMultipartForm(params url.Values, fileField, fileName string, fileContent io.Reader) (*bytes.Buffer, string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add form fields
	for key, values := range params {
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				return nil, "", fmt.Errorf("error writing form field: %w", err)
			}
		}
	}

	// Add file
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		return nil, "", fmt.Errorf("error creating form file: %w", err)
	}
	if _, err := io.Copy(part, fileContent); err != nil {
		return nil, "", fmt.Errorf("error copying file content: %w", err)
	}

	// Close writer
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("error closing multipart writer: %w", err)
	}

	return body, writer.FormDataContentType(), nil
}

// processUploadResponse processes the response from an upload request.
func processUploadResponse(respBody []byte) (map[string]interface{}, error) {
	// Parse response
	var respStruct Response
	if err := xml.Unmarshal(respBody, &respStruct); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	// Check for API errors
	if respStruct.Status == statusFail && respStruct.Error != nil {
		code, err := strconv.Atoi(respStruct.Error.Code)
		if err != nil {
			code = 0
		}
		return nil, APIError{
			Code:    code,
			Message: respStruct.Error.Message,
		}
	}

	// Simple XML to map conversion for the result
	result := make(map[string]interface{})
	var mapData map[string]interface{}
	if err := xml.Unmarshal(respBody, &mapData); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	// Extract relevant data
	for k, v := range mapData {
		if k != "stat" && k != "err" {
			result[k] = v
		}
	}

	return result, nil
}

// Upload uploads a file to RTM with the given parameters.
func (c *Client) Upload(params url.Values, fileField, fileName string, fileContent io.Reader) (map[string]interface{}, error) {
	return c.UploadWithContext(context.Background(), params, fileField, fileName, fileContent)
}

// UploadWithContext uploads a file to RTM with the given parameters and context.
func (c *Client) UploadWithContext(ctx context.Context, params url.Values, fileField, fileName string, fileContent io.Reader) (map[string]interface{}, error) {
	// Apply rate limiting
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("UploadWithContext: rate limit error: %w", err)
	}

	// Add required parameters
	if params == nil {
		params = url.Values{}
	}

	// Add API key and format
	params.Set("api_key", c.APIKey)
	params.Set("format", "rest")

	// Add authentication token if available
	if c.AuthToken != "" {
		params.Set("auth_token", c.AuthToken)
	}

	// Generate signature
	apiSig := c.generateSignature(params)
	params.Set("api_sig", apiSig)

	// Create multipart form
	body, contentType, err := createMultipartForm(params, fileField, fileName, fileContent)
	if err != nil {
		return nil, err
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIURL, body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	// Send request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limit errors (HTTP 503)
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("service temporarily unavailable due to rate limiting")
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: status code %d", resp.StatusCode)
	}

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	return processUploadResponse(respBody)
}

// ConfigureRateLimit allows adjusting the rate limiting parameters.
// This can be useful for different environments or based on API requirements changes.
func (c *Client) ConfigureRateLimit(rate float64, burstLimit int) {
	if c.rateLimiter != nil {
		c.rateLimiter.SetRateLimit(rate, burstLimit)
	} else {
		c.rateLimiter = NewRateLimiter(rate, burstLimit)
	}
}
