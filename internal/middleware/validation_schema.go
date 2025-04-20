// Package middleware provides chainable handlers for processing MCP messages, like validation.
package middleware

// file: internal/middleware/validation_schema.go.

import (
	"strings"
)

// determineIncomingSchemaType applies fallback logic to find the schema key for an incoming message type hint.
// The msgTypeHint comes from identifyMessage (e.g., method name, "success_response").
// nolint:gocyclo // Complexity is acceptable due to multiple fallback checks.
func (m *ValidationMiddleware) determineIncomingSchemaType(msgTypeHint string) string {
	// Ensure validator exists and is initialized before accessing HasSchema.
	if m.validator == nil || !m.validator.IsInitialized() {
		m.logger.Error("determineIncomingSchemaType called but validator is nil or not initialized.")
		// Cannot reliably determine schema, return empty to signal failure.
		return ""
	}

	// 1. Try exact match first using the hint.
	if m.validator.HasSchema(msgTypeHint) {
		return msgTypeHint
	}

	// 2. Apply fallback logic based on common patterns derived from the hint.
	var fallbackKeys []string

	switch {
	// --- Notification Fallbacks ---
	// Example: msgTypeHint = "notifications/initialized" or "custom_notification".
	case strings.HasPrefix(msgTypeHint, "notifications/"),
		strings.HasSuffix(msgTypeHint, "_notification"),
		msgTypeHint == "notification": // If hint was already generic.
		// Order matters: more specific fallbacks first.
		if m.validator.HasSchema("JSONRPCNotification") { // Try standard base notification.
			fallbackKeys = append(fallbackKeys, "JSONRPCNotification")
		}
		if m.validator.HasSchema("notification") { // Try simpler generic name.
			fallbackKeys = append(fallbackKeys, "notification")
		}

	// --- Response Fallbacks ---
	// Example: msgTypeHint = "initialize_response", "success_response", "error_response".
	case strings.Contains(msgTypeHint, "Response"),
		strings.Contains(msgTypeHint, "Result"),
		strings.HasSuffix(msgTypeHint, "_response"),
		strings.HasSuffix(msgTypeHint, "_error"):
		// Specific error response structure.
		if m.validator.HasSchema("JSONRPCError") {
			fallbackKeys = append(fallbackKeys, "JSONRPCError")
		}
		// Generic success/error response structure.
		if m.validator.HasSchema("JSONRPCResponse") {
			fallbackKeys = append(fallbackKeys, "JSONRPCResponse")
		}
		// Simpler generic names.
		if m.validator.HasSchema("success_response") {
			fallbackKeys = append(fallbackKeys, "success_response")
		}
		if m.validator.HasSchema("error_response") {
			fallbackKeys = append(fallbackKeys, "error_response")
		}

	// --- Request Fallbacks ---
	// Example: msgTypeHint = "initialize", "tools/list".
	default: // Assume request if none of the above match hints.
		if m.validator.HasSchema("JSONRPCRequest") { // Try standard base request.
			fallbackKeys = append(fallbackKeys, "JSONRPCRequest")
		}
		if m.validator.HasSchema("request") { // Try simpler generic name.
			fallbackKeys = append(fallbackKeys, "request")
		}
	}

	// 3. Check if any chosen fallback keys exist in the validator.
	for _, key := range fallbackKeys {
		// Check HasSchema again in case it was added conditionally above.
		if m.validator.HasSchema(key) {
			m.logger.Debug("Using fallback schema for incoming message.", "messageTypeHint", msgTypeHint, "schemaKeyUsed", key)
			return key
		}
	}

	// 4. If specific type and standard fallbacks failed, try the absolute base schema.
	m.logger.Warn("Specific/generic schema not found for incoming message, trying 'base'.", "messageTypeHint", msgTypeHint, "triedFallbacks", fallbackKeys)
	if m.validator.HasSchema("base") {
		return "base"
	}

	// 5. If even "base" doesn't exist (major initialization issue).
	m.logger.Error("CRITICAL: No schema found for message type or any fallbacks (including base).", "messageTypeHint", msgTypeHint)
	return "" // Return empty string to signal complete failure to find schema.
}

// determineOutgoingSchemaType heuristics to find the schema key for an outgoing response.
// Uses the original request method and potentially the response content.
func (m *ValidationMiddleware) determineOutgoingSchemaType(requestMethod string, responseBytes []byte) string {
	if m.validator == nil || !m.validator.IsInitialized() {
		m.logger.Error("determineOutgoingSchemaType called but validator is nil or not initialized.")
		return "" // Cannot reliably determine schema.
	}

	// 1. Try specific response schema based on request method.
	// Example: requestMethod "tools/list" -> look for "tools/list_response".
	if requestMethod != "" {
		// Construct expected response type. Handle methods like "notifications/initialized".
		// We assume notifications don't typically have validated responses, but check just in case.
		expectedResponseSchema := ""
		// A simple convention: replace slashes with underscores? Or just append _response?
		// Let's try appending _response first.
		expectedResponseSchema = requestMethod + "_response" // e.g., "tools/list_response", "initialize_response".

		if m.validator.HasSchema(expectedResponseSchema) {
			m.logger.Debug("Using specific response schema derived from request.", "requestMethod", requestMethod, "schemaKeyUsed", expectedResponseSchema)
			return expectedResponseSchema
		}
		m.logger.Debug("Specific response schema derived from request method not found, trying fallback.",
			"requestMethod", requestMethod, "derivedSchema", expectedResponseSchema)
	}

	// 2. Identify the response type structurally (success/error).
	// Fixed: Call identifyMessage directly, not as a method on m.
	responseMsgTypeHint, _, identifyErr := identifyMessage(responseBytes)
	if identifyErr == nil && responseMsgTypeHint == "success_response" {
		// Specific heuristic for CallToolResult shape if identified as generic success.
		// Check for CallToolResult schema first.
		if m.validator.HasSchema("CallToolResult") && isCallToolResultShape(responseBytes) { // Helper from validation_helpers.go.
			m.logger.Debug("Identified response as CallToolResult shape, using CallToolResult schema.")
			return "CallToolResult" // Use the specific schema if shape matches.
		}

		// Check if the generic success response schema exists.
		if m.validator.HasSchema("JSONRPCResponse") {
			m.logger.Debug("Using generic JSONRPCResponse schema for success response.", "requestMethod", requestMethod)
			return "JSONRPCResponse"
		}
		if m.validator.HasSchema("success_response") {
			m.logger.Debug("Using generic success_response schema.", "requestMethod", requestMethod)
			return "success_response"
		}
	} else if identifyErr != nil {
		m.logger.Warn("Failed to identify outgoing response type structure for schema determination fallback.", "error", identifyErr)
	}

	// 3. Fallback to base schema if nothing else matches.
	m.logger.Warn("Specific/generic schema not found for outgoing response, trying base schema.",
		"requestMethod", requestMethod, "responsePreview", calculatePreview(responseBytes))
	if m.validator.HasSchema("base") {
		return "base"
	}

	// 4. No schema found.
	return ""
}
