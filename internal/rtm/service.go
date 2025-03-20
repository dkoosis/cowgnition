// Package rtm provides integration with the Remember The Milk API.
package rtm

import (
	"encoding/xml"
	"fmt"
	"sync"
	"time"
)

// List represents an RTM list.
type List struct {
	ID       string
	Name     string
	Deleted  bool
	Locked   bool
	Archived bool
	Position int
	Smart    bool
	Filter   string
}

// Service provides a wrapper around the RTM client with additional functionality.
type Service struct {
	client       *Client
	authStatus   Status
	authFlows    map[string]*AuthFlow // Changed from Flow to AuthFlow
	lastRefresh  time.Time
	timeline     string
	mu           sync.Mutex
	permission   string
	tokenRefresh int
}

// NewService creates a new RTM service with the provided client.
func NewService(apiKey, sharedSecret, permission string, tokenRefresh int) *Service {
	return &Service{
		client:       NewClient(apiKey, sharedSecret),
		authStatus:   StatusUnknown,
		authFlows:    make(map[string]*AuthFlow), // Changed from Flow to AuthFlow
		permission:   permission,
		tokenRefresh: tokenRefresh,
	}
}

// GetAuthStatus returns the current authentication status.
func (s *Service) GetAuthStatus() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authStatus
}

// SetAuthStatus sets the authentication status.
func (s *Service) SetAuthStatus(status Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authStatus = status
}

// IsAuthenticated checks if the user is authenticated.
func (s *Service) IsAuthenticated() bool {
	return s.GetAuthStatus() == StatusAuthenticated
}

// StartAuthFlow starts the authentication flow and returns the auth URL and frob.
func (s *Service) StartAuthFlow() (authURL, frob string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get a frob from the RTM API.
	frob, err = s.client.GetFrob()
	if err != nil {
		// SUGGESTION (Ambiguous): Improve error message for clarity.
		return "", "", fmt.Errorf("StartAuthFlow: error getting frob: %w", err)
	}

	// Generate authentication URL.
	authURL = s.client.GetAuthURL(frob, s.permission)

	// Store the authentication flow.
	s.authFlows[frob] = &AuthFlow{ // Changed from Flow to AuthFlow
		Frob:       frob,
		AuthURL:    authURL,
		StartTime:  time.Now(),
		Permission: Permission(s.permission),
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}

	// Update status.
	s.authStatus = StatusAuthenticating

	return authURL, frob, nil
}

// CompleteAuthFlow completes the authentication flow with the provided frob.
func (s *Service) CompleteAuthFlow(frob string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if frob exists.
	flow, exists := s.authFlows[frob]
	if !exists {
		// SUGGESTION (Ambiguous): Improve error message for clarity.
		return fmt.Errorf("CompleteAuthFlow: invalid frob or auth flow expired")
	}

	// Exchange frob for token.
	token, err := s.client.GetToken(flow.Frob)
	if err != nil {
		// SUGGESTION (Ambiguous): Improve error message for clarity.
		return fmt.Errorf("CompleteAuthFlow: error getting token: %w", err)
	}

	// Set token on client.
	s.client.SetAuthToken(token)

	// Update status.
	s.authStatus = StatusAuthenticated
	s.lastRefresh = time.Now()

	// Clean up auth flow.
	delete(s.authFlows, frob)

	// Create timeline.
	timeline, err := s.client.CreateTimeline()
	if err != nil {
		// SUGGESTION (Ambiguous): Improve error message for clarity.
		return fmt.Errorf("CompleteAuthFlow: error creating timeline: %w", err)
	}
	s.timeline = timeline

	return nil
}

// CleanupExpiredFlows removes expired authentication flows.
func (s *Service) CleanupExpiredFlows() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Authentication flows expire after 1 hour.
	expiry := time.Hour

	for frob, flow := range s.authFlows {
		if time.Since(flow.StartTime) > expiry {
			delete(s.authFlows, frob)
		}
	}
}

// GetAllLists returns all RTM lists.
func (s *Service) GetAllLists() ([]List, error) { // Changed return type to make it clearer
	// Check authentication.
	if s.GetAuthStatus() != StatusAuthenticated {
		// SUGGESTION (Readability): Added "GetAllLists:" prefix for context.
		return nil, fmt.Errorf("GetAllLists: not authenticated")
	}

	// Call the RTM API.
	resp, err := s.client.GetLists()
	if err != nil {
		// SUGGESTION (Ambiguous): Improve error message for clarity.
		return nil, fmt.Errorf("GetAllLists: error getting lists: %w", err)
	}

	// Parse response.
	var result struct {
		XMLName xml.Name `xml:"rsp"`
		Lists   struct {
			List []List `xml:"list"` // Fixed the XML structure
		} `xml:"lists"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		// SUGGESTION (Ambiguous): Improve error message for clarity.
		return nil, fmt.Errorf("GetAllLists: error parsing lists response: %w", err)
	}

	return result.Lists.List, nil
}

// GetActiveAuthFlows returns the number of active authentication flows.
func (s *Service) GetActiveAuthFlows() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.authFlows)
}

// ClearAuthentication clears the authentication state.
func (s *Service) ClearAuthentication() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.authStatus = StatusNotAuthenticated
	s.client.SetAuthToken("")
	s.timeline = ""
	s.lastRefresh = time.Time{}
	s.authFlows = make(map[string]*AuthFlow) // Changed from Flow to AuthFlow

	return nil
}

// formatTaskPriority returns a human-readable priority string.
func (s *Service) formatTaskPriority(priority string) string {
	switch priority {
	case "1":
		return "High"
	case "2":
		return "Medium"
	case "3":
		return "Low"
	default:
		return "None"
	}
}
