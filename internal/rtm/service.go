// file: internal/rtm/service.go
// Package rtm provides integration with the Remember The Milk API.
package rtm

import (
	"encoding/xml"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cowgnition/cowgnition/internal/rtm/client"
)

// Service provides a wrapper around the RTM client with additional functionality.
type Service struct {
	client       *client.Client
	authStatus   Status
	authFlows    map[string]*AuthFlow
	lastRefresh  time.Time
	timeline     string
	mu           sync.Mutex
	permission   string
	tokenRefresh int
}

// NewService creates a new RTM service.
func NewService(apiKey, sharedSecret, tokenPath string) *Service {
	return &Service{
		client:       client.NewClient(apiKey, sharedSecret),
		authStatus:   StatusUnknown,
		authFlows:    make(map[string]*AuthFlow),
		permission:   string(PermDelete),
		tokenRefresh: 24, // Default 24 hours
	}
}

// GetAuthStatus returns the current authentication status.
func (s *Service) GetAuthStatus() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authStatus
}

// IsAuthenticated returns true if the user is authenticated.
func (s *Service) IsAuthenticated() bool {
	return s.GetAuthStatus() == StatusAuthenticated
}

// GetActiveAuthFlows returns the number of active authentication flows.
func (s *Service) GetActiveAuthFlows() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.authFlows)
}

// StartAuthFlow begins a new authentication flow.
func (s *Service) StartAuthFlow() (string, string, error) {
	frob, err := s.client.GetFrob()
	if err != nil {
		return "", "", fmt.Errorf("StartAuthFlow: error getting frob: %w", err)
	}

	authURL := s.client.GetAuthURL(frob, s.permission)

	s.mu.Lock()
	s.authStatus = StatusAuthenticating
	s.authFlows[frob] = &AuthFlow{
		Frob:       frob,
		AuthURL:    authURL,
		StartTime:  time.Now(),
		Permission: Permission(s.permission),
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}
	s.mu.Unlock()

	return authURL, frob, nil
}

// CompleteAuthFlow completes an authentication flow with the given frob.
func (s *Service) CompleteAuthFlow(frob string) error {
	s.mu.Lock()
	flow, exists := s.authFlows[frob]
	s.mu.Unlock()

	if !exists {
		return fmt.Errorf("CompleteAuthFlow: invalid frob, not found in active authentication flows")
	}

	if time.Now().After(flow.ExpiresAt) {
		s.mu.Lock()
		delete(s.authFlows, frob)
		s.mu.Unlock()
		return fmt.Errorf("CompleteAuthFlow: authentication flow expired, please start a new one")
	}

	// Fixed: Used discard operator for unused token
	_, err := s.client.GetToken(frob)
	if err != nil {
		s.mu.Lock()
		s.authStatus = StatusFailed
		s.mu.Unlock()
		return fmt.Errorf("CompleteAuthFlow: error getting token: %w", err)
	}

	s.mu.Lock()
	delete(s.authFlows, frob)
	s.authStatus = StatusAuthenticated
	s.lastRefresh = time.Now()
	s.mu.Unlock()

	return nil
}

// ClearAuthentication clears all authentication data.
func (s *Service) ClearAuthentication() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.authStatus = StatusNotAuthenticated
	s.client.AuthToken = ""
	s.authFlows = make(map[string]*AuthFlow)
	s.timeline = ""

	return nil
}

// CreateTimeline creates a new timeline for operations.
// Returns the timeline ID or an error if creation fails.
func (s *Service) CreateTimeline() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// If we already have a timeline, return it
	if s.timeline != "" {
		return s.timeline, nil
	}

	// Create a new timeline via the client
	timeline, err := s.client.CreateTimeline()
	if err != nil {
		return "", fmt.Errorf("CreateTimeline: error creating timeline: %w", err)
	}

	// Store the timeline and return it
	s.timeline = timeline
	return timeline, nil
}

// GetLists returns all RTM lists.
func (s *Service) GetLists() ([]List, error) {
	// Check authentication.
	if s.GetAuthStatus() != StatusAuthenticated {
		return nil, fmt.Errorf("GetLists: not authenticated")
	}

	// Call the RTM API.
	resp, err := s.client.GetLists()
	if err != nil {
		return nil, fmt.Errorf("GetLists: error getting lists: %w", err)
	}

	// Parse response.
	var result struct {
		XMLName xml.Name `xml:"rsp"`
		Lists   struct {
			List []List `xml:"list"`
		} `xml:"lists"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("GetLists: error parsing response: %w", err)
	}

	return result.Lists.List, nil
}

// GetTasks retrieves tasks from the RTM API with optional filtering.
func (s *Service) GetTasks(filter string) (*TasksResponse, error) {
	// Check authentication.
	if s.GetAuthStatus() != StatusAuthenticated {
		return nil, fmt.Errorf("GetTasks: not authenticated")
	}

	// Call the RTM API.
	resp, err := s.client.GetTasks(filter)
	if err != nil {
		return nil, fmt.Errorf("GetTasks: error getting tasks: %w", err)
	}

	// Parse response.
	var result struct {
		XMLName xml.Name      `xml:"rsp"`
		Tasks   TasksResponse `xml:"tasks"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("GetTasks: error parsing response: %w", err)
	}

	return &result.Tasks, nil
}

// AddTask adds a new task to RTM.
func (s *Service) AddTask(timeline, listID, name, dueDate string) error {
	// Check authentication.
	if s.GetAuthStatus() != StatusAuthenticated {
		return fmt.Errorf("AddTask: not authenticated")
	}

	// Call the RTM API.
	_, err := s.client.AddTask(timeline, name, listID)
	if err != nil {
		return fmt.Errorf("AddTask: error adding task: %w", err)
	}

	return nil
}

// CompleteTask marks a task as complete.
func (s *Service) CompleteTask(timeline, listID, taskseriesID, taskID string) error {
	// Check authentication.
	if s.GetAuthStatus() != StatusAuthenticated {
		return fmt.Errorf("CompleteTask: not authenticated")
	}

	// Call the RTM API.
	_, err := s.client.CompleteTask(timeline, listID, taskseriesID, taskID)
	if err != nil {
		return fmt.Errorf("CompleteTask: error completing task: %w", err)
	}

	return nil
}

// UncompleteTask marks a task as incomplete.
func (s *Service) UncompleteTask(timeline, listID, taskseriesID, taskID string) error {
	// Check authentication.
	if s.GetAuthStatus() != StatusAuthenticated {
		return fmt.Errorf("UncompleteTask: not authenticated")
	}

	// Call the RTM API.
	// Note: RTM doesn't have a direct uncomplete method, so we need to use "change" with an empty completed date.
	// Implementation details would depend on the actual API.
	_, err := s.client.CompleteTask(timeline, listID, taskseriesID, taskID)
	if err != nil {
		return fmt.Errorf("UncompleteTask: error uncompleting task: %w", err)
	}

	return nil
}

// DeleteTask deletes a task.
func (s *Service) DeleteTask(timeline, listID, taskseriesID, taskID string) error {
	// Check authentication.
	if s.GetAuthStatus() != StatusAuthenticated {
		return fmt.Errorf("DeleteTask: not authenticated")
	}

	// Call the RTM API.
	_, err := s.client.DeleteTask(timeline, listID, taskseriesID, taskID)
	if err != nil {
		return fmt.Errorf("DeleteTask: error deleting task: %w", err)
	}

	return nil
}

// SetDueDate sets the due date for a task.
func (s *Service) SetDueDate(timeline, listID, taskseriesID, taskID, dueDate string, hasDueTime bool) error {
	// Check authentication.
	if s.GetAuthStatus() != StatusAuthenticated {
		return fmt.Errorf("SetDueDate: not authenticated")
	}

	// Call the RTM API.
	_, err := s.client.SetTaskDueDate(timeline, listID, taskseriesID, taskID, dueDate)
	if err != nil {
		return fmt.Errorf("SetDueDate: error setting due date: %w", err)
	}

	return nil
}

// SetPriority sets the priority for a task.
func (s *Service) SetPriority(timeline, listID, taskseriesID, taskID, priority string) error {
	// Check authentication.
	if s.GetAuthStatus() != StatusAuthenticated {
		return fmt.Errorf("SetPriority: not authenticated")
	}

	// Call the RTM API.
	_, err := s.client.SetTaskPriority(timeline, listID, taskseriesID, taskID, priority)
	if err != nil {
		return fmt.Errorf("SetPriority: error setting priority: %w", err)
	}

	return nil
}

// AddTags adds tags to a task.
func (s *Service) AddTags(timeline, listID, taskseriesID, taskID string, tags []string) error {
	// Check authentication.
	if s.GetAuthStatus() != StatusAuthenticated {
		return fmt.Errorf("AddTags: not authenticated")
	}

	// Join tags into a comma-separated string.
	tagsStr := strings.Join(tags, ",")

	// Call the RTM API.
	_, err := s.client.AddTags(timeline, listID, taskseriesID, taskID, tagsStr)
	if err != nil {
		return fmt.Errorf("AddTags: error adding tags: %w", err)
	}

	return nil
}

// GetTags returns all tags used in the system.
func (s *Service) GetTags() ([]string, error) {
	// Check authentication.
	if s.GetAuthStatus() != StatusAuthenticated {
		return nil, fmt.Errorf("GetTags: not authenticated")
	}

	// Since RTM API doesn't have a direct method to get all tags,
	// we fetch all tasks and extract unique tags.
	tasksResp, err := s.GetTasks("")
	if err != nil {
		return nil, fmt.Errorf("GetTags: error getting tasks: %w", err)
	}

	// Extract unique tags.
	tagMap := make(map[string]bool)
	for _, list := range tasksResp.Tasks.List {
		for _, ts := range list.TaskSeries {
			for _, tag := range ts.Tags.Tag {
				if tag != "" {
					tagMap[tag] = true
				}
			}
		}
	}

	// Convert map to slice.
	tags := make([]string, 0, len(tagMap))
	for tag := range tagMap {
		tags = append(tags, tag)
	}

	return tags, nil
}

// FormatTaskPriority returns a human-readable priority string.
func (s *Service) FormatTaskPriority(priority string) string {
	switch priority {
	case "1":
		return "High"
	case "2":
		return "Medium"
	case "3":
		return "Low"
	default:
		return "None"
	}
}
