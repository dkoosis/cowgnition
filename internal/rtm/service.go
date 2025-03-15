// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import (
	"fmt"
	"log"
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
	tokenManager *auth.TokenManager
	mu           sync.RWMutex
	lastSyncTime time.Time
	authStatus   Status
	authFlows    map[string]*Flow
	// Add permission level
	permission Permission
	// Add refresh interval
	tokenRefresh time.Duration
	// Add last refresh time
	lastRefresh time.Time
}

// NewService creates a new RTM service with the specified credentials.
func NewService(apiKey, sharedSecret, tokenPath string) *Service {
	return &Service{
		client:       NewClient(apiKey, sharedSecret),
		tokenPath:    tokenPath,
		authStatus:   StatusUnknown,
		authFlows:    make(map[string]*Flow),
		permission:   PermDelete,     // Default to full access
		tokenRefresh: 24 * time.Hour, // Check token daily
	}
}

// Initialize sets up the service and loads the auth token if available.
// It establishes the token manager and verifies any existing tokens.
func (s *Service) Initialize() error {
	// Create token manager for secure token storage
	tokenManager, err := auth.NewTokenManager(s.tokenPath)
	if err != nil {
		return fmt.Errorf("error creating token manager: %w", err)
	}
	s.tokenManager = tokenManager

	// Check if we have a stored token
	if s.tokenManager.HasToken() {
		token, err := s.tokenManager.LoadToken()
		if err != nil {
			log.Printf("Warning: Error loading token: %v", err)
		} else {
			// Set token on client
			s.client.SetAuthToken(token)

			// Check if token is valid
			valid, err := s.client.CheckToken()
			if err != nil || !valid {
				// Token is invalid, clear it
				log.Printf("Stored token is invalid, clearing it. Error: %v", err)
				s.client.SetAuthToken("")
				if err := s.tokenManager.DeleteToken(); err != nil {
					log.Printf("Warning: Failed to delete invalid token: %v", err)
				}
				s.authStatus = StatusNotAuthenticated
			} else {
				// Token is valid
				s.authStatus = StatusAuthenticated
				s.lastRefresh = time.Now()
				log.Println("Successfully authenticated with stored token")
			}
		}
	} else {
		// No token stored
		s.authStatus = StatusNotAuthenticated
	}

	// Start a background cleanup routine for expired auth flows and token refresh
	go s.startBackgroundTasks()

	return nil
}

// startBackgroundTasks starts background routines for token refresh and flow cleanup
func (s *Service) startBackgroundTasks() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.CleanupExpiredFlows()
		s.refreshTokenIfNeeded()
	}
}

// refreshTokenIfNeeded checks and refreshes the auth token if necessary
func (s *Service) refreshTokenIfNeeded() {
	s.mu.RLock()
	shouldRefresh := s.authStatus == StatusAuthenticated &&
		time.Since(s.lastRefresh) > s.tokenRefresh
	s.mu.RUnlock()

	if shouldRefresh {
		log.Println("Refreshing auth token")

		// Just validate the current token again
		valid, err := s.client.CheckToken()

		s.mu.Lock()
		defer s.mu.Unlock()

		if err != nil || !valid {
			log.Printf("Error refreshing token: %v", err)
			s.authStatus = StatusNotAuthenticated
			// Clear the invalid token
			s.client.SetAuthToken("")
			if err := s.tokenManager.DeleteToken(); err != nil {
				log.Printf("Warning: Failed to delete invalid token: %v", err)
			}
		} else {
			// Token is still valid, update refresh time
			s.lastRefresh = time.Now()
		}
	}
}

// SetPermission changes the permission level for new auth flows.
// This doesn't affect existing tokens, only new authentication flows.
func (s *Service) SetPermission(perm Permission) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.permission = perm
}

// GetPermission returns the current permission level.
func (s *Service) GetPermission() Permission {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.permission
}

// GetAuthStatus returns the current authentication status.
func (s *Service) GetAuthStatus() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authStatus
}

// GetActiveAuthFlows returns a count of active authentication flows.
func (s *Service) GetActiveAuthFlows() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.authFlows)
}

// HasAuthFlow checks if a specific frob has an active authentication flow.
func (s *Service) HasAuthFlow(frob string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.authFlows[frob]
	return exists
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

// IsAuthenticated checks if the service has a valid authentication token.
func (s *Service) IsAuthenticated() bool {
	// Check current auth status
	s.mu.RLock()
	status := s.authStatus
	timeSinceRefresh := time.Since(s.lastRefresh)
	s.mu.RUnlock()

	// If we know we're authenticated and token is still fresh, return true
	if status == StatusAuthenticated && timeSinceRefresh < s.tokenRefresh {
		return true
	}

	// If we don't know or think we're not authenticated, check if we have a token
	if !s.tokenManager.HasToken() {
		s.mu.Lock()
		s.authStatus = StatusNotAuthenticated
		s.mu.Unlock()
		return false
	}

	// We have a token, check if it's valid
	token, err := s.tokenManager.LoadToken()
	if err != nil {
		log.Printf("Error loading token: %v", err)
		s.mu.Lock()
		s.authStatus = StatusNotAuthenticated
		s.mu.Unlock()
		return false
	}

	// Set token on client
	s.client.SetAuthToken(token)

	// Verify token validity
	valid, err := s.client.CheckToken()
	if err != nil || !valid {
		// Token is invalid, clear it
		log.Printf("Stored token is invalid. Error: %v", err)
		s.client.SetAuthToken("")
		if err := s.tokenManager.DeleteToken(); err != nil {
			log.Printf("Warning: Failed to delete invalid token: %v", err)
		}
		s.mu.Lock()
		s.authStatus = StatusNotAuthenticated
		s.mu.Unlock()
		return false
	}

	// Token is valid, update refresh time
	s.mu.Lock()
	s.authStatus = StatusAuthenticated
	s.lastRefresh = time.Now()
	s.mu.Unlock()
	return true
}

// CleanupExpiredFlows removes any authentication flows that have expired.
func (s *Service) CleanupExpiredFlows() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for frob, flow := range s.authFlows {
		// Check if flow has expired
		if now.After(flow.ExpiresAt) {
			delete(s.authFlows, frob)
			log.Printf("Cleared expired auth flow for frob: %s", frob)
		}
	}
}

// StartAuthFlow initiates the RTM authentication flow.
// It returns the auth URL for the user to visit and the frob for future reference.
func (s *Service) StartAuthFlow() (string, string, error) {
	// Generate frob
	frob, err := s.client.GetFrob()
	if err != nil {
		return "", "", fmt.Errorf("error getting frob: %w", err)
	}

	// Get current permission level
	s.mu.RLock()
	perm := s.permission
	s.mu.RUnlock()

	// Generate auth URL
	authURL := s.client.GetAuthURL(frob, string(perm))

	// Create auth flow with expiration
	flow := &Flow{
		Frob:       frob,
		StartTime:  time.Now(),
		Permission: perm,
		AuthURL:    authURL,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}

	// Store auth flow
	s.mu.Lock()
	s.authFlows[frob] = flow
	s.authStatus = StatusPending
	s.mu.Unlock()

	return authURL, frob, nil
}

// CompleteAuthFlow completes the authentication flow with the provided frob.
// It exchanges the frob for a permanent auth token.
func (s *Service) CompleteAuthFlow(frob string) error {
	// Check if we have this frob
	s.mu.RLock()
	flow, exists := s.authFlows[frob]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("invalid frob, not found in active authentication flows")
	}

	// Check if flow has expired
	if time.Now().After(flow.ExpiresAt) {
		s.mu.Lock()
		delete(s.authFlows, frob)
		s.mu.Unlock()
		return fmt.Errorf("authentication flow expired, please start a new one")
	}

	// Exchange frob for token
	token, err := s.client.GetToken(frob)
	if err != nil {
		s.mu.Lock()
		s.authStatus = StatusNotAuthenticated
		s.mu.Unlock()
		return fmt.Errorf("error getting token: %w", err)
	}

	// Save token
	if err := s.tokenManager.SaveToken(token); err != nil {
		return fmt.Errorf("error saving token: %w", err)
	}

	// Clean up auth flow
	s.mu.Lock()
	delete(s.authFlows, frob)
	s.authStatus = StatusAuthenticated
	s.lastRefresh = time.Now()
	s.mu.Unlock()

	return nil
}

// ClearAuthentication removes all authentication data.
func (s *Service) ClearAuthentication() error {
	// Remove token
	if err := s.tokenManager.DeleteToken(); err != nil {
		return fmt.Errorf("error removing token: %w", err)
	}

	// Reset status and clear flows
	s.mu.Lock()
	s.authStatus = StatusNotAuthenticated
	s.authFlows = make(map[string]*Flow)
	s.client.SetAuthToken("")
	s.mu.Unlock()

	log.Println("Authentication cleared")
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
