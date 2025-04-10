package rtm

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv" // Import strconv
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
)

// GetLists retrieves all task lists for the authenticated user.
func (c *Client) GetLists(ctx context.Context) ([]TaskList, error) {
	params := map[string]string{}
	respBytes, err := c.callMethod(ctx, methodGetLists, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call getLists method") // Error context added
	}

	var result listsRsp
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, errors.Wrap(err, "failed to parse getLists response")
	}

	var lists []TaskList
	for _, l := range result.Rsp.Lists.List {
		pos, _ := strconv.Atoi(l.Position) // Ignore error for simplicity, default 0
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
		return "", mcperrors.NewRTMError(mcperrors.ErrRTMInvalidResponse, "empty timeline received from API", nil, nil)
	}
	return timeline, nil
}

// CreateTask adds a new task to RTM.
func (c *Client) CreateTask(ctx context.Context, name string, listID string) (*Task, error) {
	timeline, err := c.createTimeline(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create timeline for adding task")
	}

	params := map[string]string{
		"name":     name,
		"timeline": timeline,
		// "parse": "1", // Add this if smart-add syntax is desired
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

	// Map response to Task struct
	series := result.Rsp.List.Taskseries
	taskData := series.Task
	task := &Task{
		ID:     fmt.Sprintf("%s_%s", series.ID, taskData.ID),
		Name:   series.Name,
		ListID: result.Rsp.List.ID,
	}
	task.StartDate, _ = c.parseRTMTime(taskData.Added) // Ignore parse error for simplicity
	task.DueDate, _ = c.parseRTMTime(taskData.Due)     // Ignore parse error

	return task, nil
}

// CompleteTask marks an existing task as complete.
func (c *Client) CompleteTask(ctx context.Context, listID, taskID string) error {
	seriesID, actualTaskID, err := c.splitRTMTaskID(taskID)
	if err != nil {
		return err // Return parsing error
	}
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

	_, err = c.callMethod(ctx, methodCompleteTask, params)
	if err != nil {
		return errors.Wrap(err, "failed to call completeTask method")
	}
	return nil
}

// GetTasks retrieves tasks based on a filter, refactored for lower complexity.
func (c *Client) GetTasks(ctx context.Context, filter string) ([]Task, error) {
	params := map[string]string{}
	if filter != "" {
		params["filter"] = filter
	}

	respBytes, err := c.callMethod(ctx, methodGetTasks, params)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call getTasks method")
	}

	var result tasksRsp
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, errors.Wrap(err, "failed to parse getTasks response")
	}

	var tasks []Task
	for _, list := range result.Rsp.Tasks.List {
		for _, series := range list.Taskseries {
			tasks = c.parseTasksFromSeries(series, list.ID, list.Name, tasks)
		}
	}

	return tasks, nil
}

// --- Helper functions for GetTasks ---

// parseTasksFromSeries processes a task series and appends tasks to the list.
func (c *Client) parseTasksFromSeries(series rtmTaskSeries, listID, listName string, tasks []Task) []Task {
	for _, t := range series.Task {
		if t.Deleted != "" { // Skip deleted tasks
			continue
		}

		task := Task{
			ID:           fmt.Sprintf("%s_%s", series.ID, t.ID),
			Name:         series.Name,
			URL:          series.URL,
			LocationID:   series.LocationID,
			LocationName: series.LocationName,
			ListID:       listID,
			ListName:     listName,
			Estimate:     t.Estimate,
		}

		task.DueDate, _ = c.parseRTMTime(t.Due)
		task.StartDate, _ = c.parseRTMTime(t.Added)
		task.CompletedDate, _ = c.parseRTMTime(t.Completed)

		task.Priority = c.parseRTMPriority(t.Priority)
		task.Postponed = c.parseRTMPostponed(t.Postponed)

		if len(series.Tags.Tag) > 0 {
			task.Tags = series.Tags.Tag
		}

		task.Notes = c.parseRTMNotes(series.Notes.Note)

		tasks = append(tasks, task)
	}
	return tasks
}

// parseRTMTime safely parses RTM's ISO 8601 time format.
func (c *Client) parseRTMTime(timeStr string) (time.Time, error) {
	if timeStr == "" {
		return time.Time{}, nil // Return zero time if string is empty
	}
	t, err := time.Parse("2006-01-02T15:04:05Z", timeStr)
	if err != nil {
		c.logger.Warn("Failed to parse RTM date/time", "rawDate", timeStr, "error", err)
		return time.Time{}, err
	}
	return t, nil
}

// parseRTMPriority converts RTM priority string ("N", "1", "2", "3") to int.
func (c *Client) parseRTMPriority(priorityStr string) int {
	if priorityStr == "" || priorityStr == "N" {
		return 0 // Use 0 or 4 for 'No priority' consistently
	}
	p, err := strconv.Atoi(priorityStr)
	if err != nil || p < 1 || p > 3 {
		c.logger.Warn("Failed to parse RTM priority", "rawPriority", priorityStr, "error", err)
		return 0
	}
	return p
}

// parseRTMPostponed converts RTM postponed string to int.
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

// parseRTMNotes converts RTM note structures to []Note.
func (c *Client) parseRTMNotes(notesData []rtmNote) []Note {
	if len(notesData) == 0 {
		return nil
	}
	notes := make([]Note, 0, len(notesData))
	for _, n := range notesData {
		createdTime, _ := c.parseRTMTime(n.Created)
		notes = append(notes, Note{
			ID:        n.ID,
			Title:     n.Title,
			Text:      n.Body,
			CreatedAt: createdTime,
		})
	}
	return notes
}

// splitRTMTaskID splits the combined task ID "seriesID_taskID" into parts.
func (c *Client) splitRTMTaskID(combinedID string) (string, string, error) {
	parts := strings.Split(combinedID, "_")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", mcperrors.NewResourceError(
			fmt.Sprintf("invalid task ID format: %s, expected seriesID_taskID", combinedID),
			nil,
			map[string]interface{}{"taskID": combinedID})
	}
	return parts[0], parts[1], nil
}
