// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
// This file defines the primary data structures used for configuration, authentication state,
// and representing RTM entities like Tasks, Lists, and Tags within the CowGnition application.
// It also includes internal structs used specifically for parsing the nuances of RTM's API responses.
package rtm

// file: internal/rtm/types.go

import (
	"encoding/json"
	"net/http"
	"time"
)

// Config holds RTM client configuration settings necessary for API interaction.
// This includes credentials, endpoint overrides, and HTTP client settings.
type Config struct {
	// APIKey is the key obtained from RTM for application identification. Required.
	APIKey string
	// SharedSecret is the secret corresponding to the APIKey, used for signing requests. Required.
	SharedSecret string
	// APIEndpoint is the base URL for the RTM REST API. Defaults to the standard endpoint if empty.
	APIEndpoint string
	// HTTPClient is the client used for making API requests. Includes timeout settings.
	// Defaults to a standard http.Client if nil.
	HTTPClient *http.Client
	// AuthToken is the authentication token obtained after successful user authorization.
	// Stored and managed by the RTM service/client.
	AuthToken string
}

// AuthState represents the authentication state of the client with the RTM API.
// It indicates whether a valid token is held and provides user details if authenticated.
type AuthState struct {
	// IsAuthenticated is true if a valid, verified authentication token is present.
	IsAuthenticated bool `json:"isAuthenticated"`
	// Username is the RTM username associated with the current authentication token.
	Username string `json:"username,omitempty"`
	// FullName is the user's full name as registered with RTM, if available.
	FullName string `json:"fullName,omitempty"`
	// UserID is the unique RTM user ID associated with the current token.
	UserID string `json:"userId,omitempty"`
	// TokenExpires indicates when the current token might expire. Note: RTM API
	// doesn't explicitly provide token expiration, so this might be estimated or unused.
	TokenExpires time.Time `json:"tokenExpires,omitempty"`
}

// Task represents a Remember The Milk task, mapping RTM API fields to a structured Go type.
type Task struct {
	// ID is the unique identifier for the task, typically combining the series ID and task instance ID (e.g., "12345_67890").
	ID string `json:"id"`
	// Name is the title or description of the task.
	Name string `json:"name"`
	// URL provides a direct link to the task in the RTM web application, if available.
	URL string `json:"url,omitempty"`
	// DueDate is the date and time the task is due. Zero value if no due date.
	DueDate time.Time `json:"dueDate,omitempty"`
	// StartDate is the date the task was added or becomes active. Zero value if not available.
	StartDate time.Time `json:"startDate,omitempty"`
	// CompletedDate is the date and time the task was marked complete. Zero value if incomplete.
	CompletedDate time.Time `json:"completedDate,omitempty"`
	// Priority indicates the task priority (0=None, 1=High, 2=Medium, 3=Low). RTM uses 1, 2, 3.
	Priority int `json:"priority,omitempty"`
	// Postponed indicates the number of times the task has been postponed.
	Postponed int `json:"postponed,omitempty"`
	// Estimate represents the estimated time needed for the task (e.g., "1 hour").
	Estimate string `json:"estimate,omitempty"`
	// LocationID is the RTM identifier for any associated location.
	LocationID string `json:"locationId,omitempty"`
	// LocationName is the human-readable name of the associated location.
	LocationName string `json:"locationName,omitempty"`
	// Tags is a list of tags associated with the task.
	Tags []string `json:"tags,omitempty"`
	// Notes is a list of notes attached to the task.
	Notes []Note `json:"notes,omitempty"`
	// ListID is the identifier of the list this task belongs to.
	ListID string `json:"listId"`
	// ListName is the name of the list this task belongs to, populated for context.
	ListName string `json:"listName,omitempty"`
}

// Note represents a note attached to an RTM task.
type Note struct {
	// ID is the unique identifier for the note.
	ID string `json:"id"`
	// Title is the optional title of the note.
	Title string `json:"title"`
	// Text is the main content of the note.
	Text string `json:"text"`
	// CreatedAt is the timestamp when the note was created.
	CreatedAt time.Time `json:"createdAt"`
}

// TaskList represents a Remember The Milk task list (e.g., Inbox, Work, Personal).
type TaskList struct {
	// ID is the unique identifier for the list.
	ID string `json:"id"`
	// Name is the name of the list.
	Name string `json:"name"`
	// Deleted indicates if the list has been deleted.
	Deleted bool `json:"deleted"`
	// Locked indicates if the list is locked (read-only).
	Locked bool `json:"locked"`
	// Archived indicates if the list has been archived.
	Archived bool `json:"archived"`
	// Position indicates the sort order of the list in the user's list view.
	Position int `json:"position"`
	// SmartList indicates if this is a smart list defined by a filter, rather than a static list.
	SmartList bool `json:"smartList"`
}

// Tag represents a Remember The Milk tag used for organizing tasks.
type Tag struct {
	// Name is the name of the tag.
	Name string `json:"name"`
}

// --- Internal Structs for API Response Parsing ---
// These structs are used internally by the client to handle the specific JSON structure
// returned by the RTM API, which often differs slightly from the desired public types.

// baseRsp represents the common outer structure of RTM API responses.
type baseRsp struct {
	Stat string `json:"stat"`
	Err  *struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
	} `json:"err,omitempty"`
}

// frobRsp represents the specific structure for the getFrob response.
type frobRsp struct {
	Rsp struct {
		baseRsp
		Frob string `json:"frob"`
	} `json:"rsp"`
}

// tokenRsp represents the specific structure for the getToken response.
type tokenRsp struct {
	Rsp struct {
		baseRsp
		Auth struct {
			Token string `json:"token"`
			User  struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Fullname string `json:"fullname"`
			} `json:"user"`
		} `json:"auth"`
	} `json:"rsp"`
}

// checkTokenRsp represents the specific structure for the checkToken response.
type checkTokenRsp struct {
	Rsp struct {
		baseRsp
		Auth struct {
			User struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Fullname string `json:"fullname"`
			} `json:"user"`
			Token string `json:"token"` // RTM includes token in check response too.
		} `json:"auth"`
	} `json:"rsp"`
}

// listsRsp represents the specific structure for the getLists response.
type listsRsp struct {
	Rsp struct {
		baseRsp
		Lists struct {
			List []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				Deleted  string `json:"deleted"`  // "0" or "1".
				Locked   string `json:"locked"`   // "0" or "1".
				Archived string `json:"archived"` // "0" or "1".
				Position string `json:"position"` // Number as string.
				Smart    string `json:"smart"`    // "0" or "1".
			} `json:"list"`
		} `json:"lists"`
	} `json:"rsp"`
}

// tagsRsp represents the specific structure for the getTags response.
type tagsRsp struct {
	Rsp struct {
		baseRsp
		Tags struct {
			Tag []struct {
				Name string `json:"name"`
			} `json:"tag"`
		} `json:"tags"`
	} `json:"rsp"`
}

// timelineRsp represents the specific structure for the createTimeline response.
type timelineRsp struct {
	Rsp struct {
		baseRsp
		Timeline string `json:"timeline"`
	} `json:"rsp"`
}

// createTaskRsp represents the specific structure for the addTask response.
type createTaskRsp struct {
	Rsp struct {
		baseRsp
		List struct {
			ID         string `json:"id"` // List ID.
			Taskseries struct {
				ID   string `json:"id"`   // Task Series ID.
				Name string `json:"name"` // Task Name (as parsed/created).
				Task struct {
					ID    string `json:"id"`            // Task ID within series.
					Added string `json:"added"`         // ISO 8601 Timestamp.
					Due   string `json:"due,omitempty"` // ISO 8601 Timestamp.
					// Other task properties might be here depending on creation.
				} `json:"task"`
			} `json:"taskseries"`
		} `json:"list"`
	} `json:"rsp"`
}

// --- Tasks Response Structures ---
// These are complex due to RTM's nesting (List -> TaskSeries -> Task).

// tasksRsp is the top-level response structure for task list queries.
type tasksRsp struct {
	Rsp struct {
		baseRsp
		Tasks struct {
			// RTM nests task series under lists even when filtering across all lists.
			List []rtmList `json:"list"`
		} `json:"tasks"`
	} `json:"rsp"`
}

// rtmList holds task series belonging to a specific list in the API response.
type rtmList struct {
	ID         string          `json:"id"`
	Name       string          `json:"name,omitempty"` // Name might be present for context.
	Taskseries []rtmTaskSeries `json:"taskseries"`
}

// rtmTaskSeries represents a task series from the RTM API response.
// A task series groups recurring instances of a task.
type rtmTaskSeries struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
	Tags struct {
		Tag []string `json:"tag,omitempty"` // Tags are associated with the series.
	} `json:"tags"`
	// Notes are also associated with the series; RTM's JSON structure for this can be inconsistent.
	Notes        json.RawMessage `json:"notes,omitempty"`
	Task         []rtmTask       `json:"task"` // Individual task instances within the series.
	LocationID   string          `json:"location_id,omitempty"`
	LocationName string          `json:"location,omitempty"` // RTM often uses 'location' instead of 'locationName'.
}

// rtmTask represents an individual task instance within a series from the RTM API response.
type rtmTask struct {
	ID        string `json:"id"`
	Due       string `json:"due,omitempty"`       // ISO 8601 Timestamp or empty string.
	Added     string `json:"added,omitempty"`     // ISO 8601 Timestamp.
	Completed string `json:"completed,omitempty"` // ISO 8601 Timestamp or empty string.
	Deleted   string `json:"deleted,omitempty"`   // ISO 8601 Timestamp or empty string.
	Priority  string `json:"priority,omitempty"`  // "N", "1", "2", "3".
	Postponed string `json:"postponed,omitempty"` // Number as string (count).
	Estimate  string `json:"estimate,omitempty"`  // e.g., "1 hour".
}

// rtmNote represents a note attached to a task series in the RTM API response.
type rtmNote struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Body    string `json:"$t"`      // RTM uses '$t' for the note body text.
	Created string `json:"created"` // ISO 8601 Timestamp.
}
