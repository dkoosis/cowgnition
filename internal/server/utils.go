// Package server implements the Model Context Protocol server for RTM integration.
package server

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// writeJSONResponse writes a JSON response with the given status code and data.
func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// writeErrorResponse writes a JSON error response with the given status code and message.
func writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	response := map[string]interface{}{
		"error": message,
	}
	writeJSONResponse(w, statusCode, response)
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
