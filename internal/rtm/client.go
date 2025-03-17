// Package rtm provides integration with the Remember The Milk API.
package rtm

import (
	"bytes"
	"crypto/md5"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
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
}

// NewClient creates a new RTM client with the provided API key and shared secret.
func NewClient(apiKey, sharedSecret string) *Client {
	return &Client{
		APIKey:       apiKey,
		SharedSecret: sharedSecret,
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
		APIURL:       "https://api.rememberthemilk.com/services/rest/",
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
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
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
		return fmt.Errorf("error parsing response: %w", err)
	}

	if respStruct.Stat == "fail" {
		return fmt.Errorf("RTM API error %s: %s", respStruct.Err.Code, respStruct.Err.Msg)
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
		return "", fmt.Errorf("error parsing frob response: %w", err)
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
		return "", fmt.Errorf("error parsing token response: %w", err)
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
		return "", fmt.Errorf("error parsing timeline response: %w", err)
	}

	return result.Timeline, nil
}

// GetLists gets all lists from the RTM API.
func (c *Client) GetLists() ([]byte, error) {
	return c.callMethod("rtm.lists.getList", nil)
}

// GetTasks gets tasks from the RTM API with optional filtering.
func (c *Client) GetTasks(listID, filter string) ([]byte, error) {
	params := url.Values{}
	if listID != "" {
		params.Set("list_id", listID)
	}
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
