// file: internal/rtm/auth.go
// Package rtm handles Remember The Milk (RTM) authentication.
// This file contains RTM API method implementations related to authentication.
// Terminate all comments with a period.
package rtm

import (
	"context" // Import context package.
	"encoding/json"
	"fmt"

	// "net/url" // No longer needed here if GetAuthURL is removed.

	"github.com/cockroachdb/errors"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// Response represents the standard RTM API response wrapper.
// It contains the status and an optional error structure.
type Response struct {
	Stat  string `json:"stat"`
	Error *Error `json:"err,omitempty"`
}

// Error represents an RTM API error structure.
// Contains the numeric code and message returned by the API.
type Error struct {
	Code int    `json:"code"` // RTM returns codes as numbers in JSON, despite docs sometimes showing strings. Ensure parsing handles this.
	Msg  string `json:"msg"`
}

// User represents an RTM user structure.
// Contains ID, username, and full name.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Fullname string `json:"fullname"`
}

// Auth represents the content of a successful RTM authentication response.
// Includes the auth token, granted permissions, and user details.
type Auth struct {
	Token string `json:"token"`
	Perms string `json:"perms"`
	User  User   `json:"user"`
}

// GetFrob gets a frob from RTM for desktop authentication flow.
// A frob is a temporary identifier used during the web auth process.
func (c *Client) GetFrob() (string, error) {
	params := map[string]string{}
	// Corrected: Pass context to MakeRequest. Using Background() as no specific context is available here.
	resp, err := c.MakeRequest(context.Background(), "rtm.auth.getFrob", params)
	if err != nil {
		// Add function context to Wrap message.
		return "", errors.Wrap(err, "Client.GetFrob: failed API request.")
	}

	var response struct {
		Rsp struct {
			Stat  string `json:"stat"`
			Frob  string `json:"frob,omitempty"`
			Error *Error `json:"err,omitempty"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &response); err != nil {
		// Add function context to cgerr message.
		return "", cgerr.NewRTMError(
			0,
			"Client.GetFrob: Failed to unmarshal frob response.", // Added context.
			err,
			map[string]interface{}{
				"response_body_length": len(resp),
			},
		)
	}

	if response.Rsp.Stat != "ok" {
		if response.Rsp.Error != nil {
			// Add function context to RTM error message.
			return "", cgerr.NewRTMError(
				response.Rsp.Error.Code,
				fmt.Sprintf("Client.GetFrob: %s.", response.Rsp.Error.Msg), // Added context and period.
				nil,
				map[string]interface{}{
					"method": "rtm.auth.getFrob",
				},
			)
		}
		// Add function context to generic non-ok status message.
		return "", cgerr.NewRTMError(
			0,
			fmt.Sprintf("Client.GetFrob: RTM API returned non-ok status: %s.", response.Rsp.Stat), // Added context.
			nil,
			map[string]interface{}{
				"method": "rtm.auth.getFrob",
				"status": response.Rsp.Stat,
			},
		)
	}
	// Add check for missing Frob even on "ok" status.
	if response.Rsp.Frob == "" {
		return "", cgerr.NewRTMError(
			0,
			"Client.GetFrob: RTM API status ok but no frob returned.", // Added context.
			nil,
			map[string]interface{}{
				"method": "rtm.auth.getFrob",
				"status": response.Rsp.Stat,
			},
		)
	}

	return response.Rsp.Frob, nil
}

// GetAuthURL method removed from this file to resolve the "already declared" error.
// The canonical version should exist in internal/rtm/client.go.

// GetToken gets an auth token for the given frob.
// This exchanges the temporary frob (obtained after user web authorization) for a permanent token.
func (c *Client) GetToken(frob string) (*Auth, error) {
	params := map[string]string{
		"frob": frob,
	}

	// Corrected: Pass context to MakeRequest. Using Background().
	resp, err := c.MakeRequest(context.Background(), "rtm.auth.getToken", params)
	if err != nil {
		// Add function context to Wrap message.
		return nil, errors.Wrap(err, "Client.GetToken: failed API request.")
	}

	var response struct {
		Rsp struct {
			Stat  string `json:"stat"`
			Auth  *Auth  `json:"auth,omitempty"` // Pointer for optionality checking.
			Error *Error `json:"err,omitempty"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &response); err != nil {
		// Add function context to cgerr message.
		return nil, cgerr.NewRTMError(
			0,
			"Client.GetToken: Failed to unmarshal token response.", // Added context.
			err,
			map[string]interface{}{
				"response_body_length": len(resp),
				"frob":                 frob,
			},
		)
	}

	if response.Rsp.Stat != "ok" {
		if response.Rsp.Error != nil {
			// Add function context to RTM error message.
			return nil, cgerr.NewRTMError(
				response.Rsp.Error.Code,
				fmt.Sprintf("Client.GetToken: %s.", response.Rsp.Error.Msg), // Added context and period.
				nil,
				map[string]interface{}{
					"method": "rtm.auth.getToken",
					"frob":   frob,
				},
			)
		}
		// Add function context to generic non-ok status message.
		return nil, cgerr.NewRTMError(
			0,
			fmt.Sprintf("Client.GetToken: RTM API returned non-ok status: %s.", response.Rsp.Stat), // Added context.
			nil,
			map[string]interface{}{
				"method": "rtm.auth.getToken",
				"status": response.Rsp.Stat,
				"frob":   frob,
			},
		)
	}

	// Ensure the nested Auth object is present on success.
	if response.Rsp.Auth == nil {
		// Add function context to missing auth info message.
		return nil, cgerr.NewRTMError(
			0,
			"Client.GetToken: No auth information in response.", // Added context.
			nil,
			map[string]interface{}{
				"method": "rtm.auth.getToken",
				"frob":   frob,
			},
		)
	}

	return response.Rsp.Auth, nil
}

// CheckToken verifies if the client's currently set auth token is valid.
func (c *Client) CheckToken() (*Auth, error) {
	if c.AuthToken == "" {
		// Add function context to error message.
		return nil, cgerr.NewAuthError(
			"Client.CheckToken: No auth token set.", // Added context.
			nil,
			map[string]interface{}{
				"method": "rtm.auth.checkToken",
			},
		)
	}

	params := map[string]string{}
	// Corrected: Pass context to MakeRequest. Using Background().
	resp, err := c.MakeRequest(context.Background(), "rtm.auth.checkToken", params)
	if err != nil {
		// Add function context to Wrap message.
		return nil, errors.Wrap(err, "Client.CheckToken: failed API request.")
	}

	var response struct {
		Rsp struct {
			Stat  string `json:"stat"`
			Auth  *Auth  `json:"auth,omitempty"` // Pointer for optionality checking.
			Error *Error `json:"err,omitempty"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &response); err != nil {
		// Add function context to cgerr message.
		return nil, cgerr.NewRTMError(
			0,
			"Client.CheckToken: Failed to unmarshal token response.", // Added context.
			err,
			map[string]interface{}{
				"response_body_length": len(resp),
				"method":               "rtm.auth.checkToken",
			},
		)
	}

	if response.Rsp.Stat != "ok" {
		if response.Rsp.Error != nil {
			// Add function context to RTM error message.
			return nil, cgerr.NewRTMError(
				response.Rsp.Error.Code,
				fmt.Sprintf("Client.CheckToken: %s.", response.Rsp.Error.Msg), // Added context and period.
				nil,
				map[string]interface{}{
					"method": "rtm.auth.checkToken",
				},
			)
		}
		// Add function context to generic non-ok status message.
		return nil, cgerr.NewRTMError(
			0,
			fmt.Sprintf("Client.CheckToken: RTM API returned non-ok status: %s.", response.Rsp.Stat), // Added context.
			nil,
			map[string]interface{}{
				"method": "rtm.auth.checkToken",
				"status": response.Rsp.Stat,
			},
		)
	}

	// Ensure the nested Auth object is present on success.
	if response.Rsp.Auth == nil {
		// Add function context to missing auth info message.
		return nil, cgerr.NewRTMError(
			0,
			"Client.CheckToken: No auth information in response.", // Added context.
			nil,
			map[string]interface{}{
				"method": "rtm.auth.checkToken",
			},
		)
	}

	return response.Rsp.Auth, nil
}
