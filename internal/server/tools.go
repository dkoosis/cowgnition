// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"fmt"
	"strings"
	"time"
)

// extractStringArg safely extracts a string argument with default value.
func extractStringArg(args map[string]interface{}, key string, defaultVal string) string {
	if val, ok := args[key].(string); ok && val != "" {
		return val
	}
	return defaultVal
}

// extractTags extracts tags from either string or array format.
func extractTags(args map[string]interface{}) []string {
	tags := []string{}

	// Extract from string
	if val, ok := args["tags"].(string); ok && val != "" {
		for _, tag := range strings.Split(val, ",") {
			if trimmed := strings.TrimSpace(tag); trimmed != "" {
				tags = append(tags, trimmed)
			}
		}
		return tags
	}

	// Extract from array
	if valArray, ok := args["tags"].([]interface{}); ok {
		for _, t := range valArray {
			if tagStr, ok := t.(string); ok && tagStr != "" {
				tags = append(tags, strings.TrimSpace(tagStr))
			}
		}
	}

	return tags
}

// handleAddTaskTool handles the add_task tool.
// It adds a new task to Remember The Milk.
func (s *MCPServer) handleAddTaskTool(args map[string]interface{}) (string, error) {
	// Get required arguments
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("missing or invalid 'name' argument")
	}

	// Extract optional arguments
	listID := extractStringArg(args, "list_id", "0") // Default to inbox
	dueDate := extractStringArg(args, "due_date", "")
	priority := extractStringArg(args, "priority", "")
	tags := extractTags(args)

	// Create timeline for task operations
	timeline, err := s.rtmService.CreateTimeline()
	if err != nil {
		return "", fmt.Errorf("error creating timeline: %w", err)
	}

	// Add task
	if err := s.rtmService.AddTask(timeline, listID, name, dueDate); err != nil {
		return "", fmt.Errorf("error adding task: %w", err)
	}

	// Format the success message
	var resultMsg strings.Builder
	resultMsg.WriteString(fmt.Sprintf("Task '%s' has been added successfully", name))

	if priority != "" {
		resultMsg.WriteString(fmt.Sprintf(" with priority %s", priority))
	}

	if len(tags) > 0 {
		resultMsg.WriteString(fmt.Sprintf(" with tags: %s", strings.Join(tags, ", ")))
	}

	return resultMsg.String() + ".", nil
}

// handleCompleteTaskTool handles the complete_task tool.
// It marks a task as completed in Remember The Milk.
func (s *MCPServer) handleCompleteTaskTool(args map[string]interface{}) (string, error) {
	// Extract task IDs
	listID, taskseriesID, taskID, err := extractTaskIDs(args)
	if err != nil {
		return "", err
	}

	// Create timeline for task operations
	timeline, err := s.rtmService.CreateTimeline()
	if err != nil {
		return "", fmt.Errorf("error creating timeline: %w", err)
	}

	// Complete task
	if err := s.rtmService.CompleteTask(timeline, listID, taskseriesID, taskID); err != nil {
		return "", fmt.Errorf("error completing task: %w", err)
	}

	return "Task has been marked as completed.", nil
}

// handleUncompleteTaskTool handles the uncomplete_task tool.
// It marks a completed task as incomplete in Remember The Milk.
func (s *MCPServer) handleUncompleteTaskTool(args map[string]interface{}) (string, error) {
	// Extract task IDs
	listID, taskseriesID, taskID, err := extractTaskIDs(args)
	if err != nil {
		return "", err
	}

	// Create timeline for task operations
	timeline, err := s.rtmService.CreateTimeline()
	if err != nil {
		return "", fmt.Errorf("error creating timeline: %w", err)
	}

	// Uncomplete task
	if err := s.rtmService.UncompleteTask(timeline, listID, taskseriesID, taskID); err != nil {
		return "", fmt.Errorf("error uncompleting task: %w", err)
	}

	return "Task has been marked as incomplete.", nil
}

// handleDeleteTaskTool handles the delete_task tool.
// It deletes a task in Remember The Milk.
func (s *MCPServer) handleDeleteTaskTool(args map[string]interface{}) (string, error) {
	// Extract task IDs
	listID, taskseriesID, taskID, err := extractTaskIDs(args)
	if err != nil {
		return "", err
	}

	// Create timeline for task operations
	timeline, err := s.rtmService.CreateTimeline()
	if err != nil {
		return "", fmt.Errorf("error creating timeline: %w", err)
	}

	// Delete task
	if err := s.rtmService.DeleteTask(timeline, listID, taskseriesID, taskID); err != nil {
		return "", fmt.Errorf("error deleting task: %w", err)
	}

	return "Task has been deleted.", nil
}

// handleSetDueDateTool handles the set_due_date tool.
// It sets or updates a task's due date in Remember The Milk.
func (s *MCPServer) handleSetDueDateTool(args map[string]interface{}) (string, error) {
	// Extract task IDs
	listID, taskseriesID, taskID, err := extractTaskIDs(args)
	if err != nil {
		return "", err
	}

	// Get due date
	dueDate := ""
	if val, ok := args["due_date"].(string); ok {
		dueDate = val
	}

	// Check if due date has time component
	hasDueTime := false
	if val, ok := args["has_due_time"].(bool); ok {
		hasDueTime = val
	}

	// Create timeline for task operations
	timeline, err := s.rtmService.CreateTimeline()
	if err != nil {
		return "", fmt.Errorf("error creating timeline: %w", err)
	}

	// Set due date
	if err := s.rtmService.SetDueDate(timeline, listID, taskseriesID, taskID, dueDate, hasDueTime); err != nil {
		return "", fmt.Errorf("error setting due date: %w", err)
	}

	if dueDate == "" {
		return "Due date has been cleared.", nil
	}
	return fmt.Sprintf("Due date has been set to %s.", formatDueDate(dueDate, hasDueTime)), nil
}

// formatDueDate provides a human-friendly format for due dates.
func formatDueDate(dueDate string, hasTime bool) string {
	// Try to parse as RFC3339
	if t, err := time.Parse(time.RFC3339, dueDate); err == nil {
		if hasTime {
			return t.Format("Monday, January 2, 2006 at 3:04 PM")
		}
		return t.Format("Monday, January 2, 2006")
	}

	// Return as-is if parsing fails
	return dueDate
}

// handleSetPriorityTool handles the set_priority tool.
// It sets a task's priority in Remember The Milk.
func (s *MCPServer) handleSetPriorityTool(args map[string]interface{}) (string, error) {
	// Extract task IDs
	listID, taskseriesID, taskID, err := extractTaskIDs(args)
	if err != nil {
		return "", err
	}

	priority, ok := args["priority"].(string)
	if !ok || priority == "" {
		return "", fmt.Errorf("missing or invalid 'priority' argument")
	}

	// Create timeline for task operations
	timeline, err := s.rtmService.CreateTimeline()
	if err != nil {
		return "", fmt.Errorf("error creating timeline: %w", err)
	}

	// Validate priority value
	validPriorities := map[string]bool{"0": true, "1": true, "2": true, "3": true, "none": true, "high": true, "medium": true, "low": true}
	normalizedPriority := strings.ToLower(priority)

	// Convert text priorities to numeric
	if normalizedPriority == "high" {
		priority = "1"
	} else if normalizedPriority == "medium" {
		priority = "2"
	} else if normalizedPriority == "low" {
		priority = "3"
	} else if normalizedPriority == "none" {
		priority = "0"
	}

	if !validPriorities[normalizedPriority] {
		return "", fmt.Errorf("invalid priority value: must be 0-3, none, low, medium, or high")
	}

	// Set priority
	if err := s.rtmService.SetPriority(timeline, listID, taskseriesID, taskID, priority); err != nil {
		return "", fmt.Errorf("error setting priority: %w", err)
	}

	// Format priority for display
	priorityDisplay := "none"
	switch priority {
	case "1":
		priorityDisplay = "high"
	case "2":
		priorityDisplay = "medium"
	case "3":
		priorityDisplay = "low"
	}

	return fmt.Sprintf("Priority has been set to %s.", priorityDisplay), nil
}

// parseTagArgument extracts tags from the tags argument, which can be a string or array.
func parseTagArgument(tagsArg interface{}) ([]string, error) {
	var tags []string

	// Handle different tag formats
	switch t := tagsArg.(type) {
	case []interface{}:
		for _, tagItem := range t {
			if tagStr, ok := tagItem.(string); ok && tagStr != "" {
				tags = append(tags, strings.TrimSpace(tagStr))
			}
		}
	case string:
		if t != "" {
			for _, tag := range strings.Split(t, ",") {
				trimmed := strings.TrimSpace(tag)
				if trimmed != "" {
					tags = append(tags, trimmed)
				}
			}
		}
	case nil:
		return nil, fmt.Errorf("missing 'tags' argument")
	default:
		return nil, fmt.Errorf("invalid 'tags' argument type: must be string or array")
	}

	if len(tags) == 0 {
		return nil, fmt.Errorf("no valid tags provided")
	}

	return tags, nil
}

// handleAddTagsTool handles the add_tags tool.
// It adds tags to a task in Remember The Milk.
func (s *MCPServer) handleAddTagsTool(args map[string]interface{}) (string, error) {
	// Extract task IDs first
	listID, taskseriesID, taskID, err := extractTaskIDs(args)
	if err != nil {
		return "", err
	}

	// Parse tags
	tags, err := parseTagArgument(args["tags"])
	if err != nil {
		return "", err
	}

	// Create timeline for task operations
	timeline, err := s.rtmService.CreateTimeline()
	if err != nil {
		return "", fmt.Errorf("error creating timeline: %w", err)
	}

	// Add tags
	if err := s.rtmService.AddTags(timeline, listID, taskseriesID, taskID, tags); err != nil {
		return "", fmt.Errorf("error adding tags: %w", err)
	}

	// Use plural form correctly
	tagWord := "tag"
	if len(tags) > 1 {
		tagWord = "tags"
	}

	return fmt.Sprintf("%d %s (%s) added to the task.", len(tags), tagWord, strings.Join(tags, ", ")), nil
}

// extractTaskIDs extracts the standard task identifiers from args map.
func extractTaskIDs(args map[string]interface{}) (listID, taskseriesID, taskID string, err error) {
	var ok bool

	// Get list_id
	listID, ok = args["list_id"].(string)
	if !ok || listID == "" {
		return "", "", "", fmt.Errorf("missing or invalid 'list_id' argument")
	}

	// Get taskseries_id
	taskseriesID, ok = args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", "", "", fmt.Errorf("missing or invalid 'taskseries_id' argument")
	}

	// Get task_id
	taskID, ok = args["task_id"].(string)
	if !ok || taskID == "" {
		return "", "", "", fmt.Errorf("missing or invalid 'task_id' argument")
	}

	return listID, taskseriesID, taskID, nil
}
