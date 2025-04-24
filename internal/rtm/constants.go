// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/constants.go

// API endpoints.
const (
	defaultAPIEndpoint = "https://api.rememberthemilk.com/services/rest/"
	authEndpoint       = "https://www.rememberthemilk.com/services/auth/"
)

// API response format.
const responseFormat = "json"

// RTM API method names. See: https://www.rememberthemilk.com/services/api/methods.rtm .
const (
	methodGetFrob        = "rtm.auth.getFrob"
	methodGetToken       = "rtm.auth.getToken"   // nolint:gosec // Keep existing nolint.
	methodCheckToken     = "rtm.auth.checkToken" //nolint:gosec // Keep existing nolint.
	methodGetLists       = "rtm.lists.getList"
	methodGetTasks       = "rtm.tasks.getList"
	methodAddTask        = "rtm.tasks.add"
	methodCompleteTask   = "rtm.tasks.complete"
	methodGetTags        = "rtm.tags.getList"
	methodCreateTimeline = "rtm.timelines.create"

	// New methods - Task updates.
	methodSetTaskName    = "rtm.tasks.setName"     //nolint:unused
	methodSetDueDate     = "rtm.tasks.setDueDate"  //nolint:unused
	methodSetPriority    = "rtm.tasks.setPriority" //nolint:unused
	methodDeleteTask     = "rtm.tasks.delete"      //nolint:unused
	methodUncompleteTask = "rtm.tasks.uncomplete"  //nolint:unused

	// New methods - List management.
	methodAddList       = "rtm.lists.add"       //nolint:unused
	methodSetListName   = "rtm.lists.setName"   //nolint:unused
	methodDeleteList    = "rtm.lists.delete"    //nolint:unused
	methodArchiveList   = "rtm.lists.archive"   //nolint:unused
	methodUnarchiveList = "rtm.lists.unarchive" //nolint:unused

	// New methods - Notes.
	methodAddNote    = "rtm.tasks.notes.add"    //nolint:unused
	methodEditNote   = "rtm.tasks.notes.edit"   //nolint:unused
	methodDeleteNote = "rtm.tasks.notes.delete" //nolint:unused

	// New methods - Tags.
	methodAddTags    = "rtm.tasks.addTags"    //nolint:unused
	methodRemoveTags = "rtm.tasks.removeTags" //nolint:unused
	methodSetTags    = "rtm.tasks.setTags"    //nolint:unused
)

// Auth permission level.
const permDelete = "delete" // Allows adding, editing and deleting tasks.

// RTM API Status Codes (Examples - Add more as needed).
const (
	rtmErrCodeInvalidAuthToken = 98
	// Add other known RTM error codes here.
)
