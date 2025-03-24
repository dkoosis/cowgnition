// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
)

// CreateTimeline creates a new timeline for making changes to tasks.
// A timeline is required for any operations that modify data in RTM.
// It helps RTM track changes and enables undo functionality.
func (c *Client) CreateTimeline() (string, error) {
	ctx := context.Background()
	resp, err := c.callMethod(ctx, "rtm.timelines.create", nil)
	if err != nil {
		return "", err
	}

	var result struct {
		Timeline string `xml:"timeline"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("CreateTimeline: failed to parse timeline response: %w", err)
	}

	return result.Timeline, nil
}

// GetTasks gets tasks from the RTM API with optional filtering.
// The filter parameter uses RTM's search syntax to filter the tasks returned.
func (c *Client) GetTasks(filter string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	if filter != "" {
		params.Set("filter", filter)
	}

	return c.callMethod(ctx, "rtm.tasks.getList", params)
}

// AddTask adds a new task to the specified list.
// The timeline parameter should be obtained by calling CreateTimeline.
func (c *Client) AddTask(timeline, name, listID string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("name", name)
	if listID != "" {
		params.Set("list_id", listID)
	}

	return c.callMethod(ctx, "rtm.tasks.add", params)
}

// CompleteTask marks a task as complete.
// The timeline parameter should be obtained by calling CreateTimeline.
func (c *Client) CompleteTask(timeline, listID, taskseriesID, taskID string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)

	return c.callMethod(ctx, "rtm.tasks.complete", params)
}

// DeleteTask deletes a task.
// The timeline parameter should be obtained by calling CreateTimeline.
func (c *Client) DeleteTask(timeline, listID, taskseriesID, taskID string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)

	return c.callMethod(ctx, "rtm.tasks.delete", params)
}

// SetTaskDueDate sets the due date for a task.
// The timeline parameter should be obtained by calling CreateTimeline.
// The due parameter can be a date string in various formats (e.g., "today", "tomorrow", "2023-12-31").
// To clear the due date, pass an empty string.
func (c *Client) SetTaskDueDate(timeline, listID, taskseriesID, taskID, due string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("due", due)

	return c.callMethod(ctx, "rtm.tasks.setDueDate", params)
}

// SetTaskPriority sets the priority for a task.
// The timeline parameter should be obtained by calling CreateTimeline.
// Priority values: 1 (high), 2 (medium), 3 (low), or 0/N (none).
func (c *Client) SetTaskPriority(timeline, listID, taskseriesID, taskID, priority string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("priority", priority)

	return c.callMethod(ctx, "rtm.tasks.setPriority", params)
}

// AddTags adds tags to a task.
// The timeline parameter should be obtained by calling CreateTimeline.
// The tags parameter should be a comma-separated list of tags.
func (c *Client) AddTags(timeline, listID, taskseriesID, taskID, tags string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("tags", tags)

	return c.callMethod(ctx, "rtm.tasks.addTags", params)
}

// RemoveTags removes tags from a task.
// The timeline parameter should be obtained by calling CreateTimeline.
// The tags parameter should be a comma-separated list of tags to remove.
func (c *Client) RemoveTags(timeline, listID, taskseriesID, taskID, tags string) ([]byte, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("timeline", timeline)
	params.Set("list_id", listID)
	params.Set("taskseries_id", taskseriesID)
	params.Set("task_id", taskID)
	params.Set("tags", tags)

	return c.callMethod(ctx, "rtm.tasks.removeTags", params)
}
