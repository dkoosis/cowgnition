// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/auth.go

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/cockroachdb/errors"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
)

// GetAuthState checks the current token's validity and returns the auth state.
func (c *Client) GetAuthState(ctx context.Context) (*AuthState, error) {
	if c.config.AuthToken == "" {
		return &AuthState{IsAuthenticated: false}, nil
	}

	params := map[string]string{}
	respBytes, err := c.callMethod(ctx, methodCheckToken, params)

	// Check for specific auth token error (98) identified by callMethod
	var authErr *mcperrors.AuthError
	if err != nil && errors.As(err, &authErr) && authErr.Code == mcperrors.ErrAuthFailure {
		// This mapping happens if callMethod detected RTM code 98
		c.logger.Info("Auth token is invalid according to RTM API (Code 98).")
		return &AuthState{IsAuthenticated: false}, nil
	} else if err != nil {
		// Handle other errors from callMethod
		c.logger.Warn("Failed to check auth token validity, assuming invalid.", "error", err)
		// Optionally, try to inspect the error further if needed
		return &AuthState{IsAuthenticated: false}, nil
	}

	// Token is valid, parse user info
	var result checkTokenRsp
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, errors.Wrap(err, "failed to parse checkToken response")
	}

	// Defensive check in case RTM response structure changes unexpectedly
	if result.Rsp.Auth.User.Username == "" {
		return nil, errors.New("checkToken response missing user information despite ok status")
	}

	return &AuthState{
		IsAuthenticated: true,
		Username:        result.Rsp.Auth.User.Username,
		FullName:        result.Rsp.Auth.User.Fullname,
		UserID:          result.Rsp.Auth.User.ID,
	}, nil
}

// StartAuthFlow begins the RTM auth flow by getting a frob and generating the auth URL.
func (c *Client) StartAuthFlow(ctx context.Context) (string, string, error) { // Returns authURL, frob, error
	params := map[string]string{}
	respBytes, err := c.callMethod(ctx, methodGetFrob, params)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get authentication frob") // Already wrapped by callMethod
	}

	var result frobRsp
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", "", errors.Wrap(err, "failed to parse frob response")
	}

	frob := result.Rsp.Frob
	if frob == "" {
		return "", "", mcperrors.NewRTMError(mcperrors.ErrRTMInvalidResponse, "empty frob received from API", nil, nil)
	}

	c.logger.Info("Got authentication frob.") // Don't log the frob itself

	// Generate the authentication URL including the signature
	authParams := map[string]string{
		"api_key": c.config.APIKey,
		"perms":   permDelete,
		"frob":    frob,
	}
	sig := c.generateSignature(authParams) // Signature is calculated BEFORE adding api_sig param

	authURL, err := url.Parse(authEndpoint)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to parse auth endpoint URL")
	}

	q := authURL.Query()
	q.Set("api_key", c.config.APIKey)
	q.Set("perms", permDelete)
	q.Set("frob", frob)
	q.Set("api_sig", sig) // Add the signature
	authURL.RawQuery = q.Encode()

	// Return both URL and frob separately
	return authURL.String(), frob, nil
}

// CompleteAuthFlow exchanges the frob for a permanent auth token.
func (c *Client) CompleteAuthFlow(ctx context.Context, frob string) (string, error) { // Returns token, error
	if frob == "" {
		return "", mcperrors.NewRTMError(mcperrors.ErrAuthMissing, "frob is required to complete auth flow", nil, nil)
	}

	params := map[string]string{"frob": frob}
	respBytes, err := c.callMethod(ctx, methodGetToken, params)
	if err != nil {
		return "", errors.Wrap(err, "failed to get auth token") // Already wrapped by callMethod
	}

	var result tokenRsp
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", errors.Wrap(err, "failed to parse token response")
	}

	token := result.Rsp.Auth.Token
	if token == "" {
		return "", mcperrors.NewRTMError(mcperrors.ErrRTMInvalidResponse, "empty token received from API", nil, nil)
	}

	// Store the token in the client's config immediately
	c.config.AuthToken = token

	c.logger.Info("Successfully authenticated with RTM.",
		"userId", result.Rsp.Auth.User.ID,
		"username", result.Rsp.Auth.User.Username)

	return token, nil // Return the obtained token
}
