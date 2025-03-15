// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/cowgnition/cowgnition/internal/auth"
)

// Service provides methods for interacting with RTM API.
// It handles authentication, request signing, and API operations.
type Service struct {
	client       *Client
	tokenPath    string
	authManager  *auth.AuthManager
	mu           sync.RWMutex
	lastSyncTime time.Time
}

// NewService creates a new RTM service with the specified credentials.
func NewService(apiKey, sharedSecret, tokenPath string) *Service {
	return &Service{
		client:    NewClient(apiKey, sharedSecret),
		tokenPath: tokenPath,
	}
}

// Initialize sets up the service and loads the auth token if available.
// It establishes the auth manager and verifies any existing tokens.
func (s *Service) Initialize() error {
	// Create auth manager with delete permission (full access)
	authManager, err := auth.NewAuthManager(s.tokenPath, auth.PermDelete)
	if err != nil {
		return fmt.Errorf("error creating auth manager: %w", err)
	}
	s.authManager = authManager

	// Check authentication status
	status, err := s.authManager.CheckAuthStatus(s.verifyToken)
	if err != nil {
		return fmt.Errorf("error checking authentication status: %w", err)
	}

	// If authenticated, set the token on the client
	if status == auth.StatusAuthenticated {
		token, err := s.authManager.GetToken()
		if err != nil {
			return fmt.Errorf("error loading token: %w", err)
		}
		s.client.SetAuthToken(token)
	}

	return nil
}

// verifyToken checks if a token is valid with the RTM API.
func (s *Service) verifyToken(token string) (bool, error) {
	// Set token temporarily for verification
	s.client.SetAuthToken(token)

	// Check token validity
	valid, err := s.client.CheckToken()

	// If error or invalid, clear the token
	if err != nil || !valid {
		s.client.SetAuthToken("")
		return false, err
	}

	return true, nil
}

// IsAuthenticated checks if the service has a valid auth token.
func (s *Service) IsAuthenticated() bool {
	status := s.authManager.GetStatus()
	return status == auth.StatusAuthenticated
}

// StartAuthFlow begins the authentication flow and returns a URL for the user to visit.
func (s *Service) StartAuthFlow() (string, string, error) {
	// Generate auth URL through the auth manager
	authURL, frob, err := s.authManager.StartAuthFlow(s.generateAuthURL)
	if err != nil {
		return "", "", fmt.Errorf("error starting auth flow: %w", err)
	}

	return authURL, frob, nil
}

// generateAuthURL creates an authentication URL for the given frob and permission level.
func (s *Service) generateAuthURL(frob string, perm string) (string, error) {
	// Use client to generate auth URL
	authURL := s.client.GetAuthURL(frob, perm)
	return authURL, nil
}

// CompleteAuthFlow completes the authentication flow with the provided frob.
func (s *Service) CompleteAuthFlow(frob string) error {
	// Exchange frob for token through auth manager
	err := s.authManager.CompleteAuthFlow(frob, s.getToken)
	if err != nil {
		return fmt.Errorf("error completing auth flow: %w", err)
	}

	// Get and set the token on successful completion
	token, err := s.authManager.GetToken()
	if err != nil {
		return fmt.Errorf("error loading token after auth completion: %w", err)
	}

	s.client.SetAuthToken(token)
	return nil
}

// getToken exchanges a frob for a token using the RTM API.
func (s *Service) getToken(frob string) (string, error) {
	// Use client to get token from frob
	token, err := s.client.GetToken(frob)
	if err != nil {
		return "", fmt.Errorf("error getting token from RTM API: %w", err)
	}

	return token, nil
}

// GetLists retrieves all lists from RTM.
func (s *Service) GetLists() ([]List, error) {
	// Ensure we're authenticated
	if !s.IsAuthenticated() {
		return nil, fmt.Errorf("not authenticated with RTM")
	}

	params := url.Values{}
	params.Set("method", "rtm.lists.getList")

	type listResponse struct {
		Response
		Lists struct {
			List []List `xml:"list"`
		} `xml:"lists"`
	}

	var resp listResponse
	if err := s.client.doRequest(params, &resp); err != nil {
		return nil, fmt.Errorf("error getting lists: %w", err)
	}

	return resp.Lists.List, nil
}

// GetTasks retrieves tasks based on the provided filter.
func (s *Service) GetTasks(filter string) (*TasksResponse, error) {
	// Ensure we're authenticated
	if !s.IsAuthenticated() {
		return nil, fmt.Errorf("not authenticated with RTM")
	}

	params := url.Values{}
	params.Set("method", "rtm.tasks.getList")

	// Add filter if provided
	if filter != "" {
		params.Set("filter", filter)
	}

	// Add last_sync if we have a previous sync time
	s.mu.RLock()
	if !s.lastSyncTime.IsZero() {
		params.Set("last_sync", s.lastSyncTime.Format(time.RFC3339))
	}
	s.mu.RUnlock()

	var resp TasksResponse
	if err := s.client.doRequest(params, &resp); err != nil {
		return nil, fmt.Errorf("error getting tasks: %w", err)
	}

	// Update last sync time
	s.mu.Lock()
	s.lastSyncTime = time.Now()
	s.mu.Unlock()

	return &resp, nil
}

// CreateTimeline creates a new timeline for operations that support undo.
func (s *Service) CreateTimeline() (string, error) {
	// Ensure we're authenticated
	if !s.IsAuthenticated() {
		return "", fmt.Errorf("not authenticated with RTM")
	}

	params := url.Values{}
	params.Set("method", "rtm.timelines.create")

	var resp TimelineResponse
	if err := s.client.doRequest(params, &resp); err != nil {
		return "", fmt.Errorf("error creating timeline: %w", err)
	}

	return resp.Timeline, nil
}

// AddTask adds a new task.
func (s *Service) AddTask(timeline, listID, name, dueDate string) error {
	// Ensure we're authenticated
	if !s.IsAuthenticated() {
		return fmt.Errorf("not authenticated with RTM")
	}

	params := url.Values{}
	params.Set("method", "rtm.tasks.add")
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("name", name)

	if dueDate != "" {
		params.Set("due", dueDate)
	}

	var resp Response
	if err := s.client.doRequest(params, &resp); err != nil {
		return fmt.Errorf("error adding task: %w", err)
	}

	return nil
}

// CompleteTask marks a task as completed.
func (s *Service) CompleteTask(timeline, listID, taskseriesID, taskID string) error {
	// Ensure we're authenticated
	if !s.IsAuthenticated() {
		return fmt.Errorf("not authenticated with RTM")
	}

	params := url.Values{}
	params.Set("method", "rtm.tasks.complete")
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)

	var resp Response
	if err := s.client.doRequest(params, &resp); err != nil {
		return fmt.Errorf("error completing task: %w", err)
	}

	return nil
}

// UncompleteTask marks a completed task as incomplete.
func (s *Service) UncompleteTask(timeline, listID, taskseriesID, taskID string) error {
	// Ensure we're authenticated
	if !s.IsAuthenticated() {
		return fmt.Errorf("not authenticated with RTM")
	}

	params := url.Values{}
	params.Set("method", "rtm.tasks.uncomplete")
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)

	var resp Response
	if err := s.client.doRequest(params, &resp); err != nil {
		return fmt.Errorf("error uncompleting task: %w", err)
	}

	return nil
}

// DeleteTask deletes a task.
func (s *Service) DeleteTask(timeline, listID, taskseriesID, taskID string) error {
	// Ensure we're authenticated
	if !s.IsAuthenticated() {
		return fmt.Errorf("not authenticated with RTM")
	}

	params := url.Values{}
	params.Set("method", "rtm.tasks.delete")
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)

	var resp Response
	if err := s.client.doRequest(params, &resp); err != nil {
		return fmt.Errorf("error deleting task: %w", err)
	}

	return nil
}

// AddTags adds tags to a task.
func (s *Service) AddTags(timeline, listID, taskseriesID, taskID string, tags []string) error {
	// Ensure we're authenticated
	if !s.IsAuthenticated() {
		return fmt.Errorf("not authenticated with RTM")
	}

	params := url.Values{}
	params.Set("method", "rtm.tasks.addTags")
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("tags", combineTagsForAPI(tags))

	var resp Response
	if err := s.client.doRequest(params, &resp); err != nil {
		return fmt.Errorf("error adding tags: %w", err)
	}

	return nil
}

// RemoveTags removes tags from a task.
func (s *Service) RemoveTags(timeline, listID, taskseriesID, taskID string, tags []string) error {
	// Ensure we're authenticated
	if !s.IsAuthenticated() {
		return fmt.Errorf("not authenticated with RTM")
	}

	params := url.Values{}
	params.Set("method", "rtm.tasks.removeTags")
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("tags", combineTagsForAPI(tags))

	var resp Response
	if err := s.client.doRequest(params, &resp); err != nil {
		return fmt.Errorf("error removing tags: %w", err)
	}

	return nil
}

// SetDueDate sets or updates a task's due date.
func (s *Service) SetDueDate(timeline, listID, taskseriesID, taskID, dueDate string, hasDueTime bool) error {
	// Ensure we're authenticated
	if !s.IsAuthenticated() {
		return fmt.Errorf("not authenticated with RTM")
	}

	params := url.Values{}
	params.Set("method", "rtm.tasks.setDueDate")
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)

	if dueDate == "" {
		// Clear due date
		params.Set("due", "")
	} else {
		params.Set("due", dueDate)
		if hasDueTime {
			params.Set("has_due_time", "1")
		} else {
			params.Set("has_due_time", "0")
		}
	}

	var resp Response
	if err := s.client.doRequest(params, &resp); err != nil {
		return fmt.Errorf("error setting due date: %w", err)
	}

	return nil
}

// SetPriority sets a task's priority.
func (s *Service) SetPriority(timeline, listID, taskseriesID, taskID, priority string) error {
	// Ensure we're authenticated
	if !s.IsAuthenticated() {
		return fmt.Errorf("not authenticated with RTM")
	}

	params := url.Values{}
	params.Set("method", "rtm.tasks.setPriority")
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("priority", priority)

	var resp Response
	if err := s.client.doRequest(params, &resp); err != nil {
		return fmt.Errorf("error setting priority: %w", err)
	}

	return nil
}

// SetName renames a task.
func (s *Service) SetName(timeline, listID, taskseriesID, taskID, name string) error {
	// Ensure we're authenticated
	if !s.IsAuthenticated() {
		return fmt.Errorf("not authenticated with RTM")
	}

	params := url.Values{}
	params.Set("method", "rtm.tasks.setName")
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("name", name)

	var resp Response
	if err := s.client.doRequest(params, &resp); err != nil {
		return fmt.Errorf("error setting name: %w", err)
	}

	return nil
}

// AddNote adds a note to a task.
func (s *Service) AddNote(timeline, listID, taskseriesID, taskID, title, text string) error {
	// Ensure we're authenticated
	if !s.IsAuthenticated() {
		return fmt.Errorf("not authenticated with RTM")
	}

	params := url.Values{}
	params.Set("method", "rtm.tasks.notes.add")
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("note_title", title)
	params.Set("note_text", text)

	var resp Response
	if err := s.client.doRequest(params, &resp); err != nil {
		return fmt.Errorf("error adding note: %w", err)
	}

	return nil
}

// combineTagsForAPI combines tags for the API.
func combineTagsForAPI(tags []string) string {
	// URL encode each tag and combine with commas
	for i, tag := range tags {
		tags[i] = url.QueryEscape(tag)
	}
	return fmt.Sprintf("\"%s\"", url.QueryEscape(fmt.Sprintf("%s", tags)))
}
