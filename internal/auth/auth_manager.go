// Package auth provides authentication handling for the RTM service.
package auth

import (
	"fmt"
	"sync"
	"time"
)

// Status represents the current authentication status.
type Status int

const (
	// StatusNotAuthenticated indicates no authentication has been attempted.
	StatusNotAuthenticated Status = iota

	// StatusPending indicates authentication is in progress.
	StatusPending

	// StatusAuthenticated indicates successful authentication.
	StatusAuthenticated

	// StatusFailed indicates authentication failure.
	StatusFailed
)

// Permission represents the RTM permission level.
type Permission string

const (
	// PermRead provides read-only access.
	PermRead Permission = "read"

	// PermWrite provides read and write access.
	PermWrite Permission = "write"

	// PermDelete provides full access including deletion.
	PermDelete Permission = "delete"
)

// Flow represents an ongoing authentication flow.
type Flow struct {
	Frob        string
	CreatedAt   time.Time
	Permission  Permission
	CallbackURL string
	AuthURL     string
}

// Manager handles the RTM authentication process.
// It manages auth flows, tokens, and permission levels.
type Manager struct {
	tokenPath    string
	permission   Permission
	tokenManager *TokenManager
	authFlows    map[string]*Flow
	mu           sync.RWMutex
	refreshChan  chan struct{}
	status       Status
}

// NewManager creates a new authentication manager.
func NewManager(tokenPath string, permission Permission) (*Manager, error) {
	// Create token manager
	tokenManager, err := NewTokenManager(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("error creating token manager: %w", err)
	}

	// Create auth manager
	manager := &Manager{
		tokenPath:    tokenPath,
		permission:   permission,
		tokenManager: tokenManager,
		authFlows:    make(map[string]*Flow),
		refreshChan:  make(chan struct{}, 1),
		status:       StatusNotAuthenticated,
	}

	return manager, nil
}

// StartAuthFlow begins a new authentication flow with the specified permission level.
// It returns the authentication URL for the user to visit and the frob for future use.
func (am *Manager) StartAuthFlow(generateAuthURL func(frob string, perm string) (string, error)) (string, string, error) {
	// Generate a frob
	frob, err := generateFrob()
	if err != nil {
		return "", "", fmt.Errorf("error generating frob: %w", err)
	}

	// Generate auth URL
	authURL, err := generateAuthURL(frob, string(am.permission))
	if err != nil {
		return "", "", fmt.Errorf("error generating auth URL: %w", err)
	}

	// Create auth flow
	flow := &Flow{
		Frob:       frob,
		CreatedAt:  time.Now(),
		Permission: am.permission,
		AuthURL:    authURL,
	}

	// Store auth flow
	am.mu.Lock()
	am.authFlows[frob] = flow
	am.status = StatusPending
	am.mu.Unlock()

	return authURL, frob, nil
}

// CompleteAuthFlow completes an authentication flow with the provided frob.
// It exchanges the frob for a token and stores it securely.
func (am *Manager) CompleteAuthFlow(frob string, getToken func(frob string) (string, error)) error {
	// Validate frob
	am.mu.RLock()
	flow, exists := am.authFlows[frob]
	am.mu.RUnlock()

	if !exists {
		return fmt.Errorf("invalid frob, not found in active authentication flows")
	}

	// Check for expired flow (24 hours)
	if time.Since(flow.CreatedAt) > 24*time.Hour {
		am.mu.Lock()
		delete(am.authFlows, frob)
		am.mu.Unlock()
		return fmt.Errorf("authentication flow expired, please start a new one")
	}

	// Exchange frob for token
	token, err := getToken(frob)
	if err != nil {
		am.mu.Lock()
		am.status = StatusFailed
		am.mu.Unlock()
		return fmt.Errorf("error getting token: %w", err)
	}

	// Save token
	if saveErr := am.tokenManager.SaveToken(token); saveErr != nil {
		return fmt.Errorf("error saving token: %w", saveErr)
	}

	// Clean up auth flow
	am.mu.Lock()
	delete(am.authFlows, frob)
	am.status = StatusAuthenticated
	am.mu.Unlock()

	return nil
}

// CheckAuthStatus checks the current authentication status.
// It verifies token existence and validity.
func (am *Manager) CheckAuthStatus(verifyToken func(token string) (bool, error)) (Status, error) {
	am.mu.RLock()
	status := am.status
	am.mu.RUnlock()

	// If we already know we're authenticated or failed, return that
	if status == StatusAuthenticated || status == StatusFailed {
		return status, nil
	}

	// If we're not in a pending state, check if we have a token
	if !am.tokenManager.HasToken() {
		am.mu.Lock()
		am.status = StatusNotAuthenticated
		am.mu.Unlock()
		return StatusNotAuthenticated, nil
	}

	// Load the token
	token, err := am.tokenManager.LoadToken()
	if err != nil {
		am.mu.Lock()
		am.status = StatusFailed
		am.mu.Unlock()
		return StatusFailed, fmt.Errorf("error loading token: %w", err)
	}

	// Verify token validity
	valid, err := verifyToken(token)
	if err != nil || !valid {
		// Token is invalid, remove it
		if deleteErr := am.tokenManager.DeleteToken(); deleteErr != nil {
			return StatusFailed, fmt.Errorf("error removing invalid token: %w", deleteErr)
		}

		am.mu.Lock()
		am.status = StatusNotAuthenticated
		am.mu.Unlock()

		if err != nil {
			return StatusFailed, fmt.Errorf("error verifying token: %w", err)
		}
		return StatusNotAuthenticated, nil
	}

	// Token is valid
	am.mu.Lock()
	am.status = StatusAuthenticated
	am.mu.Unlock()

	return StatusAuthenticated, nil
}

// GetStatus returns the current authentication status without verification.
func (am *Manager) GetStatus() Status {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.status
}

// GetToken retrieves the stored authentication token if available.
func (am *Manager) GetToken() (string, error) {
	return am.tokenManager.LoadToken()
}

// ClearAuthentication removes all authentication data.
func (am *Manager) ClearAuthentication() error {
	// Remove token
	if err := am.tokenManager.DeleteToken(); err != nil {
		return fmt.Errorf("error removing token: %w", err)
	}

	// Reset status
	am.mu.Lock()
	am.status = StatusNotAuthenticated
	am.authFlows = make(map[string]*Flow)
	am.mu.Unlock()

	return nil
}

// HasPendingFlow checks if there are any pending authentication flows.
func (am *Manager) HasPendingFlow() bool {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return len(am.authFlows) > 0
}

// CleanExpiredFlows removes expired authentication flows.
func (am *Manager) CleanExpiredFlows() {
	am.mu.Lock()
	defer am.mu.Unlock()

	for frob, flow := range am.authFlows {
		if time.Since(flow.CreatedAt) > 24*time.Hour {
			delete(am.authFlows, frob)
		}
	}
}

// generateFrob creates a unique frob for authentication.
// In a real implementation, this would be provided by the RTM API.
// nolint:unparam
func generateFrob() (string, error) {
	// This is a placeholder - in the real implementation,
	// we'd call RTM API to get a frob rather than generating one
	return fmt.Sprintf("frob-%d", time.Now().UnixNano()), nil
}
