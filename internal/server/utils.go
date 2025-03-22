// internal/server/utils.go
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ErrorResponse represents a standardized error response according to MCP protocol.
type ErrorResponse struct {
	Error     string `json:"error"`
	Status    int    `json:"status"`
	Path      string `json:"path,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Timestamp string `json:"timestamp"`
}

// writeJSONResponse writes a JSON response with the given status code and data.
// Currently all calls use http.StatusOK, but parameter is kept for API consistency.
// nolint:unparam
func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		http.Error(w, "Internal server error: JSON encoding failed", http.StatusInternalServerError)
	}
}

// writeErrorResponse writes a JSON error response with the given status code and message.
func writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	requestID := generateRequestID()

	errorResponse := ErrorResponse{
		Error:     message,
		Status:    statusCode,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		RequestID: requestID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(errorResponse); err != nil {
		log.Printf("Error encoding error response: %v", err)
		http.Error(w, message, statusCode)
	}
}

// generateRequestID creates a unique ID for tracking request/error correlations.
func generateRequestID() string {
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}

// formatTimeComponent returns formatted time if present, empty string otherwise.
func formatTimeComponent(t time.Time) string {
	if t.Hour() > 0 || t.Minute() > 0 {
		return fmt.Sprintf(" at %s", t.Format("3:04 PM"))
	}
	return ""
}

// formatDate formats an RTM date string for display.
func formatDate(dateStr string) string {
	if dateStr == "" {
		return ""
	}

	// Parse the date string (format: 2006-01-02T15:04:05Z)
	t, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return dateStr // Return original if parsing fails
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

// formatTags formats a list of tags into a string.
func formatTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

	// Sort tags alphabetically for consistent output
	sort.Strings(tags)

	// If only a few tags, comma-separate them
	if len(tags) <= 5 {
		return strings.Join(tags, ", ")
	}

	// For many tags, use bullet points
	var sb strings.Builder
	for _, tag := range tags {
		sb.WriteString("\n- ")
		sb.WriteString(tag)
	}
	return sb.String()
}
