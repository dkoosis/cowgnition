// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import (
	"bytes"
	"crypto/md5"
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
}

// NewClient creates a new RTM client with the provided API key and shared secret.
func NewClient(apiKey, sharedSecret string) *Client {
	return &Client{
		APIKey:       apiKey,
		SharedSecret: sharedSecret,
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
		APIURL:       "https://api.rememberthemilk.com/services/rest/",
		usePOST:      false, // Default to GET requests.
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

	// Calculate MD5 hash
	hash := md5.Sum([]byte(sb.String()))
	return fmt.Sprintf("%x", hash)
}

// callMethod calls an RTM API method with the provided parameters.
func (c *Client) callMethod(method string, params url.Values) ([]byte, error) {
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
	req, err := http.NewRequest(http.MethodPost, c.APIURL, bytes.NewBufferString(params.Encode()))
	if err != nil {
		// SUGGESTION (Ambiguous): Improve error message for clarity.
		return nil, fmt.Errorf("callMethod: failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		// SUGGESTION (Ambiguous): Improve error message for clarity.
		return nil, fmt.Errorf("callMethod: failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// SUGGESTION (Ambiguous): Improve error message for clarity.
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
		// SUGGESTION (Ambiguous): Improve error message for clarity.
		return fmt.Errorf("checkResponseForError: failed to parse response: %w", err)
	}

	if respStruct.Stat == "fail" {
		// SUGGESTION (Readability): Added "RTM" for context.
		return fmt.Errorf("checkResponseForError: RTM API error %s: %s", respStruct.Err.Code, respStruct.Err.Msg)
	}

	return nil
}

// GetFrob gets a frob from the RTM API for authentication.
func (c *Client) GetFrob() (string, error) {
	resp, err := c.callMethod("rtm.auth.getFrob", nil)
	if err != nil {
		return "", err
	}

	var result struct {
		Frob string `xml:"frob"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		// SUGGESTION (Ambiguous): Improve error message for clarity.
		return "", fmt.Errorf("GetFrob: failed to parse frob response: %w", err)
	}

	return result.Frob, nil
}

// GetToken exchanges a frob for an authentication token.
func (c *Client) GetToken(frob string) (string, error) {
	params := url.Values{}
	params.Set("frob", frob)

	resp, err := c.callMethod("rtm.auth.getToken", params)
	if err != nil {
		return "", err
	}

	var result struct {
		Auth struct {
			Token string `xml:"token"`
		} `xml:"auth"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		// SUGGESTION (Ambiguous): Improve error message for clarity.
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

	_, err := c.callMethod("rtm.auth.checkToken", nil)
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
	resp, err := c.callMethod("rtm.timelines.create", nil)
	if err != nil {
		return "", err
	}

	var result struct {
		Timeline string `xml:"timeline"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		// SUGGESTION (Ambiguous): Improve error message for clarity.
		return "", fmt.Errorf("CreateTimeline: failed to parse timeline response: %w", err)
	}

	return result.Timeline, nil
}

// GetLists gets all lists from the RTM API.
func (c *Client) GetLists() ([]byte, error) {
	return c.callMethod("rtm.lists.getList", nil)
}

// GetTasks gets tasks from the RTM API with optional filtering.
func (c *Client) GetTasks(filter string) ([]byte, error) {
	params := url.Values{}
	if filter != "" {
		params.Set("filter", filter)
	}

	return c.callMethod("rtm.tasks.getList", params)
}

// AddTask adds a new task to the specified list.
func (c *Client) AddTask(timeline, name, listID string) ([]byte, error) {
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("name", name)
	if listID != "" {
		params.Set("list_id", listID)
	}

	return c.callMethod("rtm.tasks.add", params)
}

// CompleteTask marks a task as complete.
func (c *Client) CompleteTask(timeline, listID, taskseriesID, taskID string) ([]byte, error) {
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)

	return c.callMethod("rtm.tasks.complete", params)
}

// DeleteTask deletes a task.
func (c *Client) DeleteTask(timeline, listID, taskseriesID, taskID string) ([]byte, error) {
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)

	return c.callMethod("rtm.tasks.delete", params)
}

// SetTaskDueDate sets the due date for a task.
func (c *Client) SetTaskDueDate(timeline, listID, taskseriesID, taskID, due string) ([]byte, error) {
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("due", due)

	return c.callMethod("rtm.tasks.setDueDate", params)
}

// SetTaskPriority sets the priority for a task.
func (c *Client) SetTaskPriority(timeline, listID, taskseriesID, taskID, priority string) ([]byte, error) {
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("priority", priority)

	return c.callMethod("rtm.tasks.setPriority", params)
}

// AddTags adds tags to a task.
func (c *Client) AddTags(timeline, listID, taskseriesID, taskID, tags string) ([]byte, error) {
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("tags", tags)

	return c.callMethod("rtm.tasks.addTags", params)
}

// RemoveTags removes tags from a task.
func (c *Client) RemoveTags(timeline, listID, taskseriesID, taskID, tags string) ([]byte, error) {
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("tags", tags)

	return c.callMethod("rtm.tasks.removeTags", params)
}

// GetTags gets all tags from the RTM API.
func (c *Client) GetTags() ([]byte, error) {
	return c.callMethod("rtm.tags.getList", nil)
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

	// Determine request method
	var req *http.Request
	var err error
	if c.usePOST {
		// Create POST request
		req, err = http.NewRequest(http.MethodPost, c.APIURL, strings.NewReader(params.Encode()))
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		// Create GET request
		req, err = http.NewRequest(http.MethodGet, c.APIURL+"?"+params.Encode(), nil)
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

// Upload uploads a file to RTM with the given parameters.
func (c *Client) Upload(params url.Values, fileField, fileName string, fileContent io.Reader) (map[string]interface{}, error) {
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
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body) // Fixed: Use multipart.NewWriter instead of http.NewWriter

	// Add form fields
	for key, values := range params {
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				return nil, fmt.Errorf("error writing form field: %w", err)
			}
		}
	}

	// Add file
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		return nil, fmt.Errorf("error creating form file: %w", err)
	}
	if _, err := io.Copy(part, fileContent); err != nil {
		return nil, fmt.Errorf("error copying file content: %w", err)
	}

	// Close writer
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("error closing multipart writer: %w", err)
	}

	// Create request
	req, err := http.NewRequest(http.MethodPost, c.APIURL, body)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: status code %d", resp.StatusCode)
	}

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

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
	// In a real implementation, this would be more sophisticated
	result := make(map[string]interface{})
	var mapData map[string]interface{}
	if err := xml.Unmarshal(respBody, &mapData); err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	// Extract relevant data
	// This is a simple implementation; in practice, would need recursive traversal
	for k, v := range mapData {
		if k != "stat" && k != "err" {
			result[k] = v
		}
	}

	return result, nil
}
