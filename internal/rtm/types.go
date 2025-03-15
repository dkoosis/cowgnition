// Package rtm provides client functionality for the Remember The Milk API.
package rtm

// List represents an RTM list.
type List struct {
	ID        string `xml:"id,attr"`
	Name      string `xml:"name,attr"`
	Deleted   string `xml:"deleted,attr"`
	Locked    string `xml:"locked,attr"`
	Archived  string `xml:"archived,attr"`
	Position  string `xml:"position,attr"`
	Smart     string `xml:"smart,attr"`
	Filter    string `xml:"filter,omitempty"`
}

// Task represents a task in RTM.
type Task struct {
	ID           string `xml:"id,attr"`
	Due          string `xml:"due,attr"`
	HasDueTime   string `xml:"has_due_time,attr"`
	Added        string `xml:"added,attr"`
	Completed    string `xml:"completed,attr"`
	Deleted      string `xml:"deleted,attr"`
	Priority     string `xml:"priority,attr"`
	Postponed    string `xml:"postponed,attr"`
	Estimate     string `xml:"estimate,attr"`
}

// Taskseries represents a task series in RTM.
type Taskseries struct {
	ID           string  `xml:"id,attr"`
	Created      string  `xml:"created,attr"`
	Modified     string  `xml:"modified,attr"`
	Name         string  `xml:"name,attr"`
	Source       string  `xml:"source,attr"`
	URL          string  `xml:"url,attr,omitempty"`
	LocationID   string  `xml:"location_id,attr,omitempty"`
	Tags         Tags    `xml:"tags"`
	Participants string  `xml:"participants"`
	Notes        Notes   `xml:"notes"`
	Tasks        []Task  `xml:"task"`
}

// TaskList represents a list of tasks in RTM.
type TaskList struct {
	ID          string       `xml:"id,attr"`
	Taskseries  []Taskseries `xml:"taskseries"`
}

// Tags represents a collection of tags.
type Tags struct {
	Tag []string `xml:"tag"`
}

// Note represents a note attached to a task.
type Note struct {
	ID        string `xml:"id,attr"`
	Created   string `xml:"created,attr"`
	Modified  string `xml:"modified,attr"`
	Title     string `xml:"title"`
	Content   string `xml:"content"`
}

// Notes represents a collection of notes.
type Notes struct {
	Note []Note `xml:"note"`
}

// TasksResponse represents the response to a tasks.getList request.
type TasksResponse struct {
	Response
	Tasks struct {
		List []TaskList `xml:"list"`
	} `xml:"tasks"`
}

// TimelineResponse represents the response to a timelines.create request.
type TimelineResponse struct {
	Response
	Timeline string `xml:"timeline"`
}

// ListsResponse represents the response to a lists.getList request.
type ListsResponse struct {
	Response
	Lists struct {
		List []List `xml:"list"`
	} `xml:"lists"`
}
