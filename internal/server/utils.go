// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ErrorResponse represents a standardized error response.
type ErrorResponse struct {
	Error     string `json:"error"`
	Status    int    `json:"status"`
	Path      string `json:"path,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Timestamp string `json:"timestamp"`
}

// writeJSONResponse writes a JSON response with the given status code and data.
func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		// If we can't encode the intended response, fall back to a simple error
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// writeErrorResponse writes a JSON error response with the given status code and message.
func writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	errorResponse := ErrorResponse{
		Error:     message,
		Status:    statusCode,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Path:      "", // Path could be populated from request if needed
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		log.Printf("Error encoding error response: %v", err)
		// If we can't encode the error response, fall back to a simple error
		http.Error(w, message, statusCode)
	}
}

// formatDate formats an RTM date string for display.
func formatDate(dateStr string) string {
	if dateStr == "" {
		return ""
	}

	// Parse the date string (format: 2006-01-02T15:04:05Z)
	t, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		// If parsing fails, return the original string
		return dateStr
	}

	// Today
	today := time.Now()
	if t.Year() == today.Year() && t.Month() == today.Month() && t.Day() == today.Day() {
		return "Today"
	}

	// Tomorrow
	tomorrow := today.AddDate(0, 0, 1)
	if t.Year() == tomorrow.Year() && t.Month() == tomorrow.Month() && t.Day() == tomorrow.Day() {
		return "Tomorrow"
	}

	// This week
	if t.Before(today.AddDate(0, 0, 7)) {
		return t.Format("Monday")
	}

	// Default format
	return t.Format("Jan 2")
}

// validateResourceName checks if a resource name is valid.
func validateResourceName(name string) bool {
	validResources := map[string]bool{
		"auth://rtm":       true,
		"tasks://all":      true,
		"tasks://today":    true,
		"tasks://tomorrow": true,
		"tasks://week":     true,
		"lists://all":      true,
		"tags://all":       true,
	}

	// Direct match
	if validResources[name] {
		return true
	}

	// Pattern match for paths with parameters
	if len(name) > 13 && name[:13] == "tasks://list/" {
		return true
	}

	return false
}

// validateToolName checks if a tool name is valid.
func validateToolName(name string) bool {
	validTools := map[string]bool{
		"authenticate":    true,
		"add_task":        true,
		"complete_task":   true,
		"uncomplete_task": true,
		"delete_task":     true,
		"set_due_date":    true,
		"set_priority":    true,
		"add_tags":        true,
		"remove_tags":     true,
		"add_note":        true,
		"auth_status":     true,
		"logout":          true,
	}

	return validTools[name]
}

// extractPathParam extracts a path parameter from a resource name.
// Example: extractPathParam("tasks://list/123", "tasks://list/") returns "123"
func extractPathParam(name, prefix string) string {
	if len(name) <= len(prefix) {
		return ""
	}
	return name[len(prefix):]
}

// formatTaskPriority returns a human-readable priority string.
func formatTaskPriority(priority string) string {
	switch priority {
	case "1":
		return "High"
	case "2":
		return "Medium"
	case "3":
		return "Low"
	default:
		return "None"
	}
}

// coalesceString returns the first non-empty string from the provided arguments.
// Useful for handling optional parameters with defaults.
func coalesceString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// formatMarkdownTable formats data as a markdown table.
func formatMarkdownTable(headers []string, rows [][]string) string {
	if len(headers) == 0 || len(rows) == 0 {
		return ""
	}

	var result strings.Builder

	// Add headers
	result.WriteString("| ")
	result.WriteString(strings.Join(headers, " | "))
	result.WriteString(" |\n")

	// Add separator
	result.WriteString("| ")
	for range headers {
		result.WriteString("--- | ")
	}
	result.WriteString("\n")

	// Add rows
	for _, row := range rows {
		rowData := make([]string, len(headers))
		copy(rowData, row)

		// Ensure we have the right number of columns
		for len(rowData) < len(headers) {
			rowData = append(rowData, "")
		}

		result.WriteString("| ")
		result.WriteString(strings.Join(rowData[:len(headers)], " | "))
		result.WriteString(" |\n")
	}

	return result.String()
}

// validateMimeType checks if a MIME type is in a valid format.
func validateMimeType(mimeType string) bool {
	mimeRegex := regexp.MustCompile(`^[a-z]+/[a-z0-9\-\.\+]*(;\s?[a-z0-9\-\.]+\s*=\s*[a-z0-9\-\.]+)*$`)
	return mimeRegex.MatchString(mimeType)
}

// formatTags formats a list of tags into a string.
func formatTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

	// Sort tags alphabetically for consistent output
	sort.Strings(tags)

	return fmt.Sprintf("[%s]", strings.Join(tags, ", "))
}

// parseTagArgument extracts tags from the tags argument, which can be a string or array.
func parseTagArgument(tagsArg interface{}) ([]string, error) {
	var tags []string

	// Handle different tag formats
	switch t := tagsArg.(type) {
	case []interface{}:
		for _, tagItem := range t {
			if tagStr, ok := tagItem.(string); ok && tagStr != "" {
				tags = append(tags, tagStr)
			}
		}
	case string:
		if t != "" {
			tags = append(tags, t)
		}
	case nil:
		return nil, fmt.Errorf("missing 'tags' argument")
	default:
		return nil, fmt.Errorf("invalid 'tags' argument type")
	}

	if len(tags) == 0 {
		return nil, fmt.Errorf("no valid tags provided")
	}

	return tags, nil
}
