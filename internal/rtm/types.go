// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/types.go

import (
	"net/http"
	"time"
)

// Config holds RTM client configuration.
type Config struct {
	APIKey       string
	SharedSecret string
	APIEndpoint  string
	HTTPClient   *http.Client // Note: You'll need to import "net/http"
	AuthToken    string
}

// AuthState represents the authentication state of the client.
type AuthState struct {
	IsAuthenticated bool      `json:"isAuthenticated"`
	Username        string    `json:"username,omitempty"`
	FullName        string    `json:"fullName,omitempty"`
	UserID          string    `json:"userId,omitempty"`
	TokenExpires    time.Time `json:"tokenExpires,omitempty"` // Note: RTM API doesn't provide this
}

// Task represents a Remember The Milk task.
type Task struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	URL           string    `json:"url,omitempty"`
	DueDate       time.Time `json:"dueDate,omitempty"`
	StartDate     time.Time `json:"startDate,omitempty"`
	CompletedDate time.Time `json:"completedDate,omitempty"`
	Priority      int       `json:"priority,omitempty"`
	Postponed     int       `json:"postponed,omitempty"`
	Estimate      string    `json:"estimate,omitempty"`
	LocationID    string    `json:"locationId,omitempty"`
	LocationName  string    `json:"locationName,omitempty"`
	Tags          []string  `json:"tags,omitempty"`
	Notes         []Note    `json:"notes,omitempty"`
	ListID        string    `json:"listId"`
	ListName      string    `json:"listName,omitempty"`
}

// Note represents a note attached to a task.
type Note struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

// TaskList represents a Remember The Milk task list.
type TaskList struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Deleted   bool   `json:"deleted"`
	Locked    bool   `json:"locked"`
	Archived  bool   `json:"archived"`
	Position  int    `json:"position"`
	SmartList bool   `json:"smartList"`
	// TasksCount int    `json:"tasksCount,omitempty"` // Removed as it's not from API
}

// Tag represents a Remember The Milk tag.
type Tag struct {
	Name string `json:"name"`
	// TasksCount int    `json:"tasksCount,omitempty"` // Removed as it's not from API
}

// --- Internal Structs for API Response Parsing ---

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
			Token string `json:"token"` // RTM includes token in check response too
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
				Deleted  string `json:"deleted"`  // "0" or "1"
				Locked   string `json:"locked"`   // "0" or "1"
				Archived string `json:"archived"` // "0" or "1"
				Position string `json:"position"` // Number as string
				Smart    string `json:"smart"`    // "0" or "1"
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
			ID         string `json:"id"` // List ID
			Taskseries struct {
				ID   string `json:"id"`   // Task Series ID
				Name string `json:"name"` // Task Name (as parsed/created)
				Task struct {
					ID    string `json:"id"`            // Task ID within series
					Added string `json:"added"`         // ISO 8601 Timestamp
					Due   string `json:"due,omitempty"` // ISO 8601 Timestamp
					// Other task properties might be here depending on creation
				} `json:"task"`
			} `json:"taskseries"`
		} `json:"list"`
	} `json:"rsp"`
}

// --- Tasks Response Structures ---
// These are complex due to RTM's nesting (List -> TaskSeries -> Task)

type tasksRsp struct {
	Rsp struct {
		baseRsp
		Tasks struct {
			List []rtmList `json:"list"`
		} `json:"tasks"`
	} `json:"rsp"`
}

type rtmList struct {
	ID         string          `json:"id"`
	Name       string          `json:"name,omitempty"`
	Taskseries []rtmTaskSeries `json:"taskseries"`
}

type rtmTaskSeries struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
	Tags struct {
		Tag []string `json:"tag,omitempty"`
	} `json:"tags"`
	Notes        []rtmNote `json:"notes,omitempty"`
	Task         []rtmTask `json:"task"`
	LocationID   string    `json:"location_id,omitempty"`
	LocationName string    `json:"location,omitempty"` // RTM uses 'location' for name
}

type rtmTask struct {
	ID        string `json:"id"`
	Due       string `json:"due,omitempty"`       // ISO 8601 Timestamp
	Added     string `json:"added,omitempty"`     // ISO 8601 Timestamp
	Completed string `json:"completed,omitempty"` // ISO 8601 Timestamp
	Deleted   string `json:"deleted,omitempty"`   // ISO 8601 Timestamp or ""
	Priority  string `json:"priority,omitempty"`  // "N", "1", "2", "3"
	Postponed string `json:"postponed,omitempty"` // Number as string
	Estimate  string `json:"estimate,omitempty"`  // e.g., "1 hour"
}

type rtmNote struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Body    string `json:"$t"`      // Special field name for the note text
	Created string `json:"created"` // ISO 8601 Timestamp
}
