// Package server provides utility functions for handling server responses and formatting data.
// file: internal/server/utils.go
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

// LegacyErrorResponse represents a legacy error response format.
// Deprecated: Use ErrorResponse from errors.go instead for JSON-RPC 2.0 compliance.
// This struct is maintained for backward compatibility during the transition to JSON-RPC 2.0
// error handling.
type LegacyErrorResponse struct {
	Error     string `json:"error"`
	Status    int    `json:"status"`
	Path      string `json:"path,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Timestamp string `json:"timestamp"`
}

// writeJSONResponse writes a JSON response with appropriate headers.
// It always uses HTTP 200 OK status as per MCP protocol for successful responses.
// This function is designed to ensure that successful responses adhere to the MCP protocol
// by consistently setting the HTTP status code to 200 OK.
func writeJSONResponse(w http.ResponseWriter, _ int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // Always use 200 OK for successful responses

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)

		// Use the new error handling system
		errResp := NewErrorResponse(InternalError, "Error encoding JSON response", nil)
		WriteJSONRPCError(w, errResp)
	}
}

// writeErrorResponse writes a JSON error response with the given status code and message.
// Deprecated: Use WriteJSONRPCError from errors.go instead.
// This function is kept for backward compatibility during transition.
// It is recommended to use WriteJSONRPCError for new implementations to ensure
// compliance with JSON-RPC 2.0 standards.
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
// This mapping is crucial for converting HTTP status codes into their
// corresponding JSON-RPC error codes, ensuring proper error representation
// in the API responses.
func mapHTTPToJSONRPCCode(httpStatus int) ErrorCode {
	switch httpStatus {
	case http.StatusBadRequest:
		return InvalidParams // InvalidParams ( -32602 ) : Invalid method parameter(s).
	case http.StatusNotFound:
		return MethodNotFound // MethodNotFound ( -32601 ) : The method does not exist / is not available.
	case http.StatusMethodNotAllowed:
		return MethodNotFound // MethodNotFound ( -32601 ) : The method does not exist / is not available.
	case http.StatusUnauthorized:
		return AuthError // AuthError ( -32600 ) : Authentication failed.
	case http.StatusForbidden:
		return AuthError // AuthError ( -32600 ) : Authentication failed.
	default:
		return InternalError // InternalError ( -32603 ) : Internal JSON-RPC error.
	}
}

// writeStandardErrorResponse is a convenient wrapper for WriteJSONRPCError.
// It creates an ErrorResponse and writes it to the response writer.
// This function simplifies the process of creating and writing standard JSON-RPC
// error responses by encapsulating the error creation and writing logic.
func writeStandardErrorResponse(w http.ResponseWriter, code ErrorCode, message string, data interface{}) {
	// Create a detailed error context for logging
	detailedContext := map[string]interface{}{
		"error_code": code,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}

	// Add the data to context if provided
	if data != nil {
		detailedContext["details"] = data
	}

	// Create detailed error for logging
	detailedErr := &DetailedError{
		OriginalError: fmt.Errorf("%s", message), // Use constant format string
		Context:       detailedContext,
	}
	detailedErr.captureStackTrace(2) // Skip this function and caller

	// Log the detailed error
	LogDetailedError(detailedErr)

	// Create and write a standard JSON-RPC 2.0 error response
	errResp := NewErrorResponse(code, message, data)
	WriteJSONRPCError(w, errResp)
}

// generateRequestID creates a unique ID for tracking request/error correlations.
// This function generates a unique request ID to aid in tracing requests and
// their associated errors, which is essential for debugging and monitoring.
func generateRequestID() string {
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}

// formatTimeComponent returns formatted time if present, empty string otherwise.
// This function formats the time component of a time.Time value if it contains
// hour or minute information; otherwise, it returns an empty string. This is used
// to conditionally include time information in date/time strings.
func formatTimeComponent(t time.Time) string {
	if t.Hour() > 0 || t.Minute() > 0 {
		return fmt.Sprintf(" at %s", t.Format("3:04 PM"))
	}
	return ""
}

// formatDate formats an RTM date string for display.
// This function formats a date string from Remember The Milk (RTM) into a
// human-readable format. It handles cases where the date is today, tomorrow,
// within the next week, within the same year, or a different year. If parsing
// the date string fails, it returns the original string.
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
// This function formats a slice of tags into a comma-separated string or a
// bullet-point list, depending on the number of tags. It sorts the tags
// alphabetically to ensure consistent output.
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

// DocEnhanced (2024-03-22)
