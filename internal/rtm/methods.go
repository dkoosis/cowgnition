// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/methods.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv" // Required for string conversions.
	"strings"
	"time" // Required for time parsing.

	"github.com/cockroachdb/errors"                                   // Used for error wrapping and context.
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // Custom MCP error types.
)

// GetLists retrieves all task lists for the authenticated user.
func (c *Client) GetLists(ctx context.Context) ([]TaskList, error) { // Use exported TaskList type.
	params := map[string]string{}
	respBytes, err := c.callMethod(ctx, methodGetLists, params)
	if err != nil {
		// Add context to the error indicating the operation failed.
		return nil, errors.Wrap(err, "failed to call getLists method")
	}

	var result listsRsp // Use internal unmarshalling type.
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, errors.Wrap(err, "failed to parse getLists response")
	}

	var lists []TaskList // Use exported TaskList type.
	for _, l := range result.Rsp.Lists.List {
		// Safely convert string position to integer.
		pos, _ := strconv.Atoi(l.Position) // Ignore error for simplicity, defaults to 0.
		lists = append(lists, TaskList{    // Use exported TaskList type.
			ID:        l.ID,
			Name:      l.Name,
			Deleted:   l.Deleted == "1",
			Locked:    l.Locked == "1",
			Archived:  l.Archived == "1",
			SmartList: l.Smart == "1",
			Position:  pos,
		})
	}
	return lists, nil
}

// GetTags retrieves all tags for the authenticated user.
func (c *Client) GetTags(ctx context.Context) ([]Tag, error) { // Use exported Tag type.
	params := map[string]string{}
	respBytes, err := c.callMethod(ctx, methodGetTags, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call getTags method")
	}

	var result tagsRsp // Use internal unmarshalling type.
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, errors.Wrap(err, "failed to parse getTags response")
	}

	var tags []Tag // Use exported Tag type.
	for _, t := range result.Rsp.Tags.Tag {
		tags = append(tags, Tag{Name: t})
	}
	return tags, nil
}

// createTimeline obtains a timeline ID required for modifying operations.
func (c *Client) createTimeline(ctx context.Context) (string, error) {
	params := map[string]string{}
	respBytes, err := c.callMethod(ctx, methodCreateTimeline, params)
	if err != nil {
		return "", errors.Wrap(err, "failed to call createTimeline method")
	}

	var result timelineRsp // Use correct type from types.go.
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", errors.Wrap(err, "failed to parse createTimeline response")
	}

	timeline := result.Rsp.Timeline
	if timeline == "" {
		// Return a specific RTM error if the timeline is unexpectedly empty.
		return "", mcperrors.NewRTMError(mcperrors.ErrRTMInvalidResponse, "empty timeline received from API", nil, nil)
	}
	return timeline, nil
}

// CreateTask adds a new task to RTM using the smart-add syntax.
func (c *Client) CreateTask(ctx context.Context, name string, listID string) (*Task, error) { // Return exported Task type.
	// A timeline is required for any write operation.
	timeline, err := c.createTimeline(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create timeline for adding task")
	}

	params := map[string]string{
		"name":     name,
		"timeline": timeline,
		"parse":    "1", // Enable smart-add syntax parsing.
	}
	if listID != "" {
		params["list_id"] = listID
	}

	respBytes, err := c.callMethod(ctx, methodAddTask, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call addTask method")
	}

	var result createTaskRsp // Use internal unmarshalling type.
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, errors.Wrap(err, "failed to parse addTask response")
	}

	// Map the RTM API response structure to our internal Task struct.
	series := result.Rsp.List.Taskseries
	taskData := series.Task
	// Combine series ID and task ID for a unique identifier.
	task := &Task{ // Use exported Task type.
		ID:     fmt.Sprintf("%s_%s", series.ID, taskData.ID),
		Name:   series.Name, // RTM returns the parsed name here.
		ListID: result.Rsp.List.ID,
	}
	// Safely parse time strings.
	task.StartDate, _ = c.parseRTMTime(taskData.Added)
	task.DueDate, _ = c.parseRTMTime(taskData.Due)
	task.HasDueTime = taskData.HasDueTime == "1" // Parse HasDueTime.

	return task, nil
}

// CompleteTask marks an existing task as complete.
func (c *Client) CompleteTask(ctx context.Context, listID, taskID string) error {
	seriesID, actualTaskID, err := c.splitRTMTaskID(taskID)
	if err != nil {
		return err
	}
	if listID == "" {
		return mcperrors.NewResourceError(mcperrors.ErrResourceInvalid, "listID is required to complete a task", nil, map[string]interface{}{"taskID": taskID})
	}

	timeline, err := c.createTimeline(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create timeline for completing task")
	}

	params := map[string]string{
		"list_id":       listID,
		"taskseries_id": seriesID,
		"task_id":       actualTaskID,
		"timeline":      timeline,
	}

	_, err = c.callMethod(ctx, methodCompleteTask, params)
	if err != nil {
		return errors.Wrap(err, "failed to call completeTask method")
	}
	return nil
}

// GetTasks retrieves tasks based on an optional filter.
// Handles inconsistent JSON structures for task notes and rrule returned by the RTM API.
func (c *Client) GetTasks(ctx context.Context, filter string) ([]Task, error) { // Return exported Task type.
	params := map[string]string{}
	if filter != "" {
		params["filter"] = filter
	}

	respBytes, err := c.callMethod(ctx, methodGetTasks, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call getTasks method")
	}

	c.logger.Debug("Raw RTM getTasks response received.", "responseBody", string(respBytes))

	var result tasksRsp // Use internal unmarshalling type.
	if err := json.Unmarshal(respBytes, &result); err != nil {
		c.logger.Error("Failed to parse getTasks response JSON.",
			"error", err,
			"responseBody", string(respBytes))
		return nil, errors.Wrap(err, "failed to parse getTasks response")
	}

	var tasks []Task // Use exported Task type.
	for _, list := range result.Rsp.Tasks.List {
		for _, series := range list.TaskSeries { // Use correct case TaskSeries
			// --- Robust Note Parsing --- (No changes here)
			var taskNotes []Note
			var parsedRtmNotes []rtmNote
			if len(series.Notes) > 0 && string(series.Notes) != `""` && string(series.Notes) != `null` {
				var notesObj struct {
					Note []rtmNote `json:"note"`
				}
				errObj := json.Unmarshal(series.Notes, &notesObj)
				if errObj == nil && len(notesObj.Note) > 0 {
					parsedRtmNotes = notesObj.Note
				} else {
					var notesArr []rtmNote
					errArr := json.Unmarshal(series.Notes, &notesArr)
					if errArr == nil {
						parsedRtmNotes = notesArr
					} else {
						c.logger.Warn("Failed to parse RTM task notes field (tried object and array).",
							"rawData", string(series.Notes), "taskId", series.ID)
						parsedRtmNotes = nil
					}
				}
			} else {
				parsedRtmNotes = nil
			}
			if parsedRtmNotes != nil {
				taskNotes = c.parseRTMNotes(parsedRtmNotes)
			} else {
				taskNotes = nil
			}
			// --- End Robust Note Parsing ---

			// --- Handle Recurrence Rule (RRule) --- START
			// We don't store the rule in the Task struct, just log it if present
			if len(series.RRule) > 0 && string(series.RRule) != "null" && string(series.RRule) != `""` {
				var rruleStr string
				// Try unmarshalling as string first
				if errStr := json.Unmarshal(series.RRule, &rruleStr); errStr == nil {
					c.logger.Debug("Found recurrence rule (string).", "rrule", rruleStr, "taskId", series.ID)
					// Example: task.RecurrenceInfo = rruleStr // If Task struct had such a field
				} else {
					// If not a string, check if it's an object
					var rruleObj map[string]interface{}
					if errObj := json.Unmarshal(series.RRule, &rruleObj); errObj == nil {
						c.logger.Debug("Found complex recurrence rule (object).", "rruleData", rruleObj, "taskId", series.ID)
						// Example: task.RecurrenceInfo = "Complex Rule" // If Task struct had such a field
					} else {
						// Log if it's neither string nor standard object
						c.logger.Warn("Failed to parse rrule field (not string or object).",
							"rawData", string(series.RRule), "taskId", series.ID)
					}
				}
			}
			// --- Handle Recurrence Rule (RRule) --- END

			for _, t := range series.Task {
				if t.Deleted != "" {
					continue
				}

				task := Task{
					ID:           fmt.Sprintf("%s_%s", series.ID, t.ID),
					Name:         series.Name,
					URL:          series.URL,
					LocationID:   series.LocationID,
					LocationName: series.LocationName,
					ListID:       list.ID,
					ListName:     list.Name,
					Estimate:     t.Estimate,
					Notes:        taskNotes,
					HasDueTime:   t.HasDueTime == "1",
				}

				task.Created, _ = c.parseRTMTime(series.Created)
				task.Modified, _ = c.parseRTMTime(series.Modified)
				task.DueDate, _ = c.parseRTMTime(t.Due)
				task.StartDate, _ = c.parseRTMTime(t.Added)
				task.CompletedDate, _ = c.parseRTMTime(t.Completed)
				task.Priority = c.parseRTMPriority(t.Priority)
				task.Postponed = c.parseRTMPostponed(t.Postponed)
				task.Completed = t.Completed != ""

				if len(series.Tags) > 0 {
					task.Tags = series.Tags
				}

				tasks = append(tasks, task)
			}
		}
	}

	return tasks, nil
}

// --- Helper functions --- (parseRTMTime, parseRTMPriority, parseRTMPostponed, parseRTMNotes, splitRTMTaskID - no changes needed here)

// parseRTMTime safely parses RTM's ISO 8601 time format (UTC).
func (c *Client) parseRTMTime(timeStr string) (time.Time, error) {
	if timeStr == "" {
		return time.Time{}, nil // Return zero time if string is empty.
	}
	t, err := time.Parse("2006-01-02T15:04:05Z", timeStr)
	if err != nil {
		c.logger.Warn("Failed to parse RTM date/time.", "rawDate", timeStr, "error", err)
		return time.Time{}, err
	}
	return t, nil
}

// parseRTMPriority converts RTM priority string ("N", "1", "2", "3") to int (0-3).
func (c *Client) parseRTMPriority(priorityStr string) int {
	if priorityStr == "" || strings.ToUpper(priorityStr) == "N" {
		return 0 // Use 0 for 'No priority'.
	}
	p, err := strconv.Atoi(priorityStr)
	if err != nil || p < 1 || p > 3 {
		c.logger.Warn("Failed to parse RTM priority.", "rawPriority", priorityStr, "error", err)
		return 0
	}
	return p
}

// parseRTMPostponed converts RTM postponed count string to int.
func (c *Client) parseRTMPostponed(postponedStr string) int {
	if postponedStr == "" {
		return 0
	}
	p, err := strconv.Atoi(postponedStr)
	if err != nil {
		c.logger.Warn("Failed to parse RTM postponed count.", "rawPostponed", postponedStr, "error", err)
		return 0
	}
	return p
}

// parseRTMNotes converts internal rtmNote structures to the public Note type.
func (c *Client) parseRTMNotes(notesData []rtmNote) []Note { // Returns exported Note type.
	if len(notesData) == 0 {
		return nil // Return nil slice if input is empty.
	}
	notes := make([]Note, 0, len(notesData)) // Use exported Note type.
	for _, n := range notesData {
		createdTime, _ := c.parseRTMTime(n.Created) // Safely parse time.
		notes = append(notes, Note{                 // Use exported Note type.
			ID:        n.ID,
			Title:     n.Title,
			Text:      n.Body, // Map the '$t' field to 'Text'.
			CreatedAt: createdTime,
		})
	}
	return notes
}

// file: internal/rtm/methods.go (partial update - only adding the GetSettings function)

// GetSettings retrieves the user's RTM settings.
func (c *Client) GetSettings(ctx context.Context) (*Settings, error) {
	params := map[string]string{}
	respBytes, err := c.callMethod(ctx, methodGetSettings, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call getSettings method")
	}

	// Log raw response for debugging
	c.logger.Debug("Raw RTM getSettings response received.", "responseBody", string(respBytes))

	var result settingsRsp
	if err := json.Unmarshal(respBytes, &result); err != nil {
		c.logger.Error("Failed to parse getSettings response JSON.",
			"error", err,
			"responseBody", string(respBytes))
		return nil, errors.Wrap(err, "failed to parse getSettings response")
	}

	// Convert raw settings to the more user-friendly format
	rawSettings := result.Rsp.Settings
	settings := &Settings{
		Timezone:       rawSettings.Timezone,
		IsAmericanDate: rawSettings.DateFormat == "1",
		Is24HourTime:   rawSettings.TimeFormat == "1",
		DefaultListID:  rawSettings.DefaultList,
		Language:       rawSettings.Language,
		DefaultDueDate: rawSettings.DefaultDueDate,
		IsProAccount:   rawSettings.Pro == "1",
	}

	c.logger.Debug("Successfully parsed settings from RTM response.",
		"timezone", settings.Timezone,
		"defaultListId", settings.DefaultListID)

	return settings, nil
}

// splitRTMTaskID splits the combined task ID format "seriesID_taskID" into its components.
func (c *Client) splitRTMTaskID(combinedID string) (string, string, error) {
	parts := strings.Split(combinedID, "_")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", mcperrors.NewResourceError(mcperrors.ErrResourceInvalid,
			fmt.Sprintf("invalid task ID format: %s, expected seriesID_taskID", combinedID),
			nil,
			map[string]interface{}{"taskID": combinedID})
	}
	return parts[0], parts[1], nil
}
