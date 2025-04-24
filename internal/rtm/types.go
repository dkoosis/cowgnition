// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/types.go

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// --- Custom Types for Unmarshalling ---

// rtmTags is a custom type to handle inconsistent JSON for task tags (either object or array).
type rtmTags []string

// UnmarshalJSON implements the json.Unmarshaler interface for rtmTags.
func (rt *rtmTags) UnmarshalJSON(data []byte) error {
	// First try: Handles {"tag": ["tag1", "tag2"]}
	var tagObj struct {
		Tag []string `json:"tag"`
	}
	if err := json.Unmarshal(data, &tagObj); err == nil && len(tagObj.Tag) > 0 {
		*rt = tagObj.Tag
		return nil
	}

	// Second try: Handles ["tag1", "tag2"] or [] or null
	var tagArr []string
	if err := json.Unmarshal(data, &tagArr); err == nil {
		*rt = tagArr
		if data == nil || string(data) == "null" {
			*rt = []string{} // Ensure empty slice for null or empty array
		}
		return nil
	}

	// Third try: Handles array of objects with name property: [{"name":"tag1"},{"name":"tag2"}]
	var tagObjArr []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &tagObjArr); err == nil {
		// Extract just the names into our string slice
		names := make([]string, len(tagObjArr))
		for i, obj := range tagObjArr {
			names[i] = obj.Name
		}
		*rt = names
		return nil
	}

	// Return a generic error if none of the formats match
	return fmt.Errorf("failed to unmarshal rtmTags as object or array: %s", string(data))
}

// --- RTM API Response Structures ---

// rtmRsp is the base structure for all RTM API responses.
type rtmRsp struct {
	Stat string    `json:"stat"` // ok or fail.
	Err  *rtmError `json:"err,omitempty"`
}

// rtmError represents the error structure returned by the RTM API.
type rtmError struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
}

// tasksRsp holds the response structure for rtm.tasks.getList.
type tasksRsp struct {
	Rsp struct {
		Stat  string    `json:"stat"`
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
	Name       string          `json:"name"`
	TaskSeries []rtmTaskSeries `json:"taskseries"`
}

// rtmTaskSeries represents a task series from the RTM API response.
type rtmTaskSeries struct {
	ID           string           `json:"id"`
	Created      string           `json:"created"`
	Modified     string           `json:"modified"`
	Name         string           `json:"name"`
	Source       string           `json:"source"`
	URL          string           `json:"url,omitempty"`
	LocationID   string           `json:"location_id,omitempty"`
	LocationName string           `json:"location,omitempty"`
	Tags         rtmTags          `json:"tags,omitempty"` // Custom type
	Participants []rtmParticipant `json:"participants,omitempty"`
	Notes        json.RawMessage  `json:"notes,omitempty"` // Flexible note parsing
	Task         []rtmTask        `json:"task"`
	RRule        json.RawMessage  `json:"rrule,omitempty"` // MODIFIED: Handles string or object
}

// rtmTask represents an individual task instance within a task series.
type rtmTask struct {
	ID         string `json:"id"`
	Due        string `json:"due"`
	HasDueTime string `json:"has_due_time"`
	Added      string `json:"added"`
	Completed  string `json:"completed"`
	Deleted    string `json:"deleted"`
	Priority   string `json:"priority"`
	Postponed  string `json:"postponed"`
	Estimate   string `json:"estimate"`
}

// rtmNote represents a note associated with a task series during unmarshalling.
type rtmNote struct {
	ID       string `json:"id"`
	Created  string `json:"created"`
	Modified string `json:"modified"`
	Title    string `json:"title"`
	Body     string `json:"$t"`
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

// --- Settings Structures --- START

// settingsRsp holds the raw response structure for rtm.settings.getList.
type settingsRsp struct {
	Rsp struct {
		Stat     string      `json:"stat"`
		Err      *rtmError   `json:"err,omitempty"`
		Settings rtmSettings `json:"settings"` // Assuming settings are nested under "settings".
	} `json:"rsp"`
}

// rtmSettings contains the raw settings data from the API.
type rtmSettings struct {
	Timezone       string `json:"timezone"`
	DateFormat     string `json:"dateformat"` // "0" or "1"
	TimeFormat     string `json:"timeformat"` // "0" or "1"
	DefaultList    string `json:"defaultlist"`
	Language       string `json:"language"`
	DefaultDueDate string `json:"defaultduedate"`
	Pro            string `json:"pro"` // "0" or "1"
}

// Settings represents the processed user settings (publicly exposed type).
type Settings struct {
	Timezone       string `json:"timezone"`
	IsAmericanDate bool   `json:"isAmericanDate"` // 0 = European (false), 1 = American (true).
	Is24HourTime   bool   `json:"is24HourTime"`   // 0 = 12h (false), 1 = 24h (true).
	DefaultListID  string `json:"defaultListId"`
	Language       string `json:"language"`
	DefaultDueDate string `json:"defaultDueDate"` // Keep as string for now.
	IsProAccount   bool   `json:"isProAccount"`   // 0 = Basic (false), 1 = Pro (true).
}

// --- Settings Structures --- END

// --- Authentication Related Structures --- START
// Restored missing structs and fields for auth flow

// rtmAuthUser holds user details within auth responses.
type rtmAuthUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Fullname string `json:"fullname"`
}

// rtmAuthDetails holds token, permissions, and user info in auth responses.
type rtmAuthDetails struct {
	Token string      `json:"token"`
	Perms string      `json:"perms"`
	User  rtmAuthUser `json:"user"`
}

// rtmAuthCheck represents the response structure for rtm.auth.checkToken.
type rtmAuthCheck struct {
	Rsp struct {
		Stat string         `json:"stat"`
		Err  *rtmError      `json:"err,omitempty"`
		Auth rtmAuthDetails `json:"auth"`
	} `json:"rsp"`
}

// AuthState holds the current authentication status details.
type AuthState struct {
	IsAuthenticated bool      `json:"isAuthenticated"`
	AuthToken       string    `json:"authToken,omitempty"`
	Permissions     string    `json:"permissions,omitempty"`
	UserID          string    `json:"userId,omitempty"`
	Username        string    `json:"username,omitempty"`
	Fullname        string    `json:"fullname,omitempty"`
	LastChecked     time.Time `json:"lastChecked,omitempty"`
	CheckError      string    `json:"checkError,omitempty"`
}

// AuthResult holds the response from rtm.auth.getToken.
type AuthResult struct {
	Rsp struct {
		Stat string         `json:"stat"`
		Err  *rtmError      `json:"err,omitempty"`
		Auth rtmAuthDetails `json:"auth"`
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

// --- Authentication Related Structures --- END

// --- Other API Response Structures --- START

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
	Deleted    string `json:"deleted"`
	Locked     string `json:"locked"`
	Archived   string `json:"archived"`
	Position   string `json:"position"`
	Smart      string `json:"smart"`
	SortOrder  string `json:"sort_order"`
	Filter     string `json:"filter,omitempty"`
	TimelineID string `json:"timeline,omitempty"`
}

// TaskList represents metadata about an RTM task list (publicly exposed type).
type TaskList struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Deleted   bool   `json:"deleted"`
	Locked    bool   `json:"locked"`
	Archived  bool   `json:"archived"`
	SmartList bool   `json:"smartList"`
	Position  int    `json:"position"`
}

// tagsRsp holds the response for rtm.tags.getList.
type tagsRsp struct {
	Rsp struct {
		Stat string    `json:"stat"`
		Err  *rtmError `json:"err,omitempty"`
		Tags struct {
			Tag rtmTags `json:"tag"` // MODIFIED: Use rtmTags custom type instead of []string
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
	Viewable  string `json:"viewable"`
}

// GenericTxnResult holds the common response structure for transactional RTM calls.
type GenericTxnResult struct {
	Rsp struct {
		Stat        string    `json:"stat"`
		Err         *rtmError `json:"err,omitempty"`
		Timeline    string    `json:"timeline"`
		Transaction struct {
			ID       string `json:"id"`
			Undoable string `json:"undoable"`
		} `json:"transaction"`
		List struct {
			ID         string          `json:"id"`
			Taskseries []rtmTaskSeries `json:"taskseries,omitempty"`
		} `json:"list,omitempty"`
	} `json:"rsp"`
}

// createTaskRsp holds the specific response structure for rtm.tasks.add.
type createTaskRsp struct {
	Rsp struct {
		Stat        string    `json:"stat"`
		Err         *rtmError `json:"err,omitempty"`
		Timeline    string    `json:"timeline"`
		Transaction struct {
			ID       string `json:"id"`
			Undoable string `json:"undoable"`
		} `json:"transaction"`
		List struct {
			ID         string `json:"id"`
			Taskseries struct {
				ID   string  `json:"id"`
				Name string  `json:"name"`
				Task rtmTask `json:"task"`
			} `json:"taskseries"`
		} `json:"list"`
	} `json:"rsp"`
}

// Task represents the simplified structure used for returning data via MCP tools/resources.
type Task struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	URL           string    `json:"url"`
	ListID        string    `json:"listId"`
	ListName      string    `json:"listName"`
	LocationID    string    `json:"locationId,omitempty"`
	LocationName  string    `json:"locationName,omitempty"`
	Completed     bool      `json:"completed"`
	Notes         []Note    `json:"notes"`
	Tags          []string  `json:"tags"`
	DueDate       time.Time `json:"dueDate,omitempty"`
	HasDueTime    bool      `json:"hasDueTime"`
	Priority      int       `json:"priority"`
	Postponed     int       `json:"postponed"`
	Estimate      string    `json:"estimate"`
	StartDate     time.Time `json:"startDate"`
	CompletedDate time.Time `json:"completedDate,omitempty"`
	Created       time.Time `json:"created"`
	Modified      time.Time `json:"modified"`
}

// --- Other API Response Structures --- END

// Config holds configuration settings specific to the RTM client.
// It contains API credentials, authentication token, and HTTP configuration.
type Config struct {
	APIKey       string
	SharedSecret string
	AuthToken    string       // Optional: Can be set after authentication.
	APIEndpoint  string       // Optional: Defaults if empty.
	HTTPClient   *http.Client // Optional: Defaults if nil.
}

// --- Helper Types --- START
// timelineRsp holds the response from rtm.timelines.create (needed by methods.go).
type timelineRsp struct {
	Rsp struct {
		Stat     string    `json:"stat"`
		Err      *rtmError `json:"err,omitempty"`
		Timeline string    `json:"timeline"`
	} `json:"rsp"`
}

// EnsureAuthResult defines a simpler result structure for the AuthManager.EnsureAuthenticated method.
type EnsureAuthResult struct {
	Success     bool
	Username    string
	Error       error
	AuthURL     string // Only populated if interactive flow starts.
	Frob        string // Only populated if interactive flow starts.
	NeedsManual bool   // True if interactive flow requires manual completion.
}

// TokenData represents the data stored for an authentication token (used by token storage).
type TokenData struct {
	Token     string    `json:"token"`
	UserID    string    `json:"userId,omitempty"`
	Username  string    `json:"username,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// --- Helper Types --- END
