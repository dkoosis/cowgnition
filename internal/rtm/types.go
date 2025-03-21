// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import "time"

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
	// StatusExpired means the authentication has expired.
	StatusExpired
	// StatusFailed means the authentication attempt failed.
	StatusFailed
)

// String returns a string representation of the auth status.
func (s Status) String() string {
	switch s {
	case StatusUnknown:
		return "Unknown"
	case StatusNotAuthenticated:
		return "Not Authenticated"
	case StatusAuthenticating:
		return "Authentication Pending"
	case StatusAuthenticated:
		return "Authenticated"
	case StatusExpired:
		return "Authentication Expired"
	case StatusFailed:
		return "Authentication Failed"
	default:
		return "Invalid Status"
	}
}

// Permission represents the RTM API permission level.
type Permission string

const (
	// PermRead is the read-only permission level.
	PermRead Permission = "read"
	// PermWrite allows reading and writing data.
	PermWrite Permission = "write"
	// PermDelete allows reading, writing, and deleting data.
	PermDelete Permission = "delete"
)

// String returns a string representation of the permission level.
func (p Permission) String() string {
	return string(p)
}

// AuthFlow represents an ongoing authentication flow with RTM.
type AuthFlow struct {
	Frob       string
	AuthURL    string
	StartTime  time.Time
	Permission Permission
	ExpiresAt  time.Time
}

// List represents a list in Remember The Milk.
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

// TasksResponse represents the response from the rtm.tasks.getList API method.
type TasksResponse struct {
	Tasks struct {
		List []TaskList `xml:"list"`
	} `xml:"tasks"`
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
