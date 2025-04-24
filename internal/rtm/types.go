// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

import (
	"encoding/json"
	"errors" // Keep 'errors' package import.
	"fmt"
	"net/http" // Needed for Config.HTTPClient.
	"time"
)

// --- Custom Types for Unmarshalling ---

// rtmTags is a custom type to handle inconsistent JSON for task tags (either object or array).
type rtmTags []string

// UnmarshalJSON implements the json.Unmarshaler interface for rtmTags.
// It handles the cases where RTM API returns tags as either:
// 1. An object: {"tag": ["tag1", "tag2"]}.
// 2. An empty array: [].
// 3. Null or omitted (which omitempty handles, but we double-check).
func (rt *rtmTags) UnmarshalJSON(data []byte) error {
	// Case 1: Try parsing as the object structure {"tag": [...]}.
	var tagObj struct {
		Tag []string `json:"tag"` // Note: omitempty not needed here.
	}
	errObj := json.Unmarshal(data, &tagObj)
	if errObj == nil {
		// Successfully parsed as object, assign the inner slice.
		*rt = tagObj.Tag
		return nil
	}

	var unmarshalTypeError *json.UnmarshalTypeError
	// Check if the error was specifically a type error (meaning input was not an object).
	if !errors.As(errObj, &unmarshalTypeError) && errObj != nil {
		// If it was a different kind of error (e.g., invalid JSON), return it.
		// It's okay to wrap errObj here as it's the primary error in this branch.
		return fmt.Errorf("error unmarshalling tags as object: %w", errObj)
	}
	// If it *was* an UnmarshalTypeError, we proceed to try parsing as an array.

	// Case 2: Try parsing as a direct string array []string.
	var tagArr []string
	errArr := json.Unmarshal(data, &tagArr)
	if errArr == nil {
		// Successfully parsed as array (could be empty [] or ["tag1"]).
		*rt = tagArr
		// Handle the case where RTM might send null explicitly for no tags.
		if data == nil || string(data) == "null" {
			*rt = []string{} // Ensure it's an empty slice, not nil.
		}
		return nil
	}

	// <<< FIX: Simplify final error message when wrapping errArr >>>.
	// If both attempts failed, return an error wrapping the array parsing error.
	// We already know object parsing failed (likely due to type mismatch).
	return fmt.Errorf("failed to unmarshal rtmTags as object or array: %w", errArr)
	// <<< END FIX >>>.
}

// --- RTM API Response Structures ---

// rtmRsp is the base structure for all RTM API responses.
type rtmRsp struct {
	Stat string    `json:"stat"` // ok or fail.
	Err  *rtmError `json:"err,omitempty"`
}

// tasksRsp holds the response structure for rtm.tasks.getList.
type tasksRsp struct {
	Rsp struct { // Embedding rtmRsp to handle base fields.
		Stat  string    `json:"stat"` // ok or fail.
		Err   *rtmError `json:"err,omitempty"`
		Tasks struct {
			Rev  string    `json:"rev"`
			List []rtmList `json:"list"`
		} `json:"tasks"`
	} `json:"rsp"`
}

// rtmList represents a single list containing task series within the RTM API response.
type rtmList struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"` // Added Name field.
	TaskSeries []rtmTaskSeries `json:"taskseries"`
}

// rtmTaskSeries represents a task series from the RTM API response.
// A task series groups recurring instances of a task.
type rtmTaskSeries struct {
	ID           string           `json:"id"`
	Created      string           `json:"created"` // Keep as string for safer parsing.
	Modified     string           `json:"modified"`
	Name         string           `json:"name"`
	Source       string           `json:"source"`
	URL          string           `json:"url,omitempty"`
	LocationID   string           `json:"location_id,omitempty"`
	LocationName string           `json:"location,omitempty"` // RTM often uses 'location' instead of 'locationName'.
	Tags         rtmTags          `json:"tags,omitempty"`     // Use the custom rtmTags type.
	Participants []rtmParticipant `json:"participants,omitempty"`
	Notes        json.RawMessage  `json:"notes,omitempty"` // Use RawMessage for flexible parsing.
	Task         []rtmTask        `json:"task"`            // Individual task instances within the series.
	RRule        string           `json:"rrule,omitempty"` // Recurrence rule string.
}

// rtmTask represents an individual task instance within a task series.
type rtmTask struct {
	ID         string `json:"id"`
	Due        string `json:"due"` // Keeping as string due to potential "" value.
	HasDueTime string `json:"has_due_time"`
	Added      string `json:"added"`     // Keep as string for safer parsing.
	Completed  string `json:"completed"` // Keeping as string due to potential "" value.
	Deleted    string `json:"deleted"`   // Keeping as string due to potential "" value.
	Priority   string `json:"priority"`  // Can be "N".
	Postponed  string `json:"postponed"`
	Estimate   string `json:"estimate"`
}

// rtmNote represents a note associated with a task series during unmarshalling.
type rtmNote struct {
	ID       string `json:"id"`
	Created  string `json:"created"` // Keep as string for safer parsing.
	Modified string `json:"modified"`
	Title    string `json:"title"`
	Body     string `json:"$t"` // RTM uses '$t' for the note body.
}

// Note represents a note associated with a task (publicly exposed type).
type Note struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

// rtmParticipant represents a participant associated with a task series.
type rtmParticipant struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Fullname string `json:"fullname"`
}

// rtmError represents the error structure returned by the RTM API.
type rtmError struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
}

// --- Authentication Related Structures ---

// rtmAuthCheck represents the response structure for rtm.auth.checkToken.
type rtmAuthCheck struct {
	Rsp struct {
		Stat string    `json:"stat"`
		Err  *rtmError `json:"err,omitempty"`
		Auth struct {
			Token string `json:"token"`
			Perms string `json:"perms"`
			User  struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Fullname string `json:"fullname"`
			} `json:"user"`
		} `json:"auth"`
	} `json:"rsp"`
}

// AuthState holds the current authentication status details.
type AuthState struct {
	IsAuthenticated bool      `json:"isAuthenticated"`
	AuthToken       string    `json:"authToken,omitempty"`
	Permissions     string    `json:"permissions,omitempty"`
	UserID          string    `json:"userId,omitempty"`
	Username        string    `json:"username,omitempty"`
	Fullname        string    `json:"fullname,omitempty"`    // Correct Go field name (upper 'n').
	LastChecked     time.Time `json:"lastChecked,omitempty"` // When the state was last verified.
	CheckError      string    `json:"checkError,omitempty"`  // Error message if verification failed.
}

// AuthResult holds the response from rtm.auth.getToken.
// This is the canonical definition, replacing the one previously in auth_manager.go.
type AuthResult struct {
	Rsp struct {
		Stat string    `json:"stat"`
		Err  *rtmError `json:"err,omitempty"`
		Auth struct {
			Token string `json:"token"`
			Perms string `json:"perms"`
			User  struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Fullname string `json:"fullname"`
			} `json:"user"`
		} `json:"auth"`
	} `json:"rsp"`
}

// FrobResult holds the response from rtm.auth.getFrob.
type FrobResult struct {
	Rsp struct {
		Stat string    `json:"stat"`
		Err  *rtmError `json:"err,omitempty"`
		Frob string    `json:"frob"`
	} `json:"rsp"`
}

// TimelineResult holds the response from rtm.timelines.create.
type TimelineResult struct {
	Rsp struct {
		Stat     string    `json:"stat"`
		Err      *rtmError `json:"err,omitempty"`
		Timeline string    `json:"timeline"`
	} `json:"rsp"`
}

// --- Other API Response Structures ---

// listsRsp holds the response for rtm.lists.getList.
type listsRsp struct {
	Rsp struct {
		Stat  string    `json:"stat"`
		Err   *rtmError `json:"err,omitempty"`
		Lists struct {
			List []rtmListMeta `json:"list"`
		} `json:"lists"`
	} `json:"rsp"`
}

// rtmListMeta contains metadata about a single list.
type rtmListMeta struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Deleted    string `json:"deleted"` // "0" or "1".
	Locked     string `json:"locked"`
	Archived   string `json:"archived"`
	Position   string `json:"position"`
	Smart      string `json:"smart"`
	SortOrder  string `json:"sort_order"`
	Filter     string `json:"filter,omitempty"` // Only present for smart lists.
	TimelineID string `json:"timeline,omitempty"`
}

// TaskList represents metadata about an RTM task list (publicly exposed type).
type TaskList struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Deleted   bool   `json:"deleted"`
	Locked    bool   `json:"locked"`
	Archived  bool   `json:"archived"`
	Position  int    `json:"position"`
	SmartList bool   `json:"smartList"`
}

// tagsRsp holds the response for rtm.tags.getList.
type tagsRsp struct {
	Rsp struct {
		Stat string    `json:"stat"`
		Err  *rtmError `json:"err,omitempty"`
		Tags struct {
			Tag []string `json:"tag"`
		} `json:"tags"`
	} `json:"rsp"`
}

// Tag represents an RTM tag (publicly exposed type).
type Tag struct {
	Name string `json:"name"`
}

// LocationsResult holds the response for rtm.locations.getList.
type LocationsResult struct {
	Rsp struct {
		Stat      string    `json:"stat"`
		Err       *rtmError `json:"err,omitempty"`
		Locations struct {
			Location []rtmLocation `json:"location"`
		} `json:"locations"`
	} `json:"rsp"`
}

// rtmLocation contains details about a single location.
type rtmLocation struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Longitude string `json:"longitude"`
	Latitude  string `json:"latitude"`
	Zoom      string `json:"zoom"`
	Address   string `json:"address"`
	Viewable  string `json:"viewable"` // "0" or "1".
}

// GenericTxnResult holds the common response structure for transactional RTM calls
// like completing or adding a task.
type GenericTxnResult struct {
	Rsp struct {
		Stat        string    `json:"stat"`
		Err         *rtmError `json:"err,omitempty"`
		Timeline    string    `json:"timeline"`
		Transaction struct {
			ID       string `json:"id"`
			Undoable string `json:"undoable"` // "0" or "1".
		} `json:"transaction"`
		List struct { // Sometimes RTM includes the list/taskseries/task info.
			ID         string          `json:"id"`
			Taskseries []rtmTaskSeries `json:"taskseries,omitempty"`
		} `json:"list,omitempty"`
	} `json:"rsp"`
}

// createTaskRsp holds the specific response structure for rtm.tasks.add.
// RTM includes the task series and task info directly under list for add.
type createTaskRsp struct {
	Rsp struct {
		Stat        string    `json:"stat"`
		Err         *rtmError `json:"err,omitempty"`
		Timeline    string    `json:"timeline"`
		Transaction struct {
			ID       string `json:"id"`
			Undoable string `json:"undoable"` // "0" or "1".
		} `json:"transaction"`
		List struct {
			ID         string   `json:"id"`
			Taskseries struct { // RTM uses singular 'taskseries' here.
				ID   string  `json:"id"`
				Name string  `json:"name"` // And returns the name here.
				Task rtmTask `json:"task"` // And singular 'task'.
			} `json:"taskseries"`
		} `json:"list"`
	} `json:"rsp"`
}

// Task represents the simplified structure used for returning data via MCP tools/resources.
type Task struct {
	ID            string    `json:"id"` // Combined: seriesID_taskID.
	Name          string    `json:"name"`
	URL           string    `json:"url"`
	ListID        string    `json:"listId"`
	ListName      string    `json:"listName"` // Added ListName.
	LocationID    string    `json:"locationId,omitempty"`
	LocationName  string    `json:"locationName,omitempty"`
	Completed     bool      `json:"completed"` // True if the specific instance is completed.
	Notes         []Note    `json:"notes"`     // Use exported Note type.
	Tags          []string  `json:"tags"`
	DueDate       time.Time `json:"dueDate,omitempty"` // Zero time if no due date.
	HasDueTime    bool      `json:"hasDueTime"`
	Priority      int       `json:"priority"`                // 0 = N, 1, 2, 3.
	Postponed     int       `json:"postponed"`               // Number of times postponed.
	Estimate      string    `json:"estimate"`                // Estimate string.
	StartDate     time.Time `json:"startDate"`               // Added date.
	CompletedDate time.Time `json:"completedDate,omitempty"` // Completion date.
	Created       time.Time `json:"created"`                 // Task Series creation time.
	Modified      time.Time `json:"modified"`                // Task Series modification time.
}

// --- Config Definition ---

// Config holds configuration settings specific to the RTM client.
// This is now the single definition used by client.go.
type Config struct {
	APIKey       string
	SharedSecret string
	AuthToken    string       // Optional: Can be set after authentication.
	APIEndpoint  string       // Optional: Defaults if empty.
	HTTPClient   *http.Client // Optional: Defaults if nil.
}

// --- Helper Types ---

// timelineRsp holds the response from rtm.timelines.create (needed by methods.go).
type timelineRsp struct {
	Rsp struct {
		Stat     string    `json:"stat"`
		Err      *rtmError `json:"err,omitempty"`
		Timeline string    `json:"timeline"`
	} `json:"rsp"`
}

// EnsureAuthResult defines a simpler result structure for the AuthManager.EnsureAuthenticated method.
// This avoids confusion with the RTM API's AuthResult struct.
type EnsureAuthResult struct {
	Success     bool
	Username    string
	Error       error
	AuthURL     string // Only populated if interactive flow starts.
	Frob        string // Only populated if interactive flow starts.
	NeedsManual bool   // True if interactive flow requires manual completion.
}
