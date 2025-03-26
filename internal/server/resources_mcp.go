// file: internal/server/resources_mcp.go
// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/cowgnition/cowgnition/internal/rtm"
)

// handleTasksResource retrieves and formats tasks based on the given filter.
// Returns formatted task content and any error encountered.
func (s *Server) handleTasksResource(filter string) (string, error) {
	// Get tasks from RTM
	tasksResp, err := s.rtmService.GetTasks(filter)
	if err != nil {
		return "", fmt.Errorf("handleTasksResource: error getting tasks: %w", err)
	}

	// Format tasks for display
	var sb strings.Builder

	// Add a descriptive header
	sb.WriteString(getTasksHeader(s, filter) + "\n\n")

	if len(tasksResp.Tasks.List) == 0 {
		sb.WriteString("No tasks found for the specified filter.\n")
		return sb.String(), nil
	}

	// Format and write tasks
	totalTasks, completedTasks := formatTasksList(&sb, tasksResp.Tasks.List)

	// Add summary at the end
	sb.WriteString(formatTasksSummary(totalTasks, completedTasks))

	return sb.String(), nil
}

// getTasksHeader returns an appropriate header based on the filter.
// The header provides context about the tasks being displayed.
func getTasksHeader(s *Server, filter string) string {
	header := "# All Tasks"
	if filter == "" {
		return header
	}

	if strings.Contains(filter, "due:today") {
		return "# Tasks Due Today"
	} else if strings.Contains(filter, "due:tomorrow") {
		return "# Tasks Due Tomorrow"
	} else if strings.Contains(filter, "due:\"within 7 days\"") {
		return "# Tasks Due This Week"
	} else if strings.HasPrefix(filter, "list:") {
		listID := strings.TrimPrefix(filter, "list:")
		// Try to get the list name if available
		listName := listID
		lists, err := s.rtmService.GetLists()
		if err == nil {
			for _, list := range lists {
				if list.ID == listID {
					listName = list.Name
					break
				}
			}
		}
		return fmt.Sprintf("# Tasks in List: %s", listName)
	} else if strings.Contains(filter, "tag:") {
		tag := strings.TrimPrefix(strings.Split(filter, "tag:")[1], "\"")
		tag = strings.TrimSuffix(tag, "\"")
		return fmt.Sprintf("# Tasks Tagged with: %s", tag)
	}

	return header
}

// formatTasksList writes formatted tasks to the string builder.
// Returns total and completed task counts for summary generation.
func formatTasksList(sb *strings.Builder, lists []rtm.TaskList) (int, int) {
	totalTasks := 0
	completedTasks := 0

	// Process each list
	for _, list := range lists {
		// Skip empty lists
		if len(list.TaskSeries) == 0 {
			continue
		}

		// Process each task series in the list
		for _, ts := range list.TaskSeries {
			for _, task := range ts.Tasks {
				// Skip deleted tasks
				if task.Deleted != "" {
					continue
				}

				totalTasks++
				if task.Completed != "" {
					completedTasks++
				}

				formatTask(sb, list.ID, ts, task)
			}
		}
	}

	return totalTasks, completedTasks
}

// formatTask writes a single formatted task to the string builder.
// Uses consistent formatting with appropriate markdown for MCP clients.
func formatTask(sb *strings.Builder, listID string, ts rtm.TaskSeries, task rtm.Task) {
	// Format priority
	priority, prioritySymbol := formatTaskPriority(task.Priority)

	// Format completion status
	completionSymbol := "☐"
	if task.Completed != "" {
		completionSymbol = "✅"
	}

	// Format due date
	dueDate, dueDateColor := formatTaskDueDate(task.Due)

	// Format notes indicator
	notesIndicator := ""
	if len(ts.Notes.Note) > 0 {
		notesIndicator = " 📝"
	}

	// Write task line
	taskLine := fmt.Sprintf("%s %s **%s**%s", completionSymbol, prioritySymbol, ts.Name, notesIndicator)
	sb.WriteString(taskLine + "\n")

	// Add metadata
	metadata := formatTaskMetadata(priority, dueDate, dueDateColor, ts.Tags.Tag)
	if metadata != "" {
		sb.WriteString(metadata + "\n")
	}

	// Add task ID information (needed for tool operations)
	idInfo := fmt.Sprintf("    <small>ID: list=%s, taskseries=%s, task=%s</small>",
		listID, ts.ID, task.ID)
	sb.WriteString(idInfo + "\n\n")
}

// Rest of the implementation...

// formatDate formats an RTM date string for display.
func formatDate(dueDate string) string {
	if dueDate == "" {
		return ""
	}

	// Parse the date string (format: 2006-01-02T15:04:05Z)
	t, err := time.Parse(time.RFC3339, dueDate)
	if err != nil {
		return dueDate // Return original if parsing fails
	}

	// Get today's date for comparison
	today := time.Now()
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	taskDate := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())

	// Calculate days difference
	daysDiff := int(taskDate.Sub(today).Hours() / 24)
	timeComponent := formatTimeComponent(t)

	// Format based on proximity to today
	switch {
	case daysDiff == 0:
		return "Today" + timeComponent
	case daysDiff == 1:
		return "Tomorrow" + timeComponent
	case daysDiff > 1 && daysDiff < 7:
		return t.Format("Monday") + timeComponent
	case t.Year() == today.Year():
		return t.Format("Jan 2") + timeComponent
	default:
		return t.Format("Jan 2, 2006") + timeComponent
	}
}

// formatTimeComponent returns formatted time if present, empty string otherwise.
func formatTimeComponent(t time.Time) string {
	if t.Hour() > 0 || t.Minute() > 0 {
		return fmt.Sprintf(" at %s", t.Format("3:04 PM"))
	}
	return ""
}

// handleListsResource retrieves and formats lists.
func (s *Server) handleListsResource() (string, error) {
	// Get lists from RTM
	lists, err := s.rtmService.GetLists()
	if err != nil {
		return "", fmt.Errorf("handleListsResource: error getting lists: %w", err)
	}

	// Format lists for display
	var sb strings.Builder
	sb.WriteString("# Lists\n\n")

	if len(lists) == 0 {
		sb.WriteString("No lists found in your Remember The Milk account.\n")
		return sb.String(), nil
	}

	// Group lists by type (smart, locked, normal)
	var smartLists, systemLists, normalLists []rtm.List
	for _, list := range lists {
		// Skip deleted lists
		if list.Deleted != "0" {
			continue
		}

		if list.Smart == "1" {
			smartLists = append(smartLists, list)
		} else if list.Locked == "1" {
			systemLists = append(systemLists, list)
		} else {
			normalLists = append(normalLists, list)
		}
	}

	// Write normal lists
	if len(normalLists) > 0 {
		sb.WriteString("## Your Lists\n\n")
		for _, list := range normalLists {
			formatListItem(&sb, list)
		}
		sb.WriteString("\n")
	}

	// Write smart lists
	if len(smartLists) > 0 {
		sb.WriteString("## Smart Lists\n\n")
		for _, list := range smartLists {
			formatListItem(&sb, list)
		}
		sb.WriteString("\n")
	}

	// Write system lists
	if len(systemLists) > 0 {
		sb.WriteString("## System Lists\n\n")
		for _, list := range systemLists {
			formatListItem(&sb, list)
		}
	}

	return sb.String(), nil
}

// formatListItem formats a single list item and writes it to the string builder.
func formatListItem(sb *strings.Builder, list rtm.List) {
	// Format list type
	listType := ""
	if list.Smart == "1" {
		listType = " 🔍" // Smart List
	} else if list.Locked == "1" {
		listType = " 🔒" // System List
	}

	// Format archived status
	archived := ""
	if list.Archived == "1" {
		archived = " [Archived]"
	}

	// Write list line with improved formatting
	sb.WriteString(fmt.Sprintf("- **%s**%s%s (ID: `%s`)\n",
		list.Name,
		listType,
		archived,
		list.ID))
}

// formatTasksSummary returns a summary string of task counts.
func formatTasksSummary(total, completed int) string {
	summary := fmt.Sprintf("\n**Summary:** %d task%s total",
		total, pluralS(total))

	if completed > 0 {
		remaining := total - completed
		summary += fmt.Sprintf(", %d completed, %d remaining",
			completed, remaining)
	}

	return summary + "\n"
}

// pluralS returns "s" if count is not 1, otherwise empty string.
func pluralS(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// formatTaskPriority returns formatted priority text and symbol.
func formatTaskPriority(priorityCode string) (string, string) {
	switch priorityCode {
	case "1":
		return "High", "❗"
	case "2":
		return "Medium", "❕"
	case "3":
		return "Low", "⚪"
	default:
		return "None", "  "
	}
}

// formatTaskDueDate formats the due date with visual indicators.
func formatTaskDueDate(dueDate string) (string, string) {
	if dueDate == "" {
		return "", ""
	}

	formatted := formatDate(dueDate)
	dueDateColor := ""

	// Add visual indicator for due date proximity
	timeNow := time.Now()
	parsedDate, err := time.Parse(time.RFC3339, dueDate)
	if err == nil {
		daysDiff := int(parsedDate.Sub(timeNow).Hours() / 24)
		if daysDiff < 0 {
			dueDateColor = " 🔴" // Overdue
		} else if daysDiff == 0 {
			dueDateColor = " 🟠" // Due today
		} else if daysDiff <= 2 {
			dueDateColor = " 🟡" // Due soon
		}
	}

	return formatted, dueDateColor
}

// formatTaskMetadata creates the metadata line for a task.
func formatTaskMetadata(priority, dueDate, dueDateColor string, tags []string) string {
	metadataItems := []string{}

	if dueDate != "" {
		metadataItems = append(metadataItems, fmt.Sprintf("Due: %s%s", dueDate, dueDateColor))
	}

	if priority != "None" {
		metadataItems = append(metadataItems, fmt.Sprintf("Priority: %s", priority))
	}

	if len(tags) > 0 {
		tagsFormatted := strings.Join(tags, ", ")
		metadataItems = append(metadataItems, fmt.Sprintf("Tags: %s", tagsFormatted))
	}

	if len(metadataItems) > 0 {
		return fmt.Sprintf("    _%s_", strings.Join(metadataItems, " | "))
	}

	return ""
}
