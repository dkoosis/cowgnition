// internal/rtm/provider.go
package rtm

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/dkoosis/cowgnition/internal/mcp"
)

const (
	// Resource URIs.
	AuthResourceURI = "auth://rtm"

	// Permissions.
	PermRead   = "read"
	PermWrite  = "write"
	PermDelete = "delete"
)

// AuthProvider implements the MCP ResourceProvider interface for RTM authentication.
type AuthProvider struct {
	client    *Client
	storage   *TokenStorage
	authState map[string]string // Maps frobs to their permission level
	mu        sync.Mutex
}

// NewAuthProvider creates a new RTM auth provider.
func NewAuthProvider(apiKey, sharedSecret, tokenPath string) (*AuthProvider, error) {
	storage, err := NewTokenStorage(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("AuthProvider.NewAuthProvider: failed to create token storage: %w", err)
	}

	client := NewClient(apiKey, sharedSecret)

	return &AuthProvider{
		client:    client,
		storage:   storage,
		authState: make(map[string]string),
	}, nil
}

// GetResourceDefinitions returns the resource definitions provided by this provider.
func (p *AuthProvider) GetResourceDefinitions() []mcp.ResourceDefinition {
	return []mcp.ResourceDefinition{
		{
			Name:        AuthResourceURI,
			Description: "Remember The Milk authentication",
			Arguments: []mcp.ResourceArgument{
				{
					Name:        "frob",
					Description: "RTM frob for authentication flow",
					Required:    false,
				},
				{
					Name:        "perms",
					Description: "RTM permissions (read, write, delete)",
					Required:    false,
				},
			},
		},
	}
}

// ReadResource handles reading RTM authentication resources.
func (p *AuthProvider) ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) {
	if name != AuthResourceURI {
		return "", "", mcp.ErrResourceNotFound
	}

	// Check for existing token first
	tokenResult, err := p.checkExistingToken()
	if err == nil {
		return tokenResult, "application/json", nil
	}

	// Handle authentication with frob if provided
	if frob, ok := args["frob"]; ok && frob != "" {
		return p.handleFrobAuthentication(frob)
	}

	// Start new authentication flow
	return p.startNewAuthFlow(args)
}

// checkExistingToken verifies if we already have a valid token.
func (p *AuthProvider) checkExistingToken() (string, error) {
	// Check if a token is already stored
	token, err := p.storage.LoadToken()
	if err != nil {
		log.Printf("AuthProvider.checkExistingToken: error loading token: %v", err)
		return "", fmt.Errorf("failed to load token: %w", err)
	}

	// If we have a token, verify it's still valid
	if token != "" {
		p.client.SetAuthToken(token)
		auth, err := p.client.CheckToken()
		if err == nil && auth != nil {
			// Token is valid
			response := map[string]interface{}{
				"status":      "authenticated",
				"username":    auth.User.Username,
				"fullname":    auth.User.Fullname,
				"permissions": auth.Perms,
			}
			responseJSON, err := json.MarshalIndent(response, "", "  ")
			if err != nil {
				return "", fmt.Errorf("failed to marshal response: %w", err)
			}
			return string(responseJSON), nil
		}
		// Token is invalid, continue with auth flow
		log.Printf("AuthProvider.checkExistingToken: invalid token, starting new auth flow")
	}

	return "", fmt.Errorf("no valid token found")
}

// handleFrobAuthentication processes authentication with a provided frob.
func (p *AuthProvider) handleFrobAuthentication(frob string) (string, string, error) {
	// Try to get token for frob
	auth, err := p.client.GetToken(frob)
	if err != nil {
		return "", "", fmt.Errorf("AuthProvider.handleFrobAuthentication: failed to get token: %w", err)
	}

	// Store the token
	if err := p.storage.SaveToken(auth.Token); err != nil {
		log.Printf("AuthProvider.handleFrobAuthentication: error saving token: %v", err)
		// Continue even if saving fails
	}

	// Set the token for future requests
	p.client.SetAuthToken(auth.Token)

	// Return successful authentication response
	response := map[string]interface{}{
		"status":      "authenticated",
		"username":    auth.User.Username,
		"fullname":    auth.User.Fullname,
		"permissions": auth.Perms,
	}

	responseJSON, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("AuthProvider.handleFrobAuthentication: failed to marshal response: %w", err)
	}

	return string(responseJSON), "application/json", nil
}

// startNewAuthFlow initiates a new authentication flow.
func (p *AuthProvider) startNewAuthFlow(args map[string]string) (string, string, error) {
	// Get permissions parameter, default to "delete" (highest)
	perms := PermDelete
	if p, ok := args["perms"]; ok && (p == PermRead || p == PermWrite || p == PermDelete) {
		perms = p
	}

	// Start desktop authentication flow
	frob, err := p.client.GetFrob()
	if err != nil {
		return "", "", fmt.Errorf("AuthProvider.startNewAuthFlow: failed to get frob: %w", err)
	}

	// Store frob with requested permissions
	p.mu.Lock()
	p.authState[frob] = perms
	p.mu.Unlock()

	// Generate authentication URL
	authURL := p.client.GetAuthURL(frob, perms)

	// Return response with auth URL and frob
	response := map[string]interface{}{
		"status":       "unauthorized",
		"auth_url":     authURL,
		"frob":         frob,
		"permissions":  perms,
		"instructions": "Visit the auth_url to authorize this application, then access this resource again with the frob parameter.",
	}

	responseJSON, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("AuthProvider.startNewAuthFlow: failed to marshal response: %w", err)
	}

	return string(responseJSON), "application/json", nil
}
