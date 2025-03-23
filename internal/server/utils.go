// Package server provides utility functions for handling server responses and formatting data.
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

// writeJSONResponse writes a JSON response with appropriate headers.
// It always uses HTTP 200 OK status as per MCP protocol for successful responses.
func writeJSONResponse(w http.ResponseWriter, _ int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // Always use 200 OK for successful responses

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)

		// Use the new error handling system
		detailedErr := withStackTrace(err, map[string]interface{}{
			"response_type": fmt.Sprintf("%T", data),
		})
		LogDetailedError(detailedErr)

		errResp := NewErrorResponse(InternalError, "Error encoding JSON response", nil)
		WriteJSONRPCError(w, errResp)
	}
}

// writeErrorResponse writes a JSON error response with the given status code and message.
// Deprecated: Use WriteJSONRPCError or writeStandardErrorResponse instead.
func writeErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	requestID := generateRequestID()

	// Log the error with the request ID for traceability
	log.Printf("[RequestID: %s] Error (%d): %s", requestID, statusCode, message)

	// Map HTTP status to an appropriate JSON-RPC error code
	errorCode := mapHTTPToJSONRPCCode(statusCode)

	// Create and write the new JSON-RPC 2.0 error response
	errResp := NewErrorResponse(errorCode, message, map[string]string{
		"request_id": requestID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})

	WriteJSONRPCError(w, errResp)
}

// mapHTTPToJSONRPCCode maps HTTP status codes to JSON-RPC error codes.
func mapHTTPToJSONRPCCode(httpStatus int) ErrorCode {
	switch httpStatus {
	case http.StatusBadRequest:
		return InvalidParams
	case http.StatusNotFound:
		return MethodNotFound
	case http.StatusMethodNotAllowed:
		return MethodNotFound
	case http.StatusUnauthorized:
		return AuthError
	case http.StatusForbidden:
		return AuthError
	default:
		return InternalError
	}
}

// writeStandardErrorResponse is a convenient wrapper for WriteJSONRPCError.
func writeStandardErrorResponse(w http.ResponseWriter, code ErrorCode, message string, data interface{}) {
	// Create context for logging
	context := map[string]interface{}{
		"error_code": code,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}

	// Add data to context if provided
	if data != nil {
		context["details"] = data
	}

	// Log detailed error
	detailedErr := withStackTrace(fmt.Errorf("%s", message), context)
	LogDetailedError(detailedErr)

	// Send standard error response
	errResp := NewErrorResponse(code, message, data)
	WriteJSONRPCError(w, errResp)
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
// This function handles cases where the date is today, tomorrow,
// within the next week, within the same year, or a different year.
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
