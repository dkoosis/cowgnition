package rtm

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

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
	TaskCount int    `xml:"-"` // Not directly from API, calculated
}

// TasksResponse represents the response from rtm.tasks.getList.
type TasksResponse struct {
	Response
	Tasks struct {
		List []struct {
			ID         string       `xml:"id,attr"`
			TaskSeries []TaskSeries `xml:"taskseries"`
		} `xml:"list"`
	} `xml:"tasks"`
}

// TaskSeries represents a series of tasks in RTM.
type TaskSeries struct {
	ID           string    `xml:"id,attr"`
	Created      time.Time `xml:"created,attr"`
	Modified     time.Time `xml:"modified,attr"`
	Name         string    `xml:"name,attr"`
	Source       string    `xml:"source,attr"`
	LocationID   string    `xml:"location_id,attr,omitempty"`
	URL          string    `xml:"url,attr,omitempty"`
	ParentTaskID string    `xml:"parent_task_id,attr,omitempty"`
	
	// Child elements
	Tags struct {
		Tag []string `xml:"tag"`
	} `xml:"tags"`
	Participants struct {
		User []struct {
			ID       string `xml:"id,attr"`
			Username string `xml:"username,attr"`
			Fullname string `xml:"fullname,attr"`
		} `xml:"user"`
	} `xml:"participants"`
	Notes struct {
		Note []Note `xml:"note"`
	} `xml:"notes"`
	Tasks []Task `xml:"task"`
}

// Task represents an individual task in RTM.
type Task struct {
	ID           string    `xml:"id,attr"`
	Due          string    `xml:"due,attr"`
	HasDueTime   string    `xml:"has_due_time,attr"`
	Added        time.Time `xml:"added,attr"`
	Completed    string    `xml:"completed,attr"`
	Deleted      string    `xml:"deleted,attr"`
	Priority     string    `xml:"priority,attr"`
	Postponed    string    `xml:"postponed,attr"`
	Estimate     string    `xml:"estimate,attr"`
	Start        string    `xml:"start,attr,omitempty"`
	HasStartTime string    `xml:"has_start_time,attr,omitempty"`
}

// Note represents a note attached to a task.
type Note struct {
	ID        string    `xml:"id,attr"`
	Created   time.Time `xml:"created,attr"`
	Modified  time.Time `xml:"modified,attr"`
	Title     string    `xml:"title"`
	Content   string    `xml:"content"`
}

// TimelineResponse represents the response from rtm.timelines.create.
type TimelineResponse struct {
	Response
	Timeline string `xml:"timeline"`
}

// GetStatus returns the response status
func (r Response) GetStatus() string {
	return r.Status
}

// GetError returns the error code and message
func (r Response) GetError() (string, string) {
	if r.Error != nil {
		return r.Error.Code, r.Error.Message
	}
	return "", ""
}

// GetFormattedTasks formats TasksResponse data for human-readable output.
func (tr *TasksResponse) GetFormattedTasks() string {
	var result strings.Builder
	
	for _, list := range tr.Tasks.List {
		for _, ts := range list.TaskSeries {
			for _, task := range ts.Tasks {
				// Skip deleted or completed tasks
				if task.Deleted != "" {
					continue
				}
				
				// Format task information
				result.WriteString("- " + ts.Name)
				
				// Add priority if set
				if task.Priority != "N" && task.Priority != "" {
					priorityMap := map[string]string{
						"1": " (High)",
						"2": " (Medium)",
						"3": " (Low)",
					}
					result.WriteString(priorityMap[task.Priority])
				}
				
				// Add due date if set
				if task.Due != "" {
					result.WriteString(" [Due: " + formatDueDate(task.Due, task.HasDueTime) + "]")
				}
				
				// Add tags if any
				if len(ts.Tags.Tag) > 0 {
					result.WriteString(" Tags: " + formatTags(ts.Tags.Tag))
				}
				
				result.WriteString("\n")
				
				// Add notes if any
				for _, note := range ts.Notes.Note {
					if note.Title != "" {
						result.WriteString(fmt.Sprintf("  * %s: ", note.Title))
					} else {
						result.WriteString("  * Note: ")
					}
					result.WriteString(note.Content + "\n")
				}
			}
		}
	}
	
	if result.Len() == 0 {
		return "No tasks found."
	}
	
	return result.String()
}

// formatDueDate formats a due date from RTM API to a human-readable format.
func formatDueDate(due string, hasTime string) string {
	if due == "" {
		return ""
	}
	
	t, err := time.Parse(time.RFC3339, due)
	if err != nil {
		return due // Return original if parsing fails
	}
	
	if hasTime == "1" {
		return t.Format("Jan 2, 2006 3:04PM")
	}
	
	return t.Format("Jan 2, 2006")
}

// formatTags formats a slice of tags into a comma-separated string.
func formatTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	
	return strings.Join(tags, ", ")
}
