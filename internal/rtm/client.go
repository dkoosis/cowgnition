// file: internal/rtm/client.go
package rtm

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
)

const (
	// API endpoints
	defaultAPIEndpoint = "https://api.rememberthemilk.com/services/rest/"
	authEndpoint       = "https://www.rememberthemilk.com/services/auth/"

	// API response format
	responseFormat = "json"

	// API methods
	methodGetFrob      = "rtm.auth.getFrob"
	methodGetToken     = "rtm.auth.getToken"
	methodCheckToken   = "rtm.auth.checkToken"
	methodGetLists     = "rtm.lists.getList"
	methodGetTasks     = "rtm.tasks.getList"
	methodAddTask      = "rtm.tasks.add"
	methodCompleteTask = "rtm.tasks.complete"
	methodGetTags      = "rtm.tags.getList"

	// Auth permission level
	permDelete = "delete" // Allows adding, editing and deleting tasks
)

// Config holds RTM client configuration.
type Config struct {
	// APIKey is the API key from developer.rememberthemilk.com.
	APIKey string

	// SharedSecret is the shared secret from developer.rememberthemilk.com.
	SharedSecret string

	// APIEndpoint is the RTM API endpoint URL (optional, defaults to production).
	APIEndpoint string

	// HTTPClient is an optional custom HTTP client.
	HTTPClient *http.Client

	// AuthToken is the authentication token after successful auth.
	AuthToken string
}

// Client is a Remember The Milk API client.
type Client struct {
	config Config
	logger logging.Logger
}

// NewClient creates a new RTM client with the given configuration.
func NewClient(config Config, logger logging.Logger) *Client {
	// Use default API endpoint if not specified
	if config.APIEndpoint == "" {
		config.APIEndpoint = defaultAPIEndpoint
	}

	// Use default HTTP client if not specified
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	// Use no-op logger if not provided
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	return &Client{
		config: config,
		logger: logger.WithField("component", "rtm_client"),
	}
}

// AuthState represents the authentication state of the client.
type AuthState struct {
	IsAuthenticated bool      `json:"isAuthenticated"`
	Username        string    `json:"username,omitempty"`
	FullName        string    `json:"fullName,omitempty"`
	UserID          string    `json:"userId,omitempty"`
	TokenExpires    time.Time `json:"tokenExpires,omitempty"`
}

// GetAuthState returns the current authentication state.
func (c *Client) GetAuthState(ctx context.Context) (*AuthState, error) {
	// If we don't have a token, we're not authenticated
	if c.config.AuthToken == "" {
		return &AuthState{
			IsAuthenticated: false,
		}, nil
	}

	// Check if the token is valid
	params := map[string]string{}
	resp, err := c.callMethod(ctx, methodCheckToken, params)
	if err != nil {
		// If there's an error, the token might be invalid
		c.logger.Warn("Failed to check auth token", "error", err)
		return &AuthState{
			IsAuthenticated: false,
		}, nil
	}

	// Extract user information from the response
	var auth struct {
		Auth struct {
			User struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Fullname string `json:"fullname"`
			} `json:"user"`
			Token string `json:"token"`
		} `json:"auth"`
	}

	if err := json.Unmarshal(resp, &auth); err != nil {
		return nil, errors.Wrap(err, "failed to parse auth check response")
	}

	return &AuthState{
		IsAuthenticated: true,
		Username:        auth.Auth.User.Username,
		FullName:        auth.Auth.User.Fullname,
		UserID:          auth.Auth.User.ID,
		// Token expiration is not provided by the API, so we don't set it
	}, nil
}

// StartAuthFlow begins the RTM authentication flow.
// It returns a URL that the user needs to visit to authorize the application.
func (c *Client) StartAuthFlow(ctx context.Context) (string, error) {
	// Get a frob from RTM
	params := map[string]string{}
	resp, err := c.callMethod(ctx, methodGetFrob, params)
	if err != nil {
		return "", errors.Wrap(err, "failed to get authentication frob")
	}

	// Extract the frob from the response
	var frobResp struct {
		Frob string `json:"frob"`
	}

	if err := json.Unmarshal(resp, &frobResp); err != nil {
		return "", errors.Wrap(err, "failed to parse frob response")
	}

	frob := frobResp.Frob
	if frob == "" {
		return "", errors.New("empty frob received from API")
	}

	c.logger.Info("Got authentication frob", "frob", frob)

	// Generate the authentication URL
	authParams := map[string]string{
		"api_key": c.config.APIKey,
		"perms":   permDelete,
		"frob":    frob,
	}

	// Sort the parameters alphabetically by key
	var keys []string
	for k := range authParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build the signature base string
	var sb strings.Builder
	sb.WriteString(c.config.SharedSecret)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(authParams[k])
	}

	// Calculate the signature
	h := md5.New()
	io.WriteString(h, sb.String())
	sig := hex.EncodeToString(h.Sum(nil))

	// Build the authentication URL
	authURL, err := url.Parse(authEndpoint)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse auth endpoint URL")
	}

	q := authURL.Query()
	q.Set("api_key", c.config.APIKey)
	q.Set("perms", permDelete)
	q.Set("frob", frob)
	q.Set("api_sig", sig)
	authURL.RawQuery = q.Encode()

	// Store the frob somewhere for CompleteAuthFlow
	// In a real implementation, we'd store this in a secure place
	// For simplicity, we're just returning it as part of the URL for now
	return authURL.String() + "&frob=" + frob, nil
}

// CompleteAuthFlow completes the authentication flow by exchanging the frob for a token.
func (c *Client) CompleteAuthFlow(ctx context.Context, frob string) error {
	if frob == "" {
		return errors.New("frob is required")
	}

	// Exchange the frob for a token
	params := map[string]string{
		"frob": frob,
	}

	resp, err := c.callMethod(ctx, methodGetToken, params)
	if err != nil {
		return errors.Wrap(err, "failed to get auth token")
	}

	// Extract the token from the response
	var tokenResp struct {
		Auth struct {
			Token string `json:"token"`
			User  struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Fullname string `json:"fullname"`
			} `json:"user"`
		} `json:"auth"`
	}

	if err := json.Unmarshal(resp, &tokenResp); err != nil {
		return errors.Wrap(err, "failed to parse token response")
	}

	token := tokenResp.Auth.Token
	if token == "" {
		return errors.New("empty token received from API")
	}

	// Store the token in the client
	c.config.AuthToken = token

	c.logger.Info("Successfully authenticated with RTM",
		"userId", tokenResp.Auth.User.ID,
		"username", tokenResp.Auth.User.Username)

	return nil
}

// SetAuthToken manually sets an authentication token.
// This is useful when loading a stored token.
func (c *Client) SetAuthToken(token string) {
	c.config.AuthToken = token
}

// GetAuthToken returns the current authentication token.
// This is useful when storing the token for later use.
func (c *Client) GetAuthToken() string {
	return c.config.AuthToken
}

// callMethod makes a call to the RTM API.
func (c *Client) callMethod(ctx context.Context, method string, params map[string]string) (json.RawMessage, error) {
	// Create a copy of the parameters
	fullParams := make(map[string]string)
	for k, v := range params {
		fullParams[k] = v
	}

	// Add standard parameters
	fullParams["method"] = method
	fullParams["api_key"] = c.config.APIKey
	fullParams["format"] = responseFormat

	// Add auth token if available
	if c.config.AuthToken != "" && method != methodGetFrob {
		fullParams["auth_token"] = c.config.AuthToken
	}

	// Generate API signature
	sig := c.generateSignature(fullParams)
	fullParams["api_sig"] = sig

	// Build request URL
	apiURL, err := url.Parse(c.config.APIEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse API endpoint URL")
	}

	// Encode parameters as query string
	q := apiURL.Query()
	for k, v := range fullParams {
		q.Set(k, v)
	}
	apiURL.RawQuery = q.Encode()

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}

	// Add headers
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", "CowGnition/0.1.0")

	// Execute request
	c.logger.Debug("Making RTM API call", "method", method, "url", apiURL.String())
	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "HTTP request failed")
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Newf("API returned non-200 status: %d %s, body: %s",
			resp.StatusCode, resp.Status, string(body))
	}

	// Parse the JSON response
	var result struct {
		Rsp struct {
			Stat string `json:"stat"`
			Err  *struct {
				Code string `json:"code"`
				Msg  string `json:"msg"`
			} `json:"err,omitempty"`
			*json.RawMessage
		} `json:"rsp"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.Wrap(err, "failed to parse API response")
	}

	// Check for API error
	if result.Rsp.Stat != "ok" {
		if result.Rsp.Err != nil {
			return nil, errors.Newf("API error: %s - %s", result.Rsp.Err.Code, result.Rsp.Err.Msg)
		}
		return nil, errors.Newf("API returned non-ok status: %s", result.Rsp.Stat)
	}

	// Return the raw response for further parsing
	return body, nil
}

// generateSignature generates an API signature for the given parameters.
func (c *Client) generateSignature(params map[string]string) string {
	// Sort the parameters alphabetically by key
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build the signature base string
	var sb strings.Builder
	sb.WriteString(c.config.SharedSecret)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}

	// Calculate the signature
	h := md5.New()
	io.WriteString(h, sb.String())
	return hex.EncodeToString(h.Sum(nil))
}

// Task represents a Remember The Milk task.
type Task struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	URL           string    `json:"url,omitempty"`
	DueDate       time.Time `json:"dueDate,omitempty"`
	StartDate     time.Time `json:"startDate,omitempty"`
	CompletedDate time.Time `json:"completedDate,omitempty"`
	Priority      int       `json:"priority,omitempty"` // 1 (highest) to 4 (no priority)
	Postponed     int       `json:"postponed,omitempty"`
	Estimate      string    `json:"estimate,omitempty"`
	LocationID    string    `json:"locationId,omitempty"`
	LocationName  string    `json:"locationName,omitempty"`
	Tags          []string  `json:"tags,omitempty"`
	Notes         []Note    `json:"notes,omitempty"`
	ListID        string    `json:"listId"`
	ListName      string    `json:"listName,omitempty"`
}

// Note represents a note attached to a task.
type Note struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

// TaskList represents a Remember The Milk task list.
type TaskList struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Deleted    bool   `json:"deleted"`
	Locked     bool   `json:"locked"`
	Archived   bool   `json:"archived"`
	Position   int    `json:"position"`
	SmartList  bool   `json:"smartList"`
	TasksCount int    `json:"tasksCount,omitempty"` // Not provided by the API, but useful for our pipeline architecture
}

// Tag represents a Remember The Milk tag.
type Tag struct {
	Name       string `json:"name"`
	TasksCount int    `json:"tasksCount,omitempty"` // Not provided by the API, but useful for our pipeline architecture
}

// GetLists retrieves all the task lists for the user.
func (c *Client) GetLists(ctx context.Context) ([]TaskList, error) {
	params := map[string]string{}
	resp, err := c.callMethod(ctx, methodGetLists, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get task lists")
	}

	// Parse the response
	var listsResp struct {
		Rsp struct {
			Lists struct {
				List []struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Deleted  string `json:"deleted"`
					Locked   string `json:"locked"`
					Archived string `json:"archived"`
					Position string `json:"position"`
					Smart    string `json:"smart"`
				} `json:"list"`
			} `json:"lists"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &listsResp); err != nil {
		return nil, errors.Wrap(err, "failed to parse lists response")
	}

	// Convert to our TaskList type
	var lists []TaskList
	for _, l := range listsResp.Rsp.Lists.List {
		list := TaskList{
			ID:        l.ID,
			Name:      l.Name,
			Deleted:   l.Deleted == "1",
			Locked:    l.Locked == "1",
			Archived:  l.Archived == "1",
			SmartList: l.Smart == "1",
		}

		// Parse position
		if l.Position != "" {
			fmt.Sscanf(l.Position, "%d", &list.Position)
		}

		lists = append(lists, list)
	}

	return lists, nil
}

// GetTasks retrieves tasks based on a filter.
func (c *Client) GetTasks(ctx context.Context, filter string) ([]Task, error) {
	params := map[string]string{}
	if filter != "" {
		params["filter"] = filter
	}

	resp, err := c.callMethod(ctx, methodGetTasks, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get tasks")
	}

	// The RTM API response for tasks is quite complex, with nested structures
	// This is a simplified parsing example
	var tasksResp struct {
		Rsp struct {
			Tasks struct {
				List []struct {
					ID         string `json:"id"`
					Name       string `json:"name,omitempty"`
					Taskseries []struct {
						ID   string `json:"id"`
						Name string `json:"name"`
						URL  string `json:"url,omitempty"`
						Tags struct {
							Tag []string `json:"tag,omitempty"`
						} `json:"tags"`
						Notes struct {
							Note []struct {
								ID      string `json:"id"`
								Title   string `json:"title"`
								Body    string `json:"$t"` // Special field name for the note text
								Created string `json:"created"`
							} `json:"note,omitempty"`
						} `json:"notes"`
						Task []struct {
							ID        string `json:"id"`
							Due       string `json:"due,omitempty"`
							Added     string `json:"added,omitempty"`
							Completed string `json:"completed,omitempty"`
							Deleted   string `json:"deleted,omitempty"`
							Priority  string `json:"priority,omitempty"`
							Postponed string `json:"postponed,omitempty"`
							Estimate  string `json:"estimate,omitempty"`
						} `json:"task"`
						LocationID   string `json:"location_id,omitempty"`
						LocationName string `json:"location,omitempty"`
					} `json:"taskseries"`
				} `json:"list"`
			} `json:"tasks"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &tasksResp); err != nil {
		return nil, errors.Wrap(err, "failed to parse tasks response")
	}

	// Convert to our Task type
	var tasks []Task
	for _, list := range tasksResp.Rsp.Tasks.List {
		for _, series := range list.Taskseries {
			for _, t := range series.Task {
				// Skip deleted tasks
				if t.Deleted != "" {
					continue
				}

				task := Task{
					ID:           series.ID + "_" + t.ID, // Combine taskseries_id and task_id
					Name:         series.Name,
					URL:          series.URL,
					LocationID:   series.LocationID,
					LocationName: series.LocationName,
					ListID:       list.ID,
					ListName:     list.Name,
				}

				// Parse date fields
				if t.Due != "" {
					dueTime, err := time.Parse("2006-01-02T15:04:05Z", t.Due)
					if err == nil {
						task.DueDate = dueTime
					}
				}

				if t.Added != "" {
					startTime, err := time.Parse("2006-01-02T15:04:05Z", t.Added)
					if err == nil {
						task.StartDate = startTime
					}
				}

				if t.Completed != "" {
					completedTime, err := time.Parse("2006-01-02T15:04:05Z", t.Completed)
					if err == nil {
						task.CompletedDate = completedTime
					}
				}

				// Parse priority
				if t.Priority != "" {
					fmt.Sscanf(t.Priority, "%d", &task.Priority)
				}

				// Parse postponed count
				if t.Postponed != "" {
					fmt.Sscanf(t.Postponed, "%d", &task.Postponed)
				}

				task.Estimate = t.Estimate

				// Parse tags
				if len(series.Tags.Tag) > 0 {
					task.Tags = series.Tags.Tag
				}

				// Parse notes
				if len(series.Notes.Note) > 0 {
					for _, n := range series.Notes.Note {
						note := Note{
							ID:    n.ID,
							Title: n.Title,
							Text:  n.Body,
						}

						if n.Created != "" {
							createdTime, err := time.Parse("2006-01-02T15:04:05Z", n.Created)
							if err == nil {
								note.CreatedAt = createdTime
							}
						}

						task.Notes = append(task.Notes, note)
					}
				}

				tasks = append(tasks, task)
			}
		}
	}

	return tasks, nil
}

// CreateTask creates a new task.
func (c *Client) CreateTask(ctx context.Context, name string, listID string) (*Task, error) {
	params := map[string]string{
		"name": name,
	}

	// Add list_id if provided
	if listID != "" {
		params["list_id"] = listID
	}

	resp, err := c.callMethod(ctx, methodAddTask, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create task")
	}

	// Parse the response to get the created task
	var createResp struct {
		Rsp struct {
			List struct {
				ID         string `json:"id"`
				Taskseries struct {
					ID   string `json:"id"`
					Name string `json:"name"`
					Task struct {
						ID    string `json:"id"`
						Added string `json:"added"`
						Due   string `json:"due"`
					} `json:"task"`
				} `json:"taskseries"`
			} `json:"list"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &createResp); err != nil {
		return nil, errors.Wrap(err, "failed to parse create task response")
	}

	// Create a task object from the response
	task := &Task{
		ID:     createResp.Rsp.List.Taskseries.ID + "_" + createResp.Rsp.List.Taskseries.Task.ID,
		Name:   createResp.Rsp.List.Taskseries.Name,
		ListID: createResp.Rsp.List.ID,
	}

	// Parse date fields
	if createResp.Rsp.List.Taskseries.Task.Added != "" {
		startTime, err := time.Parse("2006-01-02T15:04:05Z", createResp.Rsp.List.Taskseries.Task.Added)
		if err == nil {
			task.StartDate = startTime
		}
	}

	if createResp.Rsp.List.Taskseries.Task.Due != "" {
		dueTime, err := time.Parse("2006-01-02T15:04:05Z", createResp.Rsp.List.Taskseries.Task.Due)
		if err == nil {
			task.DueDate = dueTime
		}
	}

	return task, nil
}

// CompleteTask marks a task as completed.
func (c *Client) CompleteTask(ctx context.Context, taskID string) error {
	// RTM API requires task ID in the format of "taskseries_id,task_id"
	// Our task ID is in the format "taskseries_id_task_id", so we need to split it
	parts := strings.Split(taskID, "_")
	if len(parts) != 2 {
		return errors.Newf("invalid task ID format: %s", taskID)
	}

	// taskSeriesID := parts[0]
	// taskID := parts[1]

	taskIDForAPI := strings.Join(parts, ",")

	// Set up the parameters
	params := map[string]string{
		"task_id": taskIDForAPI,
	}

	// Make the API call
	_, err := c.callMethod(ctx, methodCompleteTask, params)
	if err != nil {
		return errors.Wrap(err, "failed to complete task")
	}

	return nil
}

// GetTags retrieves all the tags for the user.
func (c *Client) GetTags(ctx context.Context) ([]Tag, error) {
	params := map[string]string{}
	resp, err := c.callMethod(ctx, methodGetTags, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get tags")
	}

	// Parse the response
	var tagsResp struct {
		Rsp struct {
			Tags struct {
				Tag []struct {
					Name string `json:"name"`
				} `json:"tag"`
			} `json:"tags"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &tagsResp); err != nil {
		return nil, errors.Wrap(err, "failed to parse tags response")
	}

	// Convert to our Tag type
	var tags []Tag
	for _, t := range tagsResp.Rsp.Tags.Tag {
		tag := Tag{
			Name: t.Name,
		}
		tags = append(tags, tag)
	}

	return tags, nil
}
