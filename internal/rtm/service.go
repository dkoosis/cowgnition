// Package rtm provides integration with the Remember The Milk API.
package rtm

import (
	"encoding/xml"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Status represents the authentication status of the RTM service.
type Status int

const (
	// StatusUnknown means the authentication status has not been determined.
	StatusUnknown Status = iota
	// StatusNotAuthenticated means the user is not authenticated.
	StatusNotAuthenticated
	// StatusAuthenticating means authentication is in progress.
	StatusAuthenticating
	// StatusAuthenticated means the user is authenticated.
	StatusAuthenticated
)

// List represents an RTM list.
type List struct {
	ID       string
	Name     string
	Deleted  bool
	Locked   bool
	Archived bool
	Position int
	Smart    bool
	Filter   string
}

// Task represents an RTM task.
type Task struct {
	ID         string
	SeriesID   string
	Name       string
	Due        time.Time
	HasDueTime bool
	Added      time.Time
	Completed  time.Time
	IsComplete bool
	Deleted    bool
	Priority   string
	Postponed  int
	Estimate   string
	ListID     string
	Tags       []string
	Notes      []Note
}

// Note represents an RTM task note.
type Note struct {
	ID       string
	Title    string
	Content  string
	Created  time.Time
	Modified time.Time
}

// TaskFilter defines a filter for tasks.
type TaskFilter struct {
	ListID    string
	DueFilter string
	TagFilter string
}

// AuthFlow represents an RTM authentication flow.
type AuthFlow struct {
	Frob      string
	AuthURL   string
	Timestamp time.Time
}

// Service provides a wrapper around the RTM client with additional functionality.
type Service struct {
	client       *Client
	authStatus   Status
	authFlows    map[string]AuthFlow
	lastRefresh  time.Time
	timeline     string
	mu           sync.Mutex
	permission   string
	tokenRefresh int
}

// NewService creates a new RTM service with the provided client.
func NewService(apiKey, sharedSecret, permission string, tokenRefresh int) *Service {
	return &Service{
		client:       NewClient(apiKey, sharedSecret),
		authStatus:   StatusUnknown,
		authFlows:    make(map[string]AuthFlow),
		permission:   permission,
		tokenRefresh: tokenRefresh,
	}
}

// GetAuthStatus returns the current authentication status.
func (s *Service) GetAuthStatus() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authStatus
}

// SetAuthStatus sets the authentication status.
func (s *Service) SetAuthStatus(status Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authStatus = status
}

// IsAuthenticated checks if the user is authenticated.
func (s *Service) IsAuthenticated() bool {
	return s.GetAuthStatus() == StatusAuthenticated
}

// StartAuthFlow starts the authentication flow and returns the auth URL and frob.
func (s *Service) StartAuthFlow() (authURL, frob string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get a frob from the RTM API
	frob, err = s.client.GetFrob()
	if err != nil {
		return "", "", fmt.Errorf("error getting frob: %w", err)
	}

	// Generate authentication URL
	authURL = s.client.GetAuthURL(frob, s.permission)

	// Store the authentication flow
	s.authFlows[frob] = AuthFlow{
		Frob:      frob,
		AuthURL:   authURL,
		Timestamp: time.Now(),
	}

	// Update status
	s.authStatus = StatusAuthenticating

	return authURL, frob, nil
}

// CompleteAuthFlow completes the authentication flow with the provided frob.
func (s *Service) CompleteAuthFlow(frob string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if frob exists
	flow, exists := s.authFlows[frob]
	if !exists {
		return fmt.Errorf("invalid frob or auth flow expired")
	}

	// Exchange frob for token
	token, err := s.client.GetToken(flow.Frob)
	if err != nil {
		return fmt.Errorf("error getting token: %w", err)
	}

	// Set token on client
	s.client.SetAuthToken(token)

	// Update status
	s.authStatus = StatusAuthenticated
	s.lastRefresh = time.Now()

	// Clean up auth flow
	delete(s.authFlows, frob)

	// Create timeline
	timeline, err := s.client.CreateTimeline()
	if err != nil {
		return fmt.Errorf("error creating timeline: %w", err)
	}
	s.timeline = timeline

	return nil
}

// CleanupExpiredFlows removes expired authentication flows.
func (s *Service) CleanupExpiredFlows() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Authentication flows expire after 1 hour
	expiry := time.Hour

	for frob, flow := range s.authFlows {
		if time.Since(flow.Timestamp) > expiry {
			delete(s.authFlows, frob)
		}
	}
}

// GetAllLists returns all RTM lists.
func (s *Service) GetAllLists() ([]List, error) {
	// Check authentication
	if s.GetAuthStatus() != StatusAuthenticated {
		return nil, fmt.Errorf("not authenticated")
	}

	// Call the RTM API
	resp, err := s.client.GetLists()
	if err != nil {
		return nil, fmt.Errorf("error getting lists: %w", err)
	}

	// Parse response
	var result struct {
		XMLName xml.Name `xml:"rsp"`
		Lists   struct {
			List []struct {
				ID       string `xml:"id,attr"`
				Name     string `xml:"name,attr"`
				Deleted  string `xml:"deleted,attr"`
				Locked   string `xml:"locked,attr"`
				Archived string `xml:"archived,attr"`
				Position string `xml:"position,attr"`
				Smart    string `xml:"smart,attr"`
				Filter   string `xml:",chardata"`
			} `xml:"list"`
		} `xml:"lists"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("error parsing lists response: %w", err)
	}

	// Convert to List objects
	lists := make([]List, 0, len(result.Lists.List))
	for _, list := range result.Lists.List {
		lists = append(lists, List{
			ID:       list.ID,
			Name:     list.Name,
			Deleted:  list.Deleted == "1",
			Locked:   list.Locked == "1",
			Archived: list.Archived == "1",
			Position: atoi(list.Position, 0),
			Smart:    list.Smart == "1",
			Filter:   list.Filter,
		})
	}

	return lists, nil
}

// GetList returns a specific RTM list by ID.
func (s *Service) GetList(listID string) (*List, error) {
	lists, err := s.GetAllLists()
	if err != nil {
		return nil, err
	}

	for _, list := range lists {
		if list.ID == listID {
			return &list, nil
		}
	}

	return nil, fmt.Errorf("list not found: %s", listID)
}

// GetTasks returns tasks based on the provided filter.
func (s *Service) GetTasks(filter TaskFilter) ([]Task, error) {
	// Check authentication
	if s.GetAuthStatus() != StatusAuthenticated {
		return nil, fmt.Errorf("not authenticated")
	}

	// Call the RTM API
	var rtmFilter string
	if filter.DueFilter != "" {
		rtmFilter = filter.DueFilter
	}
	if filter.TagFilter != "" {
		if rtmFilter != "" {
			rtmFilter += " AND "
		}
		rtmFilter += "tag:" + filter.TagFilter
	}

	resp, err := s.client.GetTasks(filter.ListID, rtmFilter)
	if err != nil {
		return nil, fmt.Errorf("error getting tasks: %w", err)
	}

	// Parse response
	tasks, err := s.parseTasks(resp)
	if err != nil {
		return nil, err
	}

	return tasks, nil
}

// parseTasks parses an RTM tasks API response into Task objects.
func (s *Service) parseTasks(resp []byte) ([]Task, error) {
	var result struct {
		XMLName xml.Name `xml:"rsp"`
		Tasks   struct {
			List []struct {
				ID         string `xml:"id,attr"`
				TaskSeries []struct {
					ID        string `xml:"id,attr"`
					Created   string `xml:"created,attr"`
					Modified  string `xml:"modified,attr"`
					Name      string `xml:"name,attr"`
					Source    string `xml:"source,attr"`
					Tags      struct {
						Tag []string `xml:"tag"`
					} `xml:"tags"`
					Notes struct {
						Note []struct {
							ID       string `xml:"id,attr"`
							Created  string `xml:"created,attr"`
							Modified string `xml:"modified,attr"`
							Title    string `xml:"title,attr"`
							Content  string `xml:",chardata"`
						} `xml:"note"`
					} `xml:"notes"`
					Task []struct {
						ID         string `xml:"id,attr"`
						Due        string `xml:"due,attr"`
						HasDueTime string `xml:"has_due_time,attr"`
						Added      string `xml:"added,attr"`
						Completed  string `xml:"completed,attr"`
						Deleted    string `xml:"deleted,attr"`
						Priority   string `xml:"priority,attr"`
						Postponed  string `xml:"postponed,attr"`
						Estimate   string `xml:"estimate,attr"`
					} `xml:"task"`
				} `xml:"taskseries"`
			} `xml:"list"`
		} `xml:"tasks"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("error parsing tasks response: %w", err)
	}

	// Convert to Task objects
	var tasks []Task

	for _, list := range result.Tasks.List {
		for _, series := range list.TaskSeries {
			for _, task := range series.Task {
				// Parse dates
				added, _ := parseRTMTime(task.Added)
				due, _ := parseRTMTime(task.Due)
				completed, _ := parseRTMTime(task.Completed)

				// Parse tags
				var tags []string
				if len(series.Tags.Tag) > 0 {
					tags = make([]string, len(series.Tags.Tag))
					copy(tags, series.Tags.Tag)
				}

				// Parse notes
				var notes []Note
				if len(series.Notes.Note) > 0 {
					notes = make([]Note, 0, len(series.Notes.Note))
					for _, noteData := range series.Notes.Note {
						created, _ := parseRTMTime(noteData.Created)
						modified, _ := parseRTMTime(noteData.Modified)
						
						notes = append(notes, Note{
							ID:       noteData.ID,
							Title:    noteData.Title,
							Content:  noteData.Content,
							Created:  created,
							Modified: modified,
						})
					}
				}

				tasks = append(tasks, Task{
					ID:         task.ID,
					SeriesID:   series.ID,
					Name:       series.Name,
					Due:        due,
					HasDueTime: task.HasDueTime == "1",
					Added:      added,
					Completed:  completed,
					IsComplete: task.Completed != "",
					Deleted:    task.Deleted == "1",
					Priority:   task.Priority,
					Postponed:  atoi(task.Postponed, 0),
					Estimate:   task.Estimate,
					ListID:     list.ID,
					Tags:       tags,
					Notes:      notes,
				})
			}
		}
	}

	return tasks, nil
}

// GetTasksForToday returns tasks due today.
func (s *Service) GetTasksForToday() ([]Task, error) {
	return s.GetTasks(TaskFilter{DueFilter: "due:today"})
}

// GetTasksForTomorrow returns tasks due tomorrow.
func (s *Service) GetTasksForTomorrow() ([]Task, error) {
	return s.GetTasks(TaskFilter{DueFilter: "due:tomorrow"})
}

// GetTasksForWeek returns tasks due this week.
func (s *Service) GetTasksForWeek() ([]Task, error) {
	return s.GetTasks(TaskFilter{DueFilter: "dueBefore:today+7days dueAfter:today-1day"})
}

// GetOverdueTasks returns overdue tasks.
func (s *Service) GetOverdueTasks() ([]Task, error) {
	return s.GetTasks(TaskFilter{DueFilter: "due:before today AND status:incomplete"})
}

// GetCompletedTasks returns completed tasks.
func (s *Service) GetCompletedTasks() ([]Task, error) {
	return s.GetTasks(TaskFilter{DueFilter: "status:completed"})
}

// AddTask adds a new task with the given name to the specified list.
func (s *Service) AddTask(name, listID string) (string, error) {
	// Check authentication
	if s.GetAuthStatus() != StatusAuthenticated {
		return "", fmt.Errorf("not authenticated")
	}

	// Ensure we have a valid timeline
	if s.timeline == "" {
		timeline, err := s.client.CreateTimeline()
		if err != nil {
			return "", fmt.Errorf("error creating timeline: %w", err)
		}
		s.timeline = timeline
	}

	// Call the RTM API
	resp, err := s.client.AddTask(s.timeline, name, listID)
	if err != nil {
		return "", fmt