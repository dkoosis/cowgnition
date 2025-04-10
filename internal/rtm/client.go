// file: internal/rtm/client.go
package rtm

import (
	"context"
	"crypto/md5" //nolint:gosec // Required by RTM API for request signing.
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
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // Import MCP errors.
)

const (
	// API endpoints.
	defaultAPIEndpoint = "https://api.rememberthemilk.com/services/rest/"
	authEndpoint       = "https://www.rememberthemilk.com/services/auth/"

	// API response format.
	responseFormat = "json"

	// API methods.
	methodGetFrob      = "rtm.auth.getFrob"
	methodGetToken     = "rtm.auth.getToken"   // nolint:gosec
	methodCheckToken   = "rtm.auth.checkToken" //nolint:gosec
	methodGetLists     = "rtm.lists.getList"
	methodGetTasks     = "rtm.tasks.getList"
	methodAddTask      = "rtm.tasks.add"
	methodCompleteTask = "rtm.tasks.complete"
	methodGetTags      = "rtm.tags.getList"

	// Auth permission level.
	permDelete = "delete" // Allows adding, editing and deleting tasks.
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
	// Use default API endpoint if not specified.
	if config.APIEndpoint == "" {
		config.APIEndpoint = defaultAPIEndpoint
	}

	// Use default HTTP client if not specified.
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	// Use no-op logger if not provided.
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
	// If we don't have a token, we're not authenticated.
	if c.config.AuthToken == "" {
		return &AuthState{
			IsAuthenticated: false,
		}, nil
	}

	// Check if the token is valid.
	params := map[string]string{}
	resp, err := c.callMethod(ctx, methodCheckToken, params)
	// Check for specific RTM API errors indicating invalid token.
	var rtmErr *mcperrors.RTMError
	if err != nil && errors.As(err, &rtmErr) && rtmErr.Code == 98 { // RTM Error code 98: Invalid auth token.
		c.logger.Info("Auth token is invalid according to RTM API.", "rtmErrorCode", rtmErr.Code, "rtmErrorMessage", rtmErr.Message)
		return &AuthState{
			IsAuthenticated: false,
		}, nil
	} else if err != nil {
		// Log other errors but still treat as potentially unauthenticated.
		c.logger.Warn("Failed to check auth token validity, assuming invalid.", "error", err)
		return &AuthState{
			IsAuthenticated: false,
		}, nil
	}

	// Extract user information from the response.
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

	// Use errors.Wrap for context preservation.
	if err := json.Unmarshal(resp, &auth); err != nil {
		return nil, errors.Wrap(err, "failed to parse auth check response")
	}

	return &AuthState{
		IsAuthenticated: true,
		Username:        auth.Auth.User.Username,
		FullName:        auth.Auth.User.Fullname,
		UserID:          auth.Auth.User.ID,
		// Token expiration is not provided by the API, so we don't set it.
	}, nil
}

// StartAuthFlow begins the RTM authentication flow.
// It returns a URL that the user needs to visit to authorize the application.
func (c *Client) StartAuthFlow(ctx context.Context) (string, error) {
	// Get a frob from RTM.
	params := map[string]string{}
	resp, err := c.callMethod(ctx, methodGetFrob, params)
	if err != nil {
		// Wrap the original error.
		return "", errors.Wrap(err, "failed to get authentication frob")
	}

	// Extract the frob from the response.
	var frobResp struct {
		Frob string `json:"frob"`
	}

	// Use errors.Wrap for context preservation.
	if err := json.Unmarshal(resp, &frobResp); err != nil {
		return "", errors.Wrap(err, "failed to parse frob response")
	}

	frob := frobResp.Frob
	if frob == "" {
		// Use a more specific RTM error if available, or wrap a standard error.
		return "", mcperrors.NewRTMError(mcperrors.ErrRTMInvalidResponse, "empty frob received from API", nil, nil)
	}

	c.logger.Info("Got authentication frob.", "frob", frob)

	// Generate the authentication URL.
	authParams := map[string]string{
		"api_key": c.config.APIKey,
		"perms":   permDelete,
		"frob":    frob,
	}

	// Sort the parameters alphabetically by key.
	var keys []string
	for k := range authParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build the signature base string.
	var sb strings.Builder
	sb.WriteString(c.config.SharedSecret)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(authParams[k])
	}

	// Calculate the signature.
	h := md5.New() //nolint:gosec // Required by RTM API for request signing.
	// Error handling for io.WriteString is generally omitted for strings on md5.Hash.
	_, _ = io.WriteString(h, sb.String())
	sig := hex.EncodeToString(h.Sum(nil))

	// Build the authentication URL.
	authURL, err := url.Parse(authEndpoint)
	if err != nil {
		// Use errors.Wrap for context preservation.
		return "", errors.Wrap(err, "failed to parse auth endpoint URL")
	}

	q := authURL.Query()
	q.Set("api_key", c.config.APIKey)
	q.Set("perms", permDelete)
	q.Set("frob", frob)
	q.Set("api_sig", sig)
	authURL.RawQuery = q.Encode()

	// Store the frob somewhere for CompleteAuthFlow.
	// In a real implementation, we'd store this in a secure place.
	// For simplicity, we're just returning it as part of the URL for now.
	return authURL.String() + "&frob=" + frob, nil
}

// CompleteAuthFlow completes the authentication flow by exchanging the frob for a token.
func (c *Client) CompleteAuthFlow(ctx context.Context, frob string) error {
	if frob == "" {
		// Use a specific error.
		return mcperrors.NewRTMError(mcperrors.ErrAuthMissing, "frob is required to complete auth flow", nil, nil)
	}

	// Exchange the frob for a token.
	params := map[string]string{
		"frob": frob,
	}

	resp, err := c.callMethod(ctx, methodGetToken, params)
	if err != nil {
		// Wrap the original error.
		return errors.Wrap(err, "failed to get auth token")
	}

	// Extract the token from the response.
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

	// Use errors.Wrap for context preservation.
	if err := json.Unmarshal(resp, &tokenResp); err != nil {
		return errors.Wrap(err, "failed to parse token response")
	}

	token := tokenResp.Auth.Token
	if token == "" {
		// Use a specific error.
		return mcperrors.NewRTMError(mcperrors.ErrRTMInvalidResponse, "empty token received from API", nil, nil)
	}

	// Store the token in the client.
	c.config.AuthToken = token

	c.logger.Info("Successfully authenticated with RTM.",
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
	// Create a copy of the parameters.
	fullParams := make(map[string]string)
	for k, v := range params {
		fullParams[k] = v
	}

	// Add standard parameters.
	fullParams["method"] = method
	fullParams["api_key"] = c.config.APIKey
	fullParams["format"] = responseFormat

	// Add auth token if available and method requires it.
	if c.config.AuthToken != "" && method != methodGetFrob && method != methodGetToken {
		fullParams["auth_token"] = c.config.AuthToken
	}

	// Generate API signature.
	sig := c.generateSignature(fullParams)
	fullParams["api_sig"] = sig

	// Build request URL.
	apiURL, err := url.Parse(c.config.APIEndpoint)
	if err != nil {
		// Use errors.Wrap for context preservation.
		return nil, errors.Wrap(err, "failed to parse API endpoint URL")
	}

	// Encode parameters as query string.
	q := apiURL.Query()
	for k, v := range fullParams {
		q.Set(k, v)
	}
	apiURL.RawQuery = q.Encode()

	// Create HTTP request.
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		// Use errors.Wrap for context preservation.
		return nil, errors.Wrap(err, "failed to create HTTP request")
	}

	// Add headers.
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", "CowGnition/0.1.0")

	// Execute request.
	c.logger.Debug("Making RTM API call.", "method", method, "url", apiURL.String())
	resp, err := c.config.HTTPClient.Do(req)
	if err != nil {
		// Use errors.Wrap for context preservation with all context details in a single call.
		return nil, errors.Wrapf(err, "RTM HTTP request failed (method: %s, url: %s)", method, apiURL.String())
	}
	defer resp.Body.Close()

	// Read response body.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// Use errors.Wrap for context preservation.
		return nil, errors.Wrap(err, "failed to read RTM response body")
	}

	// Check HTTP status.
	if resp.StatusCode != http.StatusOK {
		// Return a more specific RTM error.
		errCtx := map[string]interface{}{
			"statusCode": resp.StatusCode,
			"status":     resp.Status,
			"body":       string(body),
			"rtm_method": method,
		}
		return nil, mcperrors.NewRTMError(mcperrors.ErrRTMAPIFailure,
			fmt.Sprintf("API returned non-200 status: %d", resp.StatusCode),
			nil, // No underlying Go error here, it's an API status issue.
			errCtx)
	}

	// Parse the JSON response structure to check for RTM API errors.
	var result struct {
		Rsp struct {
			Stat string `json:"stat"`
			Err  *struct {
				Code string `json:"code"`
				Msg  string `json:"msg"`
			} `json:"err,omitempty"`
			// Capture the rest of the response within Rsp if needed, but we need RawMessage.
		} `json:"rsp"`
	}

	// We need the raw body later, so unmarshal into a temporary structure first.
	if err := json.Unmarshal(body, &result); err != nil {
		// Use errors.Wrap for context preservation.
		return nil, errors.Wrap(err, "failed to parse RTM API response structure")
	}

	// Check for API error status.
	if result.Rsp.Stat != "ok" {
		if result.Rsp.Err != nil {
			// Return a specific RTM error with code and message.
			rtmErrCode := 0                                       // Default to generic RTM failure.
			_, err := fmt.Sscan(result.Rsp.Err.Code, &rtmErrCode) // Use blank identifier if count isn't needed
			if err != nil {
				// Handle the error appropriately - log it, return an error, etc.
				// Example:
				return nil, mcperrors.NewRTMError(mcperrors.ErrRTMInvalidResponse,
					fmt.Sprintf("RTM API Error: %s (failed to parse error code '%s')", result.Rsp.Err.Msg, result.Rsp.Err.Code),
					err, // Include the Sscan error
					errCtx)
			}
			errCtx := map[string]interface{}{
				"rtmErrorCode":    result.Rsp.Err.Code,
				"rtmErrorMessage": result.Rsp.Err.Msg,
				"rtm_method":      method,
			}
			// Use mcperrors.ErrRTMAPIFailure unless a more specific code exists.
			// Example: Map RTM error code 98 (Invalid auth token) to our internal AuthError.
			if rtmErrCode == 98 {
				return nil, mcperrors.NewAuthError(
					fmt.Sprintf("RTM API Error: %s", result.Rsp.Err.Msg),
					nil,
					errCtx,
				)
			}
			return nil, mcperrors.NewRTMError(mcperrors.ErrRTMAPIFailure,
				fmt.Sprintf("RTM API Error: %s", result.Rsp.Err.Msg),
				nil, // No underlying Go error here, it's an RTM API level error.
				errCtx)
		}
		// Use a specific RTM error if stat is not "ok" but no err block exists.
		return nil, mcperrors.NewRTMError(mcperrors.ErrRTMInvalidResponse,
			fmt.Sprintf("RTM API returned non-ok status '%s' without error details", result.Rsp.Stat),
			nil,
			map[string]interface{}{"rtm_method": method})
	}

	// Return the original raw response body, as it contains the actual data needed by callers.
	return body, nil
}

// generateSignature generates an API signature for the given parameters.
func (c *Client) generateSignature(params map[string]string) string {
	// Sort the parameters alphabetically by key.
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build the signature base string.
	var sb strings.Builder
	sb.WriteString(c.config.SharedSecret)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}

	// Calculate the signature.
	h := md5.New() //nolint:gosec // Required by RTM API for request signing.
	// Error handling for io.WriteString is generally omitted for strings on md5.Hash.
	_, _ = io.WriteString(h, sb.String())
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
	Priority      int       `json:"priority,omitempty"` // 1 (highest) to 4 (no priority).
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
	TasksCount int    `json:"tasksCount,omitempty"` // Not provided by the API, but useful for our pipeline architecture.
}

// Tag represents a Remember The Milk tag.
type Tag struct {
	Name       string `json:"name"`
	TasksCount int    `json:"tasksCount,omitempty"` // Not provided by the API, but useful for our pipeline architecture.
}

// GetLists retrieves all the task lists for the user.
func (c *Client) GetLists(ctx context.Context) ([]TaskList, error) {
	params := map[string]string{}
	resp, err := c.callMethod(ctx, methodGetLists, params)
	if err != nil {
		// Use errors.Wrap for context preservation.
		return nil, errors.Wrap(err, "failed to get task lists")
	}

	// Parse the response.
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

	// Use errors.Wrap for context preservation.
	if err := json.Unmarshal(resp, &listsResp); err != nil {
		return nil, errors.Wrap(err, "failed to parse lists response")
	}

	// Convert to our TaskList type.
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

		// Parse position.
		if l.Position != "" {
			// Error handling for Sscanf can be added if needed.
			_, _ = fmt.Sscanf(l.Position, "%d", &list.Position)
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
		// Use errors.Wrap for context preservation.
		return nil, errors.Wrap(err, "failed to get tasks")
	}

	// The RTM API response for tasks is quite complex, with nested structures.
	// This is a simplified parsing example.
	var tasksResp struct {
		Rsp struct {
			Tasks struct {
				List []struct {
					ID         string `json:"id"`
					Name       string `json:"name,omitempty"` // List name might be omitted if filtering by list.
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
								Body    string `json:"$t"` // Special field name for the note text.
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
						LocationName string `json:"location,omitempty"` // RTM seems to use 'location' for name in some contexts.
					} `json:"taskseries"`
				} `json:"list"`
			} `json:"tasks"`
		} `json:"rsp"`
	}

	// Use errors.Wrap for context preservation.
	if err := json.Unmarshal(resp, &tasksResp); err != nil {
		return nil, errors.Wrap(err, "failed to parse tasks response")
	}

	// Convert to our Task type.
	var tasks []Task
	for _, list := range tasksResp.Rsp.Tasks.List {
		for _, series := range list.Taskseries {
			for _, t := range series.Task {
				// Skip deleted tasks.
				if t.Deleted != "" {
					continue
				}

				task := Task{
					ID:           fmt.Sprintf("%s_%s", series.ID, t.ID), // Combine taskseries_id and task_id consistently.
					Name:         series.Name,
					URL:          series.URL,
					LocationID:   series.LocationID,
					LocationName: series.LocationName,
					ListID:       list.ID,
					ListName:     list.Name, // Use list name from outer loop if available.
				}

				// Parse date fields.
				if t.Due != "" {
					// Add robust error handling for time parsing.
					dueTime, err := time.Parse("2006-01-02T15:04:05Z", t.Due)
					if err == nil {
						task.DueDate = dueTime
					} else {
						c.logger.Warn("Failed to parse task due date.", "rawDate", t.Due, "taskId", task.ID, "error", err)
					}
				}

				if t.Added != "" {
					startTime, err := time.Parse("2006-01-02T15:04:05Z", t.Added)
					if err == nil {
						task.StartDate = startTime
					} else {
						c.logger.Warn("Failed to parse task added date.", "rawDate", t.Added, "taskId", task.ID, "error", err)
					}
				}

				if t.Completed != "" {
					completedTime, err := time.Parse("2006-01-02T15:04:05Z", t.Completed)
					if err == nil {
						task.CompletedDate = completedTime
					} else {
						c.logger.Warn("Failed to parse task completed date.", "rawDate", t.Completed, "taskId", task.ID, "error", err)
					}
				}

				// Parse priority.
				if t.Priority != "" && t.Priority != "N" { // "N" means no priority.
					// Add robust error handling for Sscanf.
					_, err := fmt.Sscan(t.Priority, &task.Priority)
					if err != nil {
						c.logger.Warn("Failed to parse task priority.", "rawPriority", t.Priority, "taskId", task.ID, "error", err)
					}
				}

				// Parse postponed count.
				if t.Postponed != "" {
					// Add robust error handling for Sscanf.
					_, err := fmt.Sscan(t.Postponed, &task.Postponed)
					if err != nil {
						c.logger.Warn("Failed to parse task postponed count.", "rawPostponed", t.Postponed, "taskId", task.ID, "error", err)
					}
				}

				task.Estimate = t.Estimate

				// Parse tags.
				if len(series.Tags.Tag) > 0 {
					task.Tags = series.Tags.Tag
				}

				// Parse notes.
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
							} else {
								c.logger.Warn("Failed to parse note created date.", "rawDate", n.Created, "noteId", n.ID, "taskId", task.ID, "error", err)
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
		// RTM's 'parse' parameter might be useful here if smart syntax needs enabling, defaults to off via API? Check docs.
		// "parse": "1", // If smart add syntax is desired.
	}

	// Add list_id if provided.
	if listID != "" {
		params["list_id"] = listID
	}

	// RTM API requires a timeline for adding tasks.
	timeline, err := c.createTimeline(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create timeline for adding task")
	}
	params["timeline"] = timeline

	resp, err := c.callMethod(ctx, methodAddTask, params)
	if err != nil {
		// Use errors.Wrap for context preservation.
		return nil, errors.Wrap(err, "failed to create task")
	}

	// Parse the response to get the created task.
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
						Due   string `json:"due,omitempty"` // Due might be empty.
					} `json:"task"`
				} `json:"taskseries"`
			} `json:"list"`
		} `json:"rsp"`
	}

	// Use errors.Wrap for context preservation.
	if err := json.Unmarshal(resp, &createResp); err != nil {
		return nil, errors.Wrap(err, "failed to parse create task response")
	}

	// Create a task object from the response.
	task := &Task{
		ID:     fmt.Sprintf("%s_%s", createResp.Rsp.List.Taskseries.ID, createResp.Rsp.List.Taskseries.Task.ID),
		Name:   createResp.Rsp.List.Taskseries.Name,
		ListID: createResp.Rsp.List.ID,
	}

	// Parse date fields.
	if createResp.Rsp.List.Taskseries.Task.Added != "" {
		startTime, err := time.Parse("2006-01-02T15:04:05Z", createResp.Rsp.List.Taskseries.Task.Added)
		if err == nil {
			task.StartDate = startTime
		} else {
			c.logger.Warn("Failed to parse created task added date.", "rawDate", createResp.Rsp.List.Taskseries.Task.Added, "taskId", task.ID, "error", err)
		}
	}

	// Due date might not be set if smart syntax wasn't used or didn't include a date.
	if createResp.Rsp.List.Taskseries.Task.Due != "" {
		dueTime, err := time.Parse("2006-01-02T15:04:05Z", createResp.Rsp.List.Taskseries.Task.Due)
		if err == nil {
			task.DueDate = dueTime
		} else {
			c.logger.Warn("Failed to parse created task due date.", "rawDate", createResp.Rsp.List.Taskseries.Task.Due, "taskId", task.ID, "error", err)
		}
	}

	return task, nil
}

// CompleteTask marks a task as completed.
// NOTE: This function requires the listID associated with the task, which is not part of the taskID string.
// Consider passing listID as an argument or fetching the task first to get it.
func (c *Client) CompleteTask(ctx context.Context, listID, taskID string) error {
	// RTM API requires task ID in the format of "taskseries_id,task_id".
	// Our task ID is in the format "taskseries_id_task_id", so we need to split it.
	parts := strings.Split(taskID, "_")
	if len(parts) != 2 {
		// Use a more specific error type if available, e.g., InvalidArgumentError.
		return mcperrors.NewResourceError(
			fmt.Sprintf("invalid task ID format: %s, expected seriesID_taskID", taskID),
			nil, // No underlying Go error.
			map[string]interface{}{"taskID": taskID})
	}

	if listID == "" {
		return mcperrors.NewResourceError(
			"listID is required to complete a task",
			nil,
			map[string]interface{}{"taskID": taskID})
	}

	// RTM API requires a timeline for completing tasks.
	timeline, err := c.createTimeline(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create timeline for completing task")
	}

	params := map[string]string{
		"list_id":       listID,
		"taskseries_id": parts[0],
		"task_id":       parts[1],
		"timeline":      timeline,
	}

	// Make the API call.
	_, err = c.callMethod(ctx, methodCompleteTask, params)
	if err != nil {
		// Use errors.Wrap for context preservation.
		return errors.Wrap(err, "failed to complete task")
	}

	return nil
}

// GetTags retrieves all the tags for the user.
func (c *Client) GetTags(ctx context.Context) ([]Tag, error) {
	params := map[string]string{}
	resp, err := c.callMethod(ctx, methodGetTags, params)
	if err != nil {
		// Use errors.Wrap for context preservation.
		return nil, errors.Wrap(err, "failed to get tags")
	}

	// Parse the response.
	var tagsResp struct {
		Rsp struct {
			Tags struct {
				Tag []struct {
					Name string `json:"name"`
				} `json:"tag"`
			} `json:"tags"`
		} `json:"rsp"`
	}

	// Use errors.Wrap for context preservation.
	if err := json.Unmarshal(resp, &tagsResp); err != nil {
		return nil, errors.Wrap(err, "failed to parse tags response")
	}

	// Convert to our Tag type.
	var tags []Tag
	for _, t := range tagsResp.Rsp.Tags.Tag {
		tag := Tag{
			Name: t.Name,
		}
		tags = append(tags, tag)
	}

	return tags, nil
}

// createTimeline creates a new RTM timeline.
func (c *Client) createTimeline(ctx context.Context) (string, error) {
	params := map[string]string{}
	resp, err := c.callMethod(ctx, "rtm.timelines.create", params) // Using literal string as method wasn't in constants.
	if err != nil {
		return "", errors.Wrap(err, "failed to create timeline")
	}

	var timelineResp struct {
		Rsp struct {
			Timeline string `json:"timeline"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &timelineResp); err != nil {
		return "", errors.Wrap(err, "failed to parse timeline response")
	}

	if timelineResp.Rsp.Timeline == "" {
		return "", mcperrors.NewRTMError(mcperrors.ErrRTMInvalidResponse, "empty timeline received from API", nil, nil)
	}

	return timelineResp.Rsp.Timeline, nil
}
