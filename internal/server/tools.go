// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"fmt"
)

// handleAddTaskTool handles the add_task tool.
// It adds a new task to Remember The Milk.
func (s *MCPServer) handleAddTaskTool(args map[string]interface{}) (string, error) {
	// Get required arguments
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("missing or invalid 'name' argument")
	}

	// Get optional arguments
	listID := "0" // Default inbox
	if val, ok := args["list_id"].(string); ok && val != "" {
		listID = val
	}

	dueDate := ""
	if val, ok := args["due_date"].(string); ok {
		dueDate = val
	}

	// Create timeline for task operations
	timeline, err := s.rtmService.CreateTimeline()
	if err != nil {
		return "", fmt.Errorf("error creating timeline: %w", err)
	}

	// Add task
	if err := s.rtmService.AddTask(timeline, listID, name, dueDate); err != nil {
		return "", fmt.Errorf("error adding task: %w", err)
	}

	return fmt.Sprintf("Task '%s' has been added successfully.", name), nil
}

// handleCompleteTaskTool handles the complete_task tool.
// It marks a task as completed in Remember The Milk.
func (s *MCPServer) handleCompleteTaskTool(args map[string]interface{}) (string, error) {
	// Get required arguments
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid 'list_id' argument")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid 'taskseries_id' argument")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid 'task_id' argument")
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
	// Get required arguments
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid 'list_id' argument")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid 'taskseries_id' argument")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid 'task_id' argument")
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
	// Get required arguments
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid 'list_id' argument")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid 'taskseries_id' argument")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid 'task_id' argument")
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
	// Get required arguments
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid 'list_id' argument")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid 'taskseries_id' argument")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid 'task_id' argument")
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
	return fmt.Sprintf("Due date has been set to %s.", dueDate), nil
}

// handleSetPriorityTool handles the set_priority tool.
// It sets a task's priority in Remember The Milk.
func (s *MCPServer) handleSetPriorityTool(args map[string]interface{}) (string, error) {
	// Get required arguments
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid 'list_id' argument")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid 'taskseries_id' argument")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid 'task_id' argument")
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

// handleAddTagsTool handles the add_tags tool.
// It adds tags to a task in Remember The Milk.
func (s *MCPServer) handleAddTagsTool(args map[string]interface{}) (string, error) {
	// Get required arguments
	listID, ok := args["list_id"].(string)
	if !ok || listID == "" {
		return "", fmt.Errorf("missing or invalid 'list_id' argument")
	}

	taskseriesID, ok := args["taskseries_id"].(string)
	if !ok || taskseriesID == "" {
		return "", fmt.Errorf("missing or invalid 'taskseries_id' argument")
	}

	taskID, ok := args["task_id"].(string)
	if !ok || taskID == "" {
		return "", fmt.Errorf("missing or invalid 'task_id' argument")
	}

	// Get tags
	var tags []string
	if tagsArg, ok := args["tags"].([]interface{}); ok {
		for _, t := range tagsArg {
			if tagStr, ok := t.(string); ok && tagStr != "" {
				tags = append(tags, tagStr)
			}
		}
	} else if tagsStr, ok := args["tags"].(string); ok && tagsStr != "" {
		tags = []string{tagsStr}
	}

	if len(tags) == 0 {
		return "", fmt.Errorf("missing or invalid 'tags' argument")
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

	return fmt.Sprintf("Tags have been added to the task."), nil
}
