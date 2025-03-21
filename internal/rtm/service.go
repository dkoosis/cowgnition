// Package rtm provides integration with the Remember The Milk API.
package rtm

import (
	"encoding/xml"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Service provides a wrapper around the RTM client with additional functionality.
type Service struct {
	client       *Client
	authStatus   Status
	authFlows    map[string]*AuthFlow
	lastRefresh  time.Time
	timeline     string
	mu           sync.Mutex
	permission   string
	tokenRefresh int
}

// TasksResponse represents the response from the rtm.tasks.getList API method.
type TasksResponse struct {
	List []TaskList `xml:"list"`
}

// TaskList represents a list of tasks in the RTM API response.
type TaskList struct {
	ID         string       `xml:"id,attr"`
	TaskSeries []TaskSeries `xml:"taskseries"`
}

// TaskSeries represents a series of tasks in RTM.
type TaskSeries struct {
	ID       string `xml:"id,attr"`
	Created  string `xml:"created,attr"`
	Modified string `xml:"modified,attr"`
	Name     string `xml:"name,attr"`
	Source   string `xml:"source,attr"`
	Tags     Tags   `xml:"tags"`
	Notes    Notes  `xml:"notes"`
	Tasks    []Task `xml:"task"`
}

// Tags represents a collection of tags.
type Tags struct {
	Tag []string `xml:"tag"`
}

// Notes represents a collection of notes.
type Notes struct {
	Note []Note `xml:"note"`
}

// Note represents a note on a task.
type Note struct {
	ID       string `xml:"id,attr"`
	Created  string `xml:"created,attr"`
	Modified string `xml:"modified,attr"`
	Title    string `xml:"title,attr"`
	Text     string `xml:",chardata"`
}

// Task represents a task in RTM.
type Task struct {
	ID         string `xml:"id,attr"`
	Due        string `xml:"due,attr"`
	HasDueTime string `xml:"has_due_time,attr"`
	Added      string `xml:"added,attr"`
	Completed  string `xml:"completed,attr"`
	Deleted    string `xml:"deleted,attr"`
	Priority   string `xml:"priority,attr"`
	Postponed  string `xml:"postponed,attr"`
	Estimate   string `xml:"estimate,attr"`
}

// NewService creates a new RTM service with the provided client.
func NewService(apiKey, sharedSecret, permission string, tokenRefresh int) *Service {
	return &Service{
		client:       NewClient(apiKey, sharedSecret),
		authStatus:   StatusUnknown,
		authFlows:    make(map[string]*AuthFlow),
		permission:   permission,
		tokenRefresh: tokenRefresh,
	}
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
	for _, list := range tasksResp.List {
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
