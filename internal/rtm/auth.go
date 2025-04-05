// Package rtm handles Remember The Milk (RTM) authentication.
// file: internal/rtm/auth.go
package rtm

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/cockroachdb/errors"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// Response represents the standard RTM API response wrapper.
// ... (comments remain the same)
type Response struct {
	Stat  string `json:"stat"`
	Error *Error `json:"err,omitempty"`
}

// Error represents an RTM API error.
// ... (comments remain the same)
type Error struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// User represents an RTM user.
// ... (comments remain the same)
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Fullname string `json:"fullname"`
}

// Auth represents an RTM authentication response.
// ... (comments remain the same)
type Auth struct {
	Token string `json:"token"`
	Perms string `json:"perms"`
	User  User   `json:"user"`
}

// GetFrob gets a frob from RTM for desktop authentication flow.
// ... (comments remain the same)
func (c *Client) GetFrob() (string, error) {
	params := map[string]string{}
	resp, err := c.MakeRequest("rtm.auth.getFrob", params)
	if err != nil {
		// Add function context to Wrap message
		return "", errors.Wrap(err, "Client.GetFrob: failed API request")
	}

	var response struct {
		Rsp struct {
			Stat  string `json:"stat"`
			Frob  string `json:"frob,omitempty"`
			Error *Error `json:"err,omitempty"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &response); err != nil {
		// Add function context to cgerr message (corresponds to assessment L61 conceptually)
		return "", cgerr.NewRTMError(
			0,
			"Client.GetFrob: Failed to unmarshal frob response", // Added context
			err,
			map[string]interface{}{
				"response_body_length": len(resp),
			},
		)
	}

	if response.Rsp.Stat != "ok" {
		if response.Rsp.Error != nil {
			// Add function context to RTM error message
			return "", cgerr.NewRTMError(
				response.Rsp.Error.Code,
				fmt.Sprintf("Client.GetFrob: %s", response.Rsp.Error.Msg), // Added context
				nil,
				map[string]interface{}{
					"method": "rtm.auth.getFrob",
				},
			)
		}
		// Add function context to generic non-ok status message
		return "", cgerr.NewRTMError(
			0,
			fmt.Sprintf("Client.GetFrob: RTM API returned non-ok status: %s", response.Rsp.Stat), // Added context
			nil,
			map[string]interface{}{
				"method": "rtm.auth.getFrob",
				"status": response.Rsp.Stat,
			},
		)
	}
	// Add check for missing Frob even on "ok" status
	if response.Rsp.Frob == "" {
		return "", cgerr.NewRTMError(
			0,
			"Client.GetFrob: RTM API status ok but no frob returned", // Added context
			nil,
			map[string]interface{}{
				"method": "rtm.auth.getFrob",
				"status": response.Rsp.Stat,
			},
		)
	}

	return response.Rsp.Frob, nil
}

// GetAuthURL generates an authentication URL for desktop application flow.
// ... (comments remain the same)
func (c *Client) GetAuthURL(frob, perms string) string {
	params := map[string]string{
		"api_key": c.APIKey,
		"perms":   perms,
		"frob":    frob,
	}
	signature := c.Sign(params)
	values := url.Values{}
	for k, v := range params {
		values.Add(k, v)
	}
	values.Add("api_sig", signature)
	return AuthURL + "?" + values.Encode()
}

// GetToken gets an auth token for the given frob.
// ... (comments remain the same)
func (c *Client) GetToken(frob string) (*Auth, error) {
	params := map[string]string{
		"frob": frob,
	}

	resp, err := c.MakeRequest("rtm.auth.getToken", params)
	if err != nil {
		// Add function context to Wrap message (corresponds to assessment L138 conceptually)
		return nil, errors.Wrap(err, "Client.GetToken: failed API request")
	}

	var response struct {
		Rsp struct {
			Stat  string `json:"stat"`
			Auth  *Auth  `json:"auth,omitempty"`
			Error *Error `json:"err,omitempty"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &response); err != nil {
		// Add function context to cgerr message (corresponds to assessment L174 conceptually)
		return nil, cgerr.NewRTMError(
			0,
			"Client.GetToken: Failed to unmarshal token response", // Added context
			err,
			map[string]interface{}{
				"response_body_length": len(resp),
				"frob":                 frob,
			},
		)
	}

	if response.Rsp.Stat != "ok" {
		if response.Rsp.Error != nil {
			// Add function context to RTM error message
			return nil, cgerr.NewRTMError(
				response.Rsp.Error.Code,
				fmt.Sprintf("Client.GetToken: %s", response.Rsp.Error.Msg), // Added context
				nil,
				map[string]interface{}{
					"method": "rtm.auth.getToken",
					"frob":   frob,
				},
			)
		}
		// Add function context to generic non-ok status message
		return nil, cgerr.NewRTMError(
			0,
			fmt.Sprintf("Client.GetToken: RTM API returned non-ok status: %s", response.Rsp.Stat), // Added context
			nil,
			map[string]interface{}{
				"method": "rtm.auth.getToken",
				"status": response.Rsp.Stat,
				"frob":   frob,
			},
		)
	}

	if response.Rsp.Auth == nil {
		// Add function context to missing auth info message
		return nil, cgerr.NewRTMError(
			0,
			"Client.GetToken: No auth information in response", // Added context
			nil,
			map[string]interface{}{
				"method": "rtm.auth.getToken",
				"frob":   frob,
			},
		)
	}

	return response.Rsp.Auth, nil
}

// CheckToken verifies if the auth token is valid.
// ... (comments remain the same)
func (c *Client) CheckToken() (*Auth, error) {
	if c.AuthToken == "" {
		// Add function context to error message
		return nil, cgerr.NewAuthError(
			"Client.CheckToken: No auth token set", // Added context
			nil,
			map[string]interface{}{
				"method": "rtm.auth.checkToken",
			},
		)
	}

	params := map[string]string{}
	resp, err := c.MakeRequest("rtm.auth.checkToken", params)
	if err != nil {
		// Add function context to Wrap message
		return nil, errors.Wrap(err, "Client.CheckToken: failed API request")
	}

	var response struct {
		Rsp struct {
			Stat  string `json:"stat"`
			Auth  *Auth  `json:"auth,omitempty"`
			Error *Error `json:"err,omitempty"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &response); err != nil {
		// Add function context to cgerr message (corresponds to assessment L241 conceptually)
		return nil, cgerr.NewRTMError(
			0,
			"Client.CheckToken: Failed to unmarshal token response", // Added context
			err,
			map[string]interface{}{
				"response_body_length": len(resp),
				"method":               "rtm.auth.checkToken",
			},
		)
	}

	if response.Rsp.Stat != "ok" {
		if response.Rsp.Error != nil {
			// Add function context to RTM error message
			return nil, cgerr.NewRTMError(
				response.Rsp.Error.Code,
				fmt.Sprintf("Client.CheckToken: %s", response.Rsp.Error.Msg), // Added context
				nil,
				map[string]interface{}{
					"method": "rtm.auth.checkToken",
				},
			)
		}
		// Add function context to generic non-ok status message
		return nil, cgerr.NewRTMError(
			0,
			fmt.Sprintf("Client.CheckToken: RTM API returned non-ok status: %s", response.Rsp.Stat), // Added context
			nil,
			map[string]interface{}{
				"method": "rtm.auth.checkToken",
				"status": response.Rsp.Stat,
			},
		)
	}

	if response.Rsp.Auth == nil {
		// Add function context to missing auth info message
		return nil, cgerr.NewRTMError(
			0,
			"Client.CheckToken: No auth information in response", // Added context
			nil,
			map[string]interface{}{
				"method": "rtm.auth.checkToken",
			},
		)
	}

	return response.Rsp.Auth, nil
}
