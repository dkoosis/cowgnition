// internal/rtm/auth.go

package rtm

import (
	"context"
	"fmt"
	"log"
	"time"
)

// AuthStatus represents the current authentication status.
type AuthStatus int

const (
	// AuthStatusUnknown indicates that the authentication status is not determined yet.
	AuthStatusUnknown AuthStatus = iota
	// AuthStatusNotAuthenticated indicates that the user is not authenticated.
	AuthStatusNotAuthenticated
	// AuthStatusPending indicates that authentication is in progress.
	AuthStatusPending
	// AuthStatusAuthenticated indicates that the user is authenticated.
	AuthStatusAuthenticated
)

// String returns a string representation of the auth status.
func (s AuthStatus) String() string {
	switch s {
	case AuthStatusUnknown:
		return "Unknown"
	case AuthStatusNotAuthenticated:
		return "Not Authenticated"
	case AuthStatusPending:
		return "Authentication Pending"
	case AuthStatusAuthenticated:
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

// AuthFlow represents an ongoing authentication flow.
type AuthFlow struct {
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
	flow := &AuthFlow{
		Frob:       frob,
		StartTime:  time.Now(),
		Permission: PermDelete,
		AuthURL:    authURL,
		ExpiresAt:  time.Now().Add(24 * time.Hour), // Frobs expire after 24 hours
	}

	// Store auth flow
	s.mu.Lock()
	s.authFlows[frob] = flow
	s.authStatus = AuthStatusPending
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
		s.authStatus = AuthStatusNotAuthenticated
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
	s.authStatus = AuthStatusAuthenticated
	delete(s.authFlows, frob)
	s.mu.Unlock()

	log.Printf("Completed RTM authentication flow for frob: %s", frob)
	return nil
}

// CheckAuthStatus checks if the current authentication is valid.
// It returns the current status and an error if the check fails.
func (s *Service) CheckAuthStatus() (AuthStatus, error) {
	s.mu.RLock()
	status := s.authStatus
	s.mu.RUnlock()

	// If we're in a definitive state, return it
	if status == AuthStatusPending {
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
			s.authStatus = AuthStatusNotAuthenticated
			s.mu.Unlock()
			return AuthStatusNotAuthenticated, nil
		}

		// Set token on client
		s.client.SetAuthToken(token)
	}

	// Verify token with RTM
	valid, err := s.client.CheckToken()
	if err != nil || !valid {
		// Token is invalid, update status and clear token
		s.client.SetAuthToken("")
		if err := s.tokenManager.DeleteToken(); err != nil {
			log.Printf("Warning: Failed to delete invalid token: %v", err)
		}

		s.mu.Lock()
		s.authStatus = AuthStatusNotAuthenticated
		s.mu.Unlock()

		if err != nil {
			return AuthStatusNotAuthenticated, fmt.Errorf("error checking token: %w", err)
		}
		return AuthStatusNotAuthenticated, nil
	}

	// Token is valid
	s.mu.Lock()
	s.authStatus = AuthStatusAuthenticated
	s.mu.Unlock()

	return AuthStatusAuthenticated, nil
}

// IsAuthenticated returns true if the user is authenticated with RTM.
func (s *Service) IsAuthenticated() bool {
	status, _ := s.CheckAuthStatus()
	return status == AuthStatusAuthenticated
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
	s.authStatus = AuthStatusNotAuthenticated
	s.authFlows = make(map[string]*AuthFlow)
	s.mu.Unlock()

	log.Println("RTM authentication data cleared")
	return nil
}

// RefreshToken attempts to refresh the auth token if it's expiring.
// Currently, RTM tokens don't expire unless revoked by the user or API key changes.
func (s *Service) RefreshToken(ctx context.Context) error {
	// RTM tokens don't currently expire, but we can verify them
	status, err := s.CheckAuthStatus()
	if err != nil {
		return fmt.Errorf("error checking token status: %w", err)
	}

	if status != AuthStatusAuthenticated {
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
