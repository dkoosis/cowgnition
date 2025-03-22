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

// ErrorResponse represents a standardized error response.
type ErrorResponse struct {
	Error     string `json:"error"`
	Status    int    `json:"status"`
	Path      string `json:"path,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Timestamp string `json:"timestamp"`
}

// writeJSONResponse writes a JSON response with the given status code and data.
// The statusCode parameter allows different success codes to be used (200, 201, etc.)
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

// formatTags formats a list of tags into a string.
func formatTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}

	// Sort tags alphabetically for consistent output
	sort.Strings(tags)

	return fmt.Sprintf("[%s]", strings.Join(tags, ", "))
}
