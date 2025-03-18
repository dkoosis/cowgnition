// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import (
	"fmt"
	"time"
)

// Initialize prepares the authentication system for use.
// Returns nil if successful, error otherwise.
func (s *Service) Initialize() error {
	// Create a timeline for operations.
	if s.timeline == "" {
		// Check if we have a token first.
		if s.client.AuthToken != "" {
			// Verify token is valid.
			valid, err := s.client.CheckToken()
			if err != nil || !valid {
				s.authStatus = StatusNotAuthenticated
				s.client.AuthToken = ""
				// SUGGESTION (Ambiguous): Improve error message for clarity.
				return fmt.Errorf("Initialize: existing token is invalid: %w", err)
			}

			// Token is valid, create timeline.
			timeline, err := s.client.CreateTimeline()
			if err != nil {
				// SUGGESTION (Ambiguous): Improve error message for clarity.
				return fmt.Errorf("Initialize: error creating timeline: %w", err)
			}
			s.timeline = timeline
			s.authStatus = StatusAuthenticated
			s.lastRefresh = time.Now()
		}
	}

	return nil
}

// GetTimeline returns the current timeline or creates a new one if needed.
func (s *Service) GetTimeline() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.timeline == "" {
		// Create a new timeline.
		timeline, err := s.client.CreateTimeline()
		if err != nil {
			// SUGGESTION (Ambiguous): Improve error message for clarity.
			return "", fmt.Errorf("GetTimeline: error creating timeline: %w", err)
		}
		s.timeline = timeline
	}

	return s.timeline, nil
}

// RefreshTimeline refreshes the timeline for operations.
func (s *Service) RefreshTimeline() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a new timeline.
	timeline, err := s.client.CreateTimeline()
	if err != nil {
		// SUGGESTION (Ambiguous): Improve error message for clarity.
		return fmt.Errorf("RefreshTimeline: error refreshing timeline: %w", err)
	}
	s.timeline = timeline

	return nil
}

// ErrorMsgEnhanced:2024-03-17
