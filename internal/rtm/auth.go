// internal/rtm/auth.go
package rtm

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// Response represents the standard RTM API response wrapper.
type Response struct {
	Stat  string `json:"stat"`
	Error *Error `json:"err,omitempty"`
}

// Error represents an RTM API error.
type Error struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// User represents an RTM user.
type User struct {
	ID       string `json:"id,attr"`
	Username string `json:"username,attr"`
	Fullname string `json:"fullname,attr"`
}

// Auth represents an RTM authentication response.
type Auth struct {
	Token string `json:"token"`
	Perms string `json:"perms"`
	User  User   `json:"user"`
}

// GetFrob gets a frob from RTM for desktop authentication flow.
func (c *Client) GetFrob() (string, error) {
	params := map[string]string{}
	resp, err := c.MakeRequest("rtm.auth.getFrob", params)
	if err != nil {
		return "", fmt.Errorf("failed to get frob: %w", err)
	}

	var response struct {
		Rsp struct {
			Stat  string `json:"stat"`
			Frob  string `json:"frob,omitempty"`
			Error *Error `json:"err,omitempty"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal frob response: %w", err)
	}

	if response.Rsp.Stat != "ok" {
		if response.Rsp.Error != nil {
			return "", fmt.Errorf("RTM API error: %d - %s", response.Rsp.Error.Code, response.Rsp.Error.Msg)
		}
		return "", fmt.Errorf("RTM API returned non-ok status: %s", response.Rsp.Stat)
	}

	return response.Rsp.Frob, nil
}

// GetAuthURL generates an authentication URL for desktop application flow.
func (c *Client) GetAuthURL(frob, perms string) string {
	params := map[string]string{
		"api_key": c.APIKey,
		"perms":   perms,
		"frob":    frob,
	}

	// Sign parameters
	signature := c.Sign(params)

	// Build URL
	values := url.Values{}
	for k, v := range params {
		values.Add(k, v)
	}
	values.Add("api_sig", signature)

	return AuthURL + "?" + values.Encode()
}

// GetToken gets an auth token for the given frob.
func (c *Client) GetToken(frob string) (*Auth, error) {
	params := map[string]string{
		"frob": frob,
	}

	resp, err := c.MakeRequest("rtm.auth.getToken", params)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	var response struct {
		Rsp struct {
			Stat  string `json:"stat"`
			Auth  *Auth  `json:"auth,omitempty"`
			Error *Error `json:"err,omitempty"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token response: %w", err)
	}

	if response.Rsp.Stat != "ok" {
		if response.Rsp.Error != nil {
			return nil, fmt.Errorf("RTM API error: %d - %s", response.Rsp.Error.Code, response.Rsp.Error.Msg)
		}
		return nil, fmt.Errorf("RTM API returned non-ok status: %s", response.Rsp.Stat)
	}

	if response.Rsp.Auth == nil {
		return nil, fmt.Errorf("no auth information in response")
	}

	return response.Rsp.Auth, nil
}

// CheckToken verifies if the auth token is valid.
func (c *Client) CheckToken() (*Auth, error) {
	if c.AuthToken == "" {
		return nil, fmt.Errorf("no auth token set")
	}

	params := map[string]string{}
	resp, err := c.MakeRequest("rtm.auth.checkToken", params)
	if err != nil {
		return nil, fmt.Errorf("failed to check token: %w", err)
	}

	var response struct {
		Rsp struct {
			Stat  string `json:"stat"`
			Auth  *Auth  `json:"auth,omitempty"`
			Error *Error `json:"err,omitempty"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token response: %w", err)
	}

	if response.Rsp.Stat != "ok" {
		if response.Rsp.Error != nil {
			return nil, fmt.Errorf("RTM API error: %d - %s", response.Rsp.Error.Code, response.Rsp.Error.Msg)
		}
		return nil, fmt.Errorf("RTM API returned non-ok status: %s", response.Rsp.Stat)
	}

	if response.Rsp.Auth == nil {
		return nil, fmt.Errorf("no auth information in response")
	}

	return response.Rsp.Auth, nil
}
