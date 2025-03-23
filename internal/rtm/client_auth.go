// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import (
	"context"
	"crypto/md5" // #nosec G501 - Required by RTM API for signature generation
	"encoding/xml"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// generateSignature creates an API signature for the given parameters.
// RTM API specifically requires MD5 for signature generation per their authentication docs.
func (c *Client) generateSignature(params url.Values) string {
	// Extract keys and sort them
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Create signature string
	var sb strings.Builder
	sb.WriteString(c.SharedSecret)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params.Get(k))
	}

	// Calculate MD5 hash - required by RTM API
	hash := md5.Sum([]byte(sb.String())) // #nosec G401 - Required by RTM API specs
	return fmt.Sprintf("%x", hash)
}

// GetAuthURL generates an authentication URL for the given frob and permission level.
// The user must visit this URL to authorize the application.
func (c *Client) GetAuthURL(frob, perms string) string {
	params := url.Values{}
	params.Set("api_key", c.APIKey)
	params.Set("perms", perms)
	params.Set("frob", frob)

	apiSig := c.generateSignature(params)
	params.Set("api_sig", apiSig)

	return fmt.Sprintf("https://www.rememberthemilk.com/services/auth/?%s", params.Encode())
}

// GetFrob gets a frob from the RTM API for authentication.
// A frob is a temporary identifier used in the authentication process.
func (c *Client) GetFrob() (string, error) {
	ctx := context.Background()
	resp, err := c.callMethod(ctx, "rtm.auth.getFrob", nil)
	if err != nil {
		return "", err
	}

	var result struct {
		Frob string `xml:"frob"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("GetFrob: failed to parse frob response: %w", err)
	}

	return result.Frob, nil
}

// GetToken exchanges a frob for an authentication token.
// This should be called after the user has authorized the application.
func (c *Client) GetToken(frob string) (string, error) {
	ctx := context.Background()
	params := url.Values{}
	params.Set("frob", frob)

	resp, err := c.callMethod(ctx, "rtm.auth.getToken", params)
	if err != nil {
		return "", err
	}

	var result struct {
		Auth struct {
			Token string `xml:"token"`
		} `xml:"auth"`
	}

	if err := xml.Unmarshal(resp, &result); err != nil {
		return "", fmt.Errorf("GetToken: failed to parse token response: %w", err)
	}

	// Set the token on the client
	c.AuthToken = result.Auth.Token
	return result.Auth.Token, nil
}

// CheckToken checks if the current authentication token is valid.
// Returns true if the token is valid, false otherwise.
func (c *Client) CheckToken() (bool, error) {
	if c.AuthToken == "" {
		return false, nil
	}

	ctx := context.Background()
	_, err := c.callMethod(ctx, "rtm.auth.checkToken", nil)
	if err != nil {
		if strings.Contains(err.Error(), "Invalid auth token") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}
