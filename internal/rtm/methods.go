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
func (c *Client) GetLists(ctx context.Context) ([]TaskList, error) {
	params := map[string]string{}
	respBytes, err := c.callMethod(ctx, methodGetLists, params)
	if err != nil {
		// Add context to the error indicating the operation failed.
		return nil, errors.Wrap(err, "failed to call getLists method")
	}

	var result listsRsp
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, errors.Wrap(err, "failed to parse getLists response")
	}

	var lists []TaskList
	for _, l := range result.Rsp.Lists.List {
		// Safely convert string position to integer.
		pos, _ := strconv.Atoi(l.Position) // Ignore error for simplicity, defaults to 0.
		lists = append(lists, TaskList{
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
func (c *Client) GetTags(ctx context.Context) ([]Tag, error) {
	params := map[string]string{}
	respBytes, err := c.callMethod(ctx, methodGetTags, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call getTags method")
	}

	var result tagsRsp
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, errors.Wrap(err, "failed to parse getTags response")
	}

	var tags []Tag
	for _, t := range result.Rsp.Tags.Tag {
		tags = append(tags, Tag{Name: t.Name})
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

	var result timelineRsp
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
func (c *Client) CreateTask(ctx context.Context, name string, listID string) (*Task, error) {
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

	var result createTaskRsp
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, errors.Wrap(err, "failed to parse addTask response")
	}

	// Map the RTM API response structure to our internal Task struct.
	series := result.Rsp.List.Taskseries
	taskData := series.Task
	// Combine series ID and task ID for a unique identifier.
	task := &Task{
		ID:     fmt.Sprintf("%s_%s", series.ID, taskData.ID),
		Name:   series.Name, // RTM returns the parsed name here.
		ListID: result.Rsp.List.ID,
	}
	// Safely parse time strings.
	task.StartDate, _ = c.parseRTMTime(taskData.Added)
	task.DueDate, _ = c.parseRTMTime(taskData.Due)

	return task, nil
}

// CompleteTask marks an existing task as complete.
func (c *Client) CompleteTask(ctx context.Context, listID, taskID string) error {
	// Task ID needs to be split into series and task components for the API call.
	seriesID, actualTaskID, err := c.splitRTMTaskID(taskID)
	if err != nil {
		return err // Return parsing error if ID format is invalid.
	}
	// List ID is mandatory for completing a task.
	if listID == "" {
		return mcperrors.NewResourceError("listID is required to complete a task", nil, map[string]interface{}{"taskID": taskID})
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

	// Call the API method. We don't need the response body on success.
	_, err = c.callMethod(ctx, methodCompleteTask, params)
	if err != nil {
		return errors.Wrap(err, "failed to call completeTask method")
	}
	return nil
}

// GetTasks retrieves tasks based on an optional filter.
// Handles inconsistent JSON structures for task notes returned by the RTM API.
func (c *Client) GetTasks(ctx context.Context, filter string) ([]Task, error) {
	params := map[string]string{}
	if filter != "" {
		params["filter"] = filter
	}

	respBytes, err := c.callMethod(ctx, methodGetTasks, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call getTasks method")
	}

	// Log raw response for debugging potential parsing issues.
	c.logger.Debug("Raw RTM getTasks response received", "responseBody", string(respBytes))

	var result tasksRsp
	if err := json.Unmarshal(respBytes, &result); err != nil {
		// Log detailed error if parsing the overall structure fails.
		c.logger.Error("Failed to parse getTasks response JSON",
			"error", err,
			"responseBody", string(respBytes))
		return nil, errors.Wrap(err, "failed to parse getTasks response")
	}

	var tasks []Task
	// Iterate through lists returned by the API.
	for _, list := range result.Rsp.Tasks.List {
		// Iterate through task series within each list.
		for _, series := range list.Taskseries {
			// --- Robust Note Parsing ---
			// This section handles the fact that RTM returns notes sometimes
			// as {"note": [...]} and sometimes as just [...].
			var taskNotes []Note         // Final public Note slice.
			var parsedRtmNotes []rtmNote // Intermediate slice after parsing RawMessage.

			// Check if the notes field exists and is not empty/null.
			if len(series.Notes) > 0 && string(series.Notes) != `""` && string(series.Notes) != `null` {
				// Try parsing as {"note": [...]} first.
				var notesObj struct {
					Note []rtmNote `json:"note"`
				}
				errObj := json.Unmarshal(series.Notes, &notesObj)
				if errObj == nil && len(notesObj.Note) > 0 {
					// Successfully parsed as object containing "note" array.
					parsedRtmNotes = notesObj.Note // Store the intermediate result.
				} else {
					// If object parsing failed, try parsing directly as [...].
					var notesArr []rtmNote
					errArr := json.Unmarshal(series.Notes, &notesArr)
					if errArr == nil {
						// Successfully parsed as direct array.
						parsedRtmNotes = notesArr // Store the intermediate result.
					} else {
						// Log the error if neither format could be parsed.
						c.logger.Warn("Failed to parse RTM task notes field (tried object and array)",
							"rawData", string(series.Notes),
							"objectError", fmt.Sprintf("%v", errObj), // Use %v for nil-safe error string.
							"arrayError", fmt.Sprintf("%v", errArr),
							"taskId", series.ID)
						parsedRtmNotes = nil // Ensure it's nil if parsing failed.
					}
				}
			} else {
				parsedRtmNotes = nil // No notes or empty/null notes field.
			}

			// Convert the intermediate parsed slice (if not nil) to the public []Note type.
			if parsedRtmNotes != nil {
				// Pass the correctly parsed intermediate slice here.
				taskNotes = c.parseRTMNotes(parsedRtmNotes)
			} else {
				taskNotes = nil
			}
			// --- End Robust Note Parsing ---

			// Iterate through individual task instances within the series.
			for _, t := range series.Task {
				// Skip tasks marked as deleted in the response.
				if t.Deleted != "" {
					continue
				}

				// Construct the final Task struct.
				task := Task{
					ID:           fmt.Sprintf("%s_%s", series.ID, t.ID), // Combine series and task ID.
					Name:         series.Name,
					URL:          series.URL,
					LocationID:   series.LocationID,
					LocationName: series.LocationName,
					ListID:       list.ID,
					ListName:     list.Name,
					Estimate:     t.Estimate,
					Notes:        taskNotes, // Assign the notes parsed above.
				}

				// Safely parse time and priority fields.
				task.DueDate, _ = c.parseRTMTime(t.Due)
				task.StartDate, _ = c.parseRTMTime(t.Added)
				task.CompletedDate, _ = c.parseRTMTime(t.Completed)
				task.Priority = c.parseRTMPriority(t.Priority)
				task.Postponed = c.parseRTMPostponed(t.Postponed)

				// Assign tags if present.
				if len(series.Tags.Tag) > 0 {
					task.Tags = series.Tags.Tag
				}

				tasks = append(tasks, task)
			}
		}
	}

	return tasks, nil
}

// --- Helper functions ---

// parseRTMTime safely parses RTM's ISO 8601 time format (UTC).
func (c *Client) parseRTMTime(timeStr string) (time.Time, error) {
	if timeStr == "" {
		return time.Time{}, nil // Return zero time if string is empty.
	}
	// RTM uses ISO 8601 format with UTC timezone indicator 'Z'.
	t, err := time.Parse("2006-01-02T15:04:05Z", timeStr)
	if err != nil {
		// Log failure but don't necessarily stop processing.
		c.logger.Warn("Failed to parse RTM date/time", "rawDate", timeStr, "error", err)
		return time.Time{}, err
	}
	return t, nil
}

// parseRTMPriority converts RTM priority string ("N", "1", "2", "3") to int (0-3).
func (c *Client) parseRTMPriority(priorityStr string) int {
	if priorityStr == "" || priorityStr == "N" {
		return 0 // Use 0 for 'No priority'.
	}
	p, err := strconv.Atoi(priorityStr)
	if err != nil || p < 1 || p > 3 {
		// Log invalid priority but return 0.
		c.logger.Warn("Failed to parse RTM priority", "rawPriority", priorityStr, "error", err)
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
		c.logger.Warn("Failed to parse RTM postponed count", "rawPostponed", postponedStr, "error", err)
		return 0
	}
	return p
}

// parseRTMNotes converts internal rtmNote structures to the public Note type.
func (c *Client) parseRTMNotes(notesData []rtmNote) []Note {
	if len(notesData) == 0 {
		return nil // Return nil slice if input is empty.
	}
	notes := make([]Note, 0, len(notesData))
	for _, n := range notesData {
		createdTime, _ := c.parseRTMTime(n.Created) // Safely parse time.
		notes = append(notes, Note{
			ID:        n.ID,
			Title:     n.Title,
			Text:      n.Body, // Map the '$t' field to 'Text'.
			CreatedAt: createdTime,
		})
	}
	return notes
}

// splitRTMTaskID splits the combined task ID format "seriesID_taskID" into its components.
// Returns an error if the format is invalid.
func (c *Client) splitRTMTaskID(combinedID string) (string, string, error) {
	parts := strings.Split(combinedID, "_")
	// Ensure exactly two non-empty parts exist.
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", mcperrors.NewResourceError(
			fmt.Sprintf("invalid task ID format: %s, expected seriesID_taskID", combinedID),
			nil,
			map[string]interface{}{"taskID": combinedID})
	}
	return parts[0], parts[1], nil
}

// Note: The parseTasksFromSeries helper function is no longer needed and should be removed
// if it exists elsewhere in the file, as its logic is fully contained within GetTasks now.
