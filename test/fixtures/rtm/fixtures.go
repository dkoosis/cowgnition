// Package fixtures provides test data for RTM API responses.
package fixtures

import (
	"fmt"
	"time"
)

// Constants for test data:
const (
	TestFrob        = "test_frob_12345"
	TestToken       = "test_token_abc123"
	TestUserID      = "123"
	TestUsername    = "test_user"
	TestFullname    = "Test User"
	TestTimeline    = "timeline_12345"
	TestListID      = "1"
	TestListName    = "Test List"
	TestTaskID      = "1001"
	TestSeriesID    = "2001"
	TestTagName     = "test_tag"
	TestNoteID      = "3001"
	TestNoteTitle   = "Test Note"
	TestNoteContent = "This is a test note content"
)

// GetAuthFrobResponse returns a mock response for rtm.auth.getFrob.
func GetAuthFrobResponse(frob string) string {
	if frob == "" {
		frob = TestFrob
	}
	return fmt.Sprintf(`<rsp stat="ok"><frob>%s</frob></rsp>`, frob)
}

// GetAuthTokenResponse returns a mock response for rtm.auth.getToken.
func GetAuthTokenResponse(token, perms, userID, username, fullname string) string {
	if token == "" {
		token = TestToken
	}
	if perms == "" {
		perms = "delete"
	}
	if userID == "" {
		userID = TestUserID
	}
	if username == "" {
		username = TestUsername
	}
	if fullname == "" {
		fullname = TestFullname
	}

	return fmt.Sprintf(`<rsp stat="ok">
  <auth>
    <token>%s</token>
    <perms>%s</perms>
    <user id="%s" username="%s" fullname="%s" />
  </auth>
</rsp>`, token, perms, userID, username, fullname)
}

// GetAuthCheckTokenResponse returns a mock response for rtm.auth.checkToken.
func GetAuthCheckTokenResponse(valid bool, token, perms, userID, username, fullname string) string {
	if !valid {
		return `<rsp stat="fail"><err code="98" msg="Login failed / Invalid auth token" /></rsp>`
	}

	if token == "" {
		token = TestToken
	}
	if perms == "" {
		perms = "delete"
	}
	if userID == "" {
		userID = TestUserID
	}
	if username == "" {
		username = TestUsername
	}
	if fullname == "" {
		fullname = TestFullname
	}

	return fmt.Sprintf(`<rsp stat="ok">
  <auth>
    <token>%s</token>
    <perms>%s</perms>
    <user id="%s" username="%s" fullname="%s" />
  </auth>
</rsp>`, token, perms, userID, username, fullname)
}

// GetTimelineCreateResponse returns a mock response for rtm.timelines.create.
func GetTimelineCreateResponse(timeline string) string {
	if timeline == "" {
		timeline = TestTimeline
	}
	return fmt.Sprintf(`<rsp stat="ok"><timeline>%s</timeline></rsp>`, timeline)
}

// List represents a list in the mock data.
type List struct {
	ID       string
	Name     string
	Deleted  string
	Locked   string
	Archived string
	Position string
	Smart    string
	Filter   string
}

// GetListsGetListResponse returns a mock response for rtm.lists.getList.
func GetListsGetListResponse(lists []List) string {
	if len(lists) == 0 {
		// Default lists
		lists = []List{
			{ID: "1", Name: "Inbox", Deleted: "0", Locked: "1", Archived: "0", Position: "-1", Smart: "0"},
			{ID: "2", Name: "Work", Deleted: "0", Locked: "0", Archived: "0", Position: "0", Smart: "0"},
			{ID: "3", Name: "Personal", Deleted: "0", Locked: "0", Archived: "0", Position: "1", Smart: "0"},
			{ID: "4", Name: "High Priority", Deleted: "0", Locked: "0", Archived: "0", Position: "2", Smart: "1", Filter: "(priority:1)"},
		}
	}

	var listsXML string
	for _, list := range lists {
		filterXML := ""
		if list.Smart == "1" && list.Filter != "" {
			filterXML = fmt.Sprintf("<filter>%s</filter>", list.Filter)
		}

		listsXML += fmt.Sprintf(`<list id="%s" name="%s" deleted="%s" locked="%s" archived="%s" position="%s" smart="%s">%s</list>`,
			list.ID, list.Name, list.Deleted, list.Locked, list.Archived, list.Position, list.Smart, filterXML)
	}

	return fmt.Sprintf(`<rsp stat="ok"><lists>%s</lists></rsp>`, listsXML)
}

// Task represents a task in the mock data.
type Task struct {
	ID         string
	SeriesID   string
	Name       string
	Due        string
	HasDueTime string
	Added      string
	Completed  string
	Deleted    string
	Priority   string
	Postponed  string
	Estimate   string
	ListID     string
	Tags       []string
	Notes      []Note
}

// Note represents a note in the mock data.
type Note struct {
	ID       string
	Title    string
	Content  string
	Created  string
	Modified string
}

// GetTasksGetListResponse returns a mock response for rtm.tasks.getList.
func GetTasksGetListResponse(tasks []Task) string {
	if len(tasks) == 0 {
		// Current date in ISO format
		now := time.Now().Format("2006-01-02T15:04:05Z")

		// Default tasks
		tasks = []Task{
			{
				ID: "1001", SeriesID: "2001", Name: "Buy milk", ListID: "1",
				Due: now, HasDueTime: "0", Added: now, Completed: "", Deleted: "",
				Priority: "1", Postponed: "0", Estimate: "", Tags: []string{"shopping", "grocery"},
			},
			{
				ID: "1002", SeriesID: "2002", Name: "Finish report", ListID: "2",
				Due: now, HasDueTime: "0", Added: now, Completed: "", Deleted: "",
				Priority: "2", Postponed: "0", Estimate: "", Tags: []string{"work"},
				Notes: []Note{{ID: "3001", Title: "Report details", Content: "Include sections on Q1 performance", Created: now, Modified: now}},
			},
			{
				ID: "1003", SeriesID: "2003", Name: "Call mom", ListID: "3",
				Due: "", HasDueTime: "0", Added: now, Completed: now, Deleted: "",
				Priority: "3", Postponed: "0", Estimate: "", Tags: []string{"personal"},
			},
		}
	}

	var listsMap = make(map[string]string)
	for _, task := range tasks {
		// Create task XML
		tagsXML := ""
		if len(task.Tags) > 0 {
			tagItems := ""
			for _, tag := range task.Tags {
				tagItems += fmt.Sprintf("<tag>%s</tag>", tag)
			}
			tagsXML = fmt.Sprintf("<tags>%s</tags>", tagItems)
		} else {
			tagsXML = "<tags/>"
		}

		notesXML := ""
		if len(task.Notes) > 0 {
			noteItems := ""
			for _, note := range task.Notes {
				if note.Created == "" {
					note.Created = time.Now().Format("2006-01-02T15:04:05Z")
				}
				if note.Modified == "" {
					note.Modified = note.Created
				}
				noteItems += fmt.Sprintf(`<note id="%s" created="%s" modified="%s" title="%s">%s</note>`,
					note.ID, note.Created, note.Modified, note.Title, note.Content)
			}
			notesXML = fmt.Sprintf("<notes>%s</notes>", noteItems)
		} else {
			notesXML = "<notes/>"
		}

		added := task.Added
		if added == "" {
			added = time.Now().Format("2006-01-02T15:04:05Z")
		}

		taskXML := fmt.Sprintf(`<task id="%s" due="%s" has_due_time="%s" added="%s" completed="%s" deleted="%s" priority="%s" postponed="%s" estimate="%s" />`,
			task.ID, task.Due, task.HasDueTime, added, task.Completed, task.Deleted, task.Priority, task.Postponed, task.Estimate)

		// Create task series XML
		modified := task.Added
		if modified == "" {
			modified = time.Now().Format("2006-01-02T15:04:05Z")
		}

		taskSeriesXML := fmt.Sprintf(`<taskseries id="%s" created="%s" modified="%s" name="%s" source="api">%s<participants/>%s%s</taskseries>`,
			task.SeriesID, added, modified, task.Name, tagsXML, notesXML, taskXML)

		// Add to list
		listID := task.ListID
		if listID == "" {
			listID = "1"
		}

		if existing, ok := listsMap[listID]; ok {
			listsMap[listID] = existing + taskSeriesXML
		} else {
			listsMap[listID] = taskSeriesXML
		}
	}

	// Generate final XML
	var listsXML string
	for listID, content := range listsMap {
		listsXML += fmt.Sprintf(`<list id="%s">%s</list>`, listID, content)
	}

	return fmt.Sprintf(`<rsp stat="ok"><tasks>%s</tasks></rsp>`, listsXML)
}

// GetTaskAddResponse returns a mock response for rtm.tasks.add.
func GetTaskAddResponse(listID, taskseriesID, taskID, name string) string {
	if listID == "" {
		listID = TestListID
	}
	if taskseriesID == "" {
		taskseriesID = TestSeriesID
	}
	if taskID == "" {
		taskID = TestTaskID
	}
	if name == "" {
		name = "New Task"
	}

	now := time.Now().Format("2006-01-02T15:04:05Z")

	return fmt.Sprintf(`<rsp stat="ok">
  <transaction id="12345" undoable="1" />
  <list id="%s">
    <taskseries id="%s" created="%s" modified="%s" name="%s" source="api">
      <tags/>
      <participants/>
      <notes/>
      <task id="%s" due="" has_due_time="0" added="%s" completed="" deleted="" priority="N" postponed="0" estimate="" />
    </taskseries>
  </list>
</rsp>`, listID, taskseriesID, now, now, name, taskID, now)
}

// GetTaskCompleteResponse returns a mock response for rtm.tasks.complete.
func GetTaskCompleteResponse(listID, taskseriesID, taskID string) string {
	if listID == "" {
		listID = TestListID
	}
	if taskseriesID == "" {
		taskseriesID = TestSeriesID
	}
	if taskID == "" {
		taskID = TestTaskID
	}

	now := time.Now().Format("2006-01-02T15:04:05Z")

	return fmt.Sprintf(`<rsp stat="ok">
  <transaction id="12345" undoable="1" />
  <list id="%s">
    <taskseries id="%s" created="%s" modified="%s" name="Task Name" source="api">
      <tags/>
      <participants/>
      <notes/>
      <task id="%s" due="" has_due_time="0" added="%s" completed="%s" deleted="" priority="N" postponed="0" estimate="" />
    </taskseries>
  </list>
</rsp>`, listID, taskseriesID, now, now, taskID, now, now)
}

// GetErrorResponse returns a mock error response.
func GetErrorResponse(code string, message string) string {
	return fmt.Sprintf(`<rsp stat="fail"><err code="%s" msg="%s" /></rsp>`, code, message)
}
