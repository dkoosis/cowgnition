// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"encoding/json"
	"log"
	"net/http"
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
func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) { // nolint:unparam
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
//
// TODO: Determine if validateResourceName is still needed. If not, remove it.
//
//lint:ignore U1000 This function is temporarily unused.
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
//
// TODO: Determine if validateToolName is still needed. If not, remove it.
//
//lint:ignore U1000 This function is temporarily unused.
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
	}

	return validTools[name]
}

// extractPathParam extracts a path parameter from a resource name.
// Example: extractPathParam("tasks://list/123", "tasks://list/") returns "123"
//
// TODO: Determine if extractPathParam is still needed. If not, remove it.
//
//lint:ignore U1000 This function is temporarily unused.
func extractPathParam(name, prefix string) string {
	if len(name) <= len(prefix) {
		return ""
	}
	return name[len(prefix):]
}

// formatTaskPriority returns a human-readable priority string.
//
// TODO: Determine if formatTaskPriority is still needed. If not, remove it.
//
//lint:ignore U1000 This function is temporarily unused.
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
//
// TODO: Determine if coalesceString is still needed. If not, remove it.
//
//lint:ignore U1000 This function is temporarily unused.
func coalesceString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// formatMarkdownTable formats data as a markdown table.
//
// TODO: Determine if formatMarkdownTable is still needed. If not, remove it.
//
//lint:ignore U1000 This function is temporarily unused.
func formatMarkdownTable(headers []string, rows [][]string) string {
	if len(headers) == 0 || len(rows) == 0 {
		return ""
	}

	var result string

	// Add headers
	result += "| "
	for _, header := range headers {
		result += header + " | "
	}
	result += "\n"

	// Add separator
	result += "| "
	for range headers {
		result += "--- | "
	}
	result += "\n"

	// Add rows
	for _, row := range rows {
		result += "| "
		for i, cell := range row {
			if i < len(headers) {
				result += cell + " | "
			}
		}
		result += "\n"
	}

	return result
}
