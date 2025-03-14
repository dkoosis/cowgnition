package rtm

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// Service provides methods for interacting with RTM API
type Service struct {
	client    *Client
	tokenPath string
}

// TaskList represents a list of tasks
type TaskList struct {
	XMLName  xml.Name `xml:"lists"`
	Lists    []List   `xml:"list"`
}

// List represents an RTM list
type List struct {
	ID       string `xml:"id,attr"`
	Name     string `xml:"name,attr"`
	Deleted  string `xml:"deleted,attr"`
	Locked   string `xml:"locked,attr"`
	Archived string `xml:"archived,attr"`
	Position string `xml:"position,attr"`
	Smart    string `xml:"smart,attr"`
	Filter   string `xml:"filter,omitempty"`
}

// TasksResponse represents the response for getting tasks
type TasksResponse struct {
	Response
	Tasks struct {
		List []struct {
			ID         string       `xml:"id,attr"`
			TaskSeries []TaskSeries `xml:"taskseries"`
		} `xml:"list"`
	} `xml:"tasks"`
}

// TaskSeries represents a series of tasks
type TaskSeries struct {
	ID         string `xml:"id,attr"`
	Created    string `xml:"created,attr"`
	Modified   string `xml:"modified,attr"`
	Name       string `xml:"name,attr"`
	Source     string `xml:"source,attr"`
	Tags       []string `xml:"tags>tag,omitempty"`
	Notes      []Note `xml:"notes>note,omitempty"`
	Tasks      []Task `xml:"task"`
}

// Task represents an RTM task
type Task struct {
	ID          string `xml:"id,attr"`
	Due         string `xml:"due,attr"`
	HasDueTime  string `xml:"has_due_time,attr"`
	Added       string `xml:"added,attr"`
	Completed   string `xml:"completed,attr"`
	Deleted     string `xml:"deleted,attr"`
	Priority    string `xml:"priority,attr"`
	Postponed   string `xml:"postponed,attr"`
	Estimate    string `xml:"estimate,attr"`
}

// Note represents a note on a task
type Note struct {
	ID        string `xml:"id,attr"`
	Created   string `xml:"created,attr"`
	Modified  string `xml:"modified,attr"`
	Title     string `xml:"title"`
	Content   string `xml:"content"`
}

// TimelineResponse represents the response for creating a timeline
type TimelineResponse struct {
	Response
	Timeline string `xml:"timeline"`
}

// NewService creates a new RTM service
func NewService(apiKey, sharedSecret, tokenPath string) *Service {
	return &Service{
		client:    NewClient(apiKey, sharedSecret),
		tokenPath: tokenPath,
	}
}

// Initialize sets up the service and loads the auth token if available
func (s *Service) Initialize() error {
	// Create token directory if it doesn't exist
	tokenDir := filepath.Dir(s.tokenPath)
	if err := os.MkdirAll(tokenDir, 0700); err != nil {
		return fmt.Errorf("error creating token directory: %w", err)
	}

	// Attempt to load token
	token, err := s.loadToken()
	if err == nil {
		s.client.SetAuthToken(token)
		
		// Verify token
		valid, err := s.client.CheckToken()
		if err != nil {
			return fmt.Errorf("error checking token: %w", err)
		}
		
		if !valid {
			// Token is invalid, remove it
			if err := os.Remove(s.tokenPath); err != nil {
				return fmt.Errorf("error removing invalid token: %w", err)
			}
			
			// Clear the token
			s.client.SetAuthToken("")
		}
	}

	return nil
}

// IsAuthenticated checks if the service has a valid auth token
func (s *Service) IsAuthenticated() bool {
	return s.client.GetAuthToken() != ""
}

// StartAuthFlow begins the authentication flow and returns a URL for the user to visit
func (s *Service) StartAuthFlow() (string, string, error) {
	// Get a frob
	frob, err := s.client.GetFrob()
	if err != nil {
		return "", "", fmt.Errorf("error getting frob: %w", err)
	}
	
	// Generate auth URL
	authURL := s.client.GetAuthURL(frob, "delete") // Use delete permission for full access
	
	return authURL, frob, nil
}

// CompleteAuthFlow completes the authentication flow with the provided frob
func (s *Service) CompleteAuthFlow(frob string) error {
	// Exchange frob for token
	token, err := s.client.GetToken(frob)
	if err != nil {
		return fmt.Errorf("error getting token: %w", err)
	}
	
	// Save token
	if err := s.saveToken(token); err != nil {
		return fmt.Errorf("error saving token: %w", err)
	}
	
	return nil
}

// GetLists retrieves all lists
func (s *Service) GetLists() ([]List, error) {
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

// GetTasks retrieves tasks based on the provided filter
func (s *Service) GetTasks(filter string) (*TasksResponse, error) {
	params := url.Values{}
	params.Set("method", "rtm.tasks.getList")
	
	if filter != "" {
		params.Set("filter", filter)
	}
	
	var resp TasksResponse
	if err := s.client.doRequest(params, &resp); err != nil {
		return nil, fmt.Errorf("error getting tasks: %w", err)
	}
	
	return &resp, nil
}

// CreateTimeline creates a new timeline for operations that support undo
func (s *Service) CreateTimeline() (string, error) {
	params := url.Values{}
	params.Set("method", "rtm.timelines.create")
	
	var resp TimelineResponse
	if err := s.client.doRequest(params, &resp); err != nil {
		return "", fmt.Errorf("error creating timeline: %w", err)
	}
	
	return resp.Timeline, nil
}

// AddTask adds a new task
func (s *Service) AddTask(timeline, listID, name, dueDate string) error {
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

// CompleteTask marks a task as completed
func (s *Service) CompleteTask(timeline, listID, taskseriesID, taskID string) error {
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

// AddTags adds tags to a task
func (s *Service) AddTags(timeline, listID, taskseriesID, taskID string, tags []string) error {
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

// combineTagsForAPI combines tags for the API
func combineTagsForAPI(tags []string) string {
	for i, tag := range tags {
		// Escape commas in tag names
		tags[i] = url.QueryEscape(tag)
	}
	return fmt.Sprintf("\"%s\"", url.QueryEscape(fmt.Sprintf("%s", tags)))
}

// SaveToken saves the token to disk
func (s *Service) saveToken(token string) error {
	return os.WriteFile(s.tokenPath, []byte(token), 0600)
}

// LoadToken loads the token from disk
func (s *Service) loadToken() (string, error) {
	data, err := os.ReadFile(s.tokenPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
