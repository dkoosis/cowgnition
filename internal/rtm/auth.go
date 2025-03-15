// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Status represents the current authentication status.
type Status int

const (
	// StatusUnknown indicates that the authentication status is not determined yet.
	StatusUnknown Status = iota
	// StatusNotAuthenticated indicates that the user is not authenticated.
	StatusNotAuthenticated
	// StatusPending indicates that authentication is in progress.
	StatusPending
	// StatusAuthenticated indicates that the user is authenticated.
	StatusAuthenticated
)

// String returns a string representation of the auth status.
func (s Status) String() string {
	switch s {
	case StatusUnknown:
		return "Unknown"
	case StatusNotAuthenticated:
		return "Not Authenticated"
	case StatusPending:
		return "Authentication Pending"
	case StatusAuthenticated:
		return "Authenticated"
	default:
		return "Invalid Status"
	}
}

// Permission represents the RTM API permission level.
type Permission string

const (
	// PermRead is the read-only permission level.
	PermRead Permission = "read"
	// PermWrite allows reading and writing data.
	PermWrite Permission = "write"
	// PermDelete allows reading, writing, and deleting data.
	PermDelete Permission = "delete"
)

// Flow represents an ongoing authentication flow.
type Flow struct {
	Frob       string
	StartTime  time.Time
	Permission Permission
	AuthURL    string
	ExpiresAt  time.Time
}

// StartAuthFlow initiates the authentication flow with Remember The Milk.
// It returns the authentication URL for the user to visit and the frob for future use.
func (s *Service) StartAuthFlow() (string, string, error) {
	// Get a frob from RTM
	frob, err := s.client.GetFrob()
	if err != nil {
		return "", "", fmt.Errorf("error getting frob: %w", err)
	}

	// Generate auth URL for desktop application
	authURL := s.client.GetAuthURL(frob, string(PermDelete))

	// Create and store auth flow
	flow := &Flow{
		Frob:       frob,
		StartTime:  time.Now(),
		Permission: PermDelete,
		AuthURL:    authURL,
		ExpiresAt:  time.Now().Add(24 * time.Hour), // Frobs expire after 24 hours
	}

	// Store auth flow
	s.mu.Lock()
	s.authFlows[frob] = flow
	s.authStatus = StatusPending
	s.mu.Unlock()

	log.Printf("Started RTM auth flow with frob: %s", frob)
	return authURL, frob, nil
}

// CompleteAuthFlow completes the authentication flow with the provided frob.
// It exchanges the frob for a token and stores it securely.
func (s *Service) CompleteAuthFlow(frob string) error {
	s.mu.RLock()
	flow, exists := s.authFlows[frob]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("invalid frob: %s", frob)
	}

	// Check if flow has expired
	if time.Now().After(flow.ExpiresAt) {
		// Clean up expired flow
		s.mu.Lock()
		delete(s.authFlows, frob)
		s.mu.Unlock()
		return fmt.Errorf("authentication flow expired")
	}

	// Exchange frob for token
	token, err := s.client.GetToken(frob)
	if err != nil {
		s.mu.Lock()
		s.authStatus = StatusNotAuthenticated
		s.mu.Unlock()
		return fmt.Errorf("error getting token: %w", err)
	}

	// Save token securely
	if err := s.tokenManager.SaveToken(token); err != nil {
		return fmt.Errorf("error saving token: %w", err)
	}

	// Set token on client
	s.client.SetAuthToken(token)

	// Update status and clean up flow
	s.mu.Lock()
	s.authStatus = StatusAuthenticated
	delete(s.authFlows, frob)
	s.mu.Unlock()

	log.Printf("Completed RTM authentication flow for frob: %s", frob)
	return nil
}

// CheckAuthStatus checks if the current authentication is valid.
// It returns the current status and an error if the check fails.
func (s *Service) CheckAuthStatus() (Status, error) {
	s.mu.RLock()
	status := s.authStatus
	s.mu.RUnlock()

	// If we're in a definitive state, return it
	if status == StatusPending {
		// Pending means we're waiting for user to authorize
		return status, nil
	}

	// If we have a token, verify it
	token := s.client.GetAuthToken()
	if token == "" {
		// Try to load from storage
		token, err := s.tokenManager.LoadToken()
		if err != nil || token == "" {
			s.mu.Lock()
			s.authStatus = StatusNotAuthenticated
			s.mu.Unlock()
			return StatusNotAuthenticated, nil
		}

		// Set token on client
		s.client.SetAuthToken(token)
	}

	// Verify token with RTM
	valid, err := s.client.CheckToken()
	if err != nil || !valid {
		// Token is invalid, update status and clear token
		s.client.SetAuthToken("")
		if deleteErr := s.tokenManager.DeleteToken(); deleteErr != nil {
			log.Printf("Warning: Failed to delete invalid token: %v", deleteErr)
		}

		s.mu.Lock()
		s.authStatus = StatusNotAuthenticated
		s.mu.Unlock()

		if err != nil {
			return StatusNotAuthenticated, fmt.Errorf("error checking token: %w", err)
		}
		return StatusNotAuthenticated, nil
	}

	// Token is valid
	s.mu.Lock()
	s.authStatus = StatusAuthenticated
	s.mu.Unlock()

	return StatusAuthenticated, nil
}

// IsAuthenticated returns true if the user is authenticated with RTM.
func (s *Service) IsAuthenticated() bool {
	status, err := s.CheckAuthStatus()
	if err != nil {
		log.Printf("Warning: Error checking authentication status: %v", err)
	}
	return status == StatusAuthenticated
}

// ClearAuthentication removes all authentication data.
func (s *Service) ClearAuthentication() error {
	// Remove the token
	if err := s.tokenManager.DeleteToken(); err != nil {
		return fmt.Errorf("error deleting token: %w", err)
	}

	// Clear client auth token
	s.client.SetAuthToken("")

	// Update status and clean up flows
	s.mu.Lock()
	s.authStatus = StatusNotAuthenticated
	s.authFlows = make(map[string]*Flow)
	s.mu.Unlock()

	log.Println("RTM authentication data cleared")
	return nil
}

// RefreshToken attempts to refresh the auth token if it's expiring.
// Currently, RTM tokens don't expire unless revoked by the user or API key changes.
func (s *Service) RefreshToken(_ context.Context) error {
	// RTM tokens don't currently expire, but we can verify them
	status, err := s.CheckAuthStatus()
	if err != nil {
		return fmt.Errorf("error checking token status: %w", err)
	}

	if status != StatusAuthenticated {
		return fmt.Errorf("not authenticated, cannot refresh token")
	}

	// For future API changes where token refresh might be needed
	log.Println("RTM token verified and is valid")
	return nil
}

// CleanupExpiredFlows removes expired authentication flows.
func (s *Service) CleanupExpiredFlows() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for frob, flow := range s.authFlows {
		if now.After(flow.ExpiresAt) {
			log.Printf("Cleaning up expired auth flow for frob: %s", frob)
			delete(s.authFlows, frob)
		}
	}
}
