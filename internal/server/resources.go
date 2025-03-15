// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"fmt"
	"sort"
	"strings"
)

// handleTasksResource retrieves and formats tasks based on the given filter.
func (s *MCPServer) handleTasksResource(filter string) (string, error) {
	// Get tasks from RTM
	tasksResp, err := s.rtmService.GetTasks(filter)
	if err != nil {
		return "", fmt.Errorf("error getting tasks: %w", err)
	}

	// Format tasks for display
	var sb strings.Builder
	sb.WriteString("# Tasks\n\n")

	if len(tasksResp.Tasks.List) == 0 {
		sb.WriteString("No tasks found.\n")
		return sb.String(), nil
	}

	// Process each list
	for _, list := range tasksResp.Tasks.List {
		// Skip empty lists
		if len(list.Taskseries) == 0 {
			continue
		}

		// Process each task series in the list
		for _, ts := range list.Taskseries {
			for _, task := range ts.Tasks {
				// Skip deleted tasks
				if task.Deleted != "" {
					continue
				}

				// Format priority
				priority := ""
				switch task.Priority {
				case "1":
					priority = "❗ "
				case "2":
					priority = "❕ "
				case "3":
					priority = "⚪ "
				}

				// Format completion status
				completed := " "
				if task.Completed != "" {
					completed = "✓ "
				}

				// Format due date
				dueDate := ""
				if task.Due != "" {
					dueDate = " (Due: " + formatDate(task.Due) + ")"
				}

				// Format tags
				tags := ""
				if len(ts.Tags.Tag) > 0 {
					tags = " [" + strings.Join(ts.Tags.Tag, ", ") + "]"
				}

				// Write task line
				sb.WriteString(fmt.Sprintf("%s%s%s%s%s\n", 
					priority, 
					completed, 
					ts.Name, 
					dueDate, 
					tags))
			}
		}
	}

	return sb.String(), nil
}

// handleListsResource retrieves and formats lists.
func (s *MCPServer) handleListsResource() (string, error) {
	// Get lists from RTM
	lists, err := s.rtmService.GetLists()
	if err != nil {
		return "", fmt.Errorf("error getting lists: %w", err)
	}

	// Format lists for display
	var sb strings.Builder
	sb.WriteString("# Lists\n\n")

	if len(lists) == 0 {
		sb.WriteString("No lists found.\n")
		return sb.String(), nil
	}

	// Process each list
	for _, list := range lists {
		// Skip deleted lists
		if list.Deleted != "0" {
			continue
		}

		// Format list type
		listType := ""
		if list.Smart == "1" {
			listType = " (Smart List)"
		} else if list.Locked == "1" {
			listType = " (System List)"
		}

		// Format archived status
		archived := ""
		if list.Archived == "1" {
			archived = " [Archived]"
		}

		// Write list line
		sb.WriteString(fmt.Sprintf("- %s (ID: %s)%s%s\n", 
			list.Name, 
			list.ID, 
			listType, 
			archived))
	}

	return sb.String(), nil
}

// handleTagsResource retrieves and formats tags.
func (s *MCPServer) handleTagsResource() (string, error) {
	// Get tasks from RTM to extract tags
	tasksResp, err := s.rtmService.GetTasks("")
	if err != nil {
		return "", fmt.Errorf("error getting tasks for tags: %w", err)
	}

	// Extract unique tags
	tagMap := make(map[string]int)
	
	for _, list := range tasksResp.Tasks.List {
		for _, ts := range list.Taskseries {
			for _, tag := range ts.Tags.Tag {
				if tag != "" {
					tagMap[tag]++
				}
			}
		}
	}

	// Format tags for display
	var sb strings.Builder
	sb.WriteString("# Tags\n\n")

	if len(tagMap) == 0 {
		sb.WriteString("No tags found.\n")
		return sb.String(), nil
	}

	// Sort tags alphabetically
	tags := make([]string, 0, len(tagMap))
	for tag := range tagMap {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	// Process each tag
	for _, tag := range tags {
		count := tagMap[tag]
		sb.WriteString(fmt.Sprintf("- %s (%d task", tag, count))
		if count != 1 {
			sb.WriteString("s")
		}
		sb.WriteString(")\n")
	}

	return sb.String(), nil
}
