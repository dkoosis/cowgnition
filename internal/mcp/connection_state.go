// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/connection_state.go

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Added import
)

// ConnectionState tracks the protocol state of an MCP connection.
// It provides simple state validation to ensure MCP protocol sequence
// requirements are followed.
type ConnectionState struct {
	// initialized indicates whether the MCP initialize method has been called.
	initialized bool

	// currentState represents the current named protocol state.
	currentState string

	// --- FIX: Add fields to store client info ---
	clientInfo         *mcptypes.Implementation
	clientCapabilities *mcptypes.ClientCapabilities
	// --- END FIX ---

	// allowedMethods contains methods that are valid in the current state.
	allowedMethods map[string]bool

	// mu protects concurrent access to state fields.
	mu sync.RWMutex
}

// State constants define the possible connection states.
const (
	// StateUninitialized is the initial state before initialize is called.
	StateUninitialized = "uninitialized"

	// StateReady is the state after successful initialization.
	StateReady = "ready"

	// StateProcessingRequest is the state during request handling.
	StateProcessingRequest = "processing_request"
)

// NewConnectionState creates a new connection state object.
// The initial state is uninitialized, which only allows the initialize method.
func NewConnectionState() *ConnectionState {
	return &ConnectionState{
		initialized:  false,
		currentState: StateUninitialized,
		allowedMethods: map[string]bool{
			"initialize": true,
			// Notifications are always allowed
			"notifications/initialized": true,
			"notifications/cancelled":   true,
			"notifications/progress":    true,
			"ping":                      true, // Ping is always allowed for heartbeat
		},
	}
}

// IsInitialized returns whether the connection has been initialized.
func (s *ConnectionState) IsInitialized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initialized
}

// CurrentState returns the current state name.
func (s *ConnectionState) CurrentState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentState
}

// IsMethodAllowed checks if a method is allowed in the current state.
func (s *ConnectionState) IsMethodAllowed(method string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if it's an allowed method
	return s.allowedMethods[method]
}

// SetInitialized marks the connection as initialized and updates allowed methods.
func (s *ConnectionState) SetInitialized() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.initialized = true
	s.currentState = StateReady

	// After initialization, most methods are allowed
	s.allowedMethods = map[string]bool{
		// Core methods
		"ping":     true,
		"shutdown": true, // Allow shutdown after init
		"exit":     true, // Allow exit after init

		// Tool methods
		"tools/list": true,
		"tools/call": true,

		// Resource methods
		"resources/list":        true,
		"resources/read":        true,
		"resources/subscribe":   true,
		"resources/unsubscribe": true,

		// Prompt methods
		"prompts/list": true,
		"prompts/get":  true,

		// Completion methods
		"completion/complete": true,

		// Logging methods
		"logging/setLevel": true,

		// Notifications (always allowed)
		"notifications/initialized":            true,
		"notifications/cancelled":              true,
		"notifications/progress":               true,
		"notifications/resources/list_changed": true,
		"notifications/resources/updated":      true,
		"notifications/prompts/list_changed":   true,
		"notifications/tools/list_changed":     true,
		"notifications/roots/list_changed":     true,
		"notifications/message":                true,
	}

	// Initialize is no longer allowed after successful initialization
	delete(s.allowedMethods, "initialize")
}

// SetUninitialized resets the connection state to uninitialized.
// Used primarily for the shutdown process or resetting.
func (s *ConnectionState) SetUninitialized() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.initialized = false
	s.currentState = StateUninitialized
	s.allowedMethods = map[string]bool{
		"initialize":                true,
		"notifications/initialized": true,
		"notifications/cancelled":   true,
		"notifications/progress":    true,
		"ping":                      true,
	}
	// Clear client info on reset
	s.clientInfo = nil
	s.clientCapabilities = nil
}

// ValidateMethodSequence validates if a method is allowed in the current state.
// Returns an error if the method is not allowed, with detailed context about why and what to do next.
func (s *ConnectionState) ValidateMethodSequence(method string) error {
	if !s.IsMethodAllowed(method) {
		s.mu.RLock()
		currentState := s.currentState
		// Create a sorted list of currently allowed methods for better error messages
		allowedMethodsList := make([]string, 0, len(s.allowedMethods))
		for m := range s.allowedMethods {
			allowedMethodsList = append(allowedMethodsList, m)
		}
		sort.Strings(allowedMethodsList) // Sort for consistent, readable error messages
		s.mu.RUnlock()

		if method == "initialize" && s.IsInitialized() {
			return fmt.Errorf("protocol sequence error: Method '%s' can only be called once. The connection is already in '%s' state. Use methods like 'tools/list' or 'resources/list' to interact with the initialized connection",
				method, currentState)
		}

		if !s.IsInitialized() && method != "initialize" && method != "ping" &&
			!isNotification(method) {
			return fmt.Errorf("protocol sequence error: Method '%s' cannot be called in '%s' state. You must first call 'initialize' to establish the connection. Allowed methods in current state: %s",
				method, currentState, formatMethodList(allowedMethodsList))
		}

		return fmt.Errorf("protocol sequence error: Method '%s' is not allowed in current state '%s'. Allowed methods: %s",
			method, currentState, formatMethodList(allowedMethodsList))
	}

	return nil
}

// formatMethodList creates a nicely formatted string of method names.
func formatMethodList(methods []string) string {
	if len(methods) == 0 {
		return "[]"
	}
	return "['" + strings.Join(methods, "', '") + "']"
}

// isNotification checks if a method is an MCP notification.
func isNotification(method string) bool {
	return len(method) >= 13 && method[:13] == "notifications/"
}

// --- FIX: Add missing Set* methods ---

// SetClientInfo stores the client's implementation details.
func (s *ConnectionState) SetClientInfo(info mcptypes.Implementation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clientInfo = &info
}

// SetClientCapabilities stores the client's declared capabilities.
func (s *ConnectionState) SetClientCapabilities(caps mcptypes.ClientCapabilities) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clientCapabilities = &caps
}

// SetShutdown marks the connection as shutting down (though doesn't change allowed methods currently).
func (s *ConnectionState) SetShutdown() {
	// Currently, just marks initialized=false to block further calls, could refine state later.
	s.SetUninitialized() // Re-use existing method to block further non-init calls
}

// --- END FIX ---
