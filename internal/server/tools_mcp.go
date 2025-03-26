// file: internal/server/tools_mcp.go
// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"fmt"
	"strings"
)

// handleAddTaskTool handles the add_task tool.
// It adds a new task to Remember The Milk.
func (s *Server) handleAddTaskTool(args map[string]interface{}) (string, error) {
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
func (s *Server) handleCompleteTaskTool(args map[string]interface{}) (string, error) {
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
func (s *Server) handleUncompleteTaskTool(args map[string]interface{}) (string, error) {
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
func (s *Server) handleDeleteTaskTool(args map[string]interface{}) (string, error) {
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
func (s *Server) handleSetDueDateTool(args map[string]interface{}) (string, error) {
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

// Helper functions

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

// Other functions...
