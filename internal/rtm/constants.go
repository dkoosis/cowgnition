// file: internal/rtm/constants.go
package rtm

// API endpoints.
const (
	defaultAPIEndpoint = "https://api.rememberthemilk.com/services/rest/"
	authEndpoint       = "https://www.rememberthemilk.com/services/auth/"
)

// API response format.
const responseFormat = "json"

// RTM API method names.
const (
	methodGetFrob        = "rtm.auth.getFrob"
	methodGetToken       = "rtm.auth.getToken"   // nolint:gosec
	methodCheckToken     = "rtm.auth.checkToken" //nolint:gosec
	methodGetLists       = "rtm.lists.getList"
	methodGetTasks       = "rtm.tasks.getList"
	methodAddTask        = "rtm.tasks.add"
	methodCompleteTask   = "rtm.tasks.complete"
	methodGetTags        = "rtm.tags.getList"
	methodCreateTimeline = "rtm.timelines.create" // Added constant
)

// Auth permission level.
const permDelete = "delete" // Allows adding, editing and deleting tasks.

// RTM API Status Codes (Examples - Add more as needed).
const (
	rtmErrCodeInvalidAuthToken = 98
	// Add other known RTM error codes here.
)
