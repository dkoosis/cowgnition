// Package rtm handles Remember The Milk (RTM) authentication.
// file: internal/rtm/auth.go
package rtm

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// Response represents the standard RTM API response wrapper.
// It is used to consistently parse the outer structure of RTM API responses,
// which always include a 'stat' field and potentially an 'err' field for errors.
type Response struct {
	Stat  string `json:"stat"`          // Stat: Indicates the status of the API call ("ok" or "fail").
	Error *Error `json:"err,omitempty"` // Error: Contains error details if the API call failed.
}

// Error represents an RTM API error.
// It provides a structured way to handle and interpret errors returned by the RTM API,
// including an error code and a descriptive message.
type Error struct {
	Code int    `json:"code"` // Code: The RTM-specific error code.
	Msg  string `json:"msg"`  // Msg: A human-readable error message.
}

// User represents an RTM user.
// It encapsulates the basic user information returned by the RTM API,
// such as ID, username, and full name.
type User struct {
	ID       string `json:"id,attr"`       // ID: The user's unique ID.
	Username string `json:"username,attr"` // Username: The user's username.
	Fullname string `json:"fullname,attr"` // Fullname: The user's full name.
}

// Auth represents an RTM authentication response.
// It contains the authentication token, permissions, and user information
// obtained after successful authentication.
type Auth struct {
	Token string `json:"token"` // Token: The authentication token.
	Perms string `json:"perms"` // Perms: The granted permissions.
	User  User   `json:"user"`  // User: The authenticated user.
}

// GetFrob gets a frob from RTM for desktop authentication flow.
// The "frob" is a temporary credential used in the RTM authentication process.
// It is the first step in obtaining an authentication token for desktop applications.
//
// Returns:
//
//	string: The frob string.
//	error:  An error if the API request fails or if the response is invalid.
func (c *Client) GetFrob() (string, error) {
	params := map[string]string{}                          // No parameters needed for getting a frob.
	resp, err := c.MakeRequest("rtm.auth.getFrob", params) // Make the API request.
	if err != nil {
		return "", fmt.Errorf("failed to get frob: %w", err) // Wrap the error with context.
	}

	var response struct { // Define a struct to unmarshal the API response.
		Rsp struct {
			Stat  string `json:"stat"`
			Frob  string `json:"frob,omitempty"`
			Error *Error `json:"err,omitempty"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal frob response: %w", err) // Handle unmarshaling errors.
	}

	if response.Rsp.Stat != "ok" { // Check if the API call was successful.
		if response.Rsp.Error != nil {
			return "", fmt.Errorf("RTM API error: %d - %s", response.Rsp.Error.Code, response.Rsp.Error.Msg) // Return RTM API error.
		}
		return "", fmt.Errorf("RTM API returned non-ok status: %s", response.Rsp.Stat) // Return non-ok status error.
	}

	return response.Rsp.Frob, nil // Return the frob.
}

// GetAuthURL generates an authentication URL for desktop application flow.
// This URL is used to redirect the user to the RTM website to grant permissions to the application.
//
// frob string: The frob obtained from GetFrob.
// perms string: The permissions being requested ("read", "write", or "delete").
//
// Returns:
//
//	string: The authentication URL.
func (c *Client) GetAuthURL(frob, perms string) string {
	params := map[string]string{ // Prepare the parameters for the authentication URL.
		"api_key": c.APIKey, // Include the API key.
		"perms":   perms,    // Include the requested permissions.
		"frob":    frob,     // Include the frob.
	}

	// Sign parameters
	signature := c.Sign(params) // Generate the API signature.

	// Build URL
	values := url.Values{} // Use url.Values to properly encode the URL.
	for k, v := range params {
		values.Add(k, v) // Add each parameter to the URL values.
	}
	values.Add("api_sig", signature) // Add the API signature.

	return AuthURL + "?" + values.Encode() // Construct the full authentication URL.
}

// GetToken gets an auth token for the given frob.
// This is the final step in the authentication process, where the temporary frob is exchanged for a permanent authentication token.
//
// frob string: The frob obtained from GetFrob and used to authorize the application.
//
// Returns:
//
//	*Auth: The authentication information, including the token, permissions, and user.
//	error: An error if the API request fails, the response is invalid, or authentication fails.
func (c *Client) GetToken(frob string) (*Auth, error) {
	params := map[string]string{ // Prepare the parameters for the API request.
		"frob": frob, // Include the frob.
	}

	resp, err := c.MakeRequest("rtm.auth.getToken", params) // Make the API request.
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err) // Wrap the error with context.
	}

	var response struct { // Define a struct to unmarshal the API response.
		Rsp struct {
			Stat  string `json:"stat"`
			Auth  *Auth  `json:"auth,omitempty"`
			Error *Error `json:"err,omitempty"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token response: %w", err) // Handle unmarshaling errors.
	}

	if response.Rsp.Stat != "ok" { // Check if the API call was successful.
		if response.Rsp.Error != nil {
			return nil, fmt.Errorf("RTM API error: %d - %s", response.Rsp.Error.Code, response.Rsp.Error.Msg) // Return RTM API error.
		}
		return nil, fmt.Errorf("RTM API returned non-ok status: %s", response.Rsp.Stat) // Return non-ok status error.
	}

	if response.Rsp.Auth == nil {
		return nil, fmt.Errorf("no auth information in response") // Handle missing auth information.
	}

	return response.Rsp.Auth, nil // Return the authentication information.
}

// CheckToken verifies if the auth token is valid.
// This method is used to check the validity of an existing authentication token.
//
// Returns:
//
//	*Auth: The authentication information if the token is valid.
//	error: An error if no auth token is set, the API request fails, the response is invalid, or the token is invalid.
func (c *Client) CheckToken() (*Auth, error) {
	if c.AuthToken == "" {
		return nil, fmt.Errorf("no auth token set") // Return an error if no auth token is set.
	}

	params := map[string]string{}                             // No parameters needed for checking the token.
	resp, err := c.MakeRequest("rtm.auth.checkToken", params) // Make the API request.
	if err != nil {
		return nil, fmt.Errorf("failed to check token: %w", err) // Wrap the error with context.
	}

	var response struct { // Define a struct to unmarshal the API response.
		Rsp struct {
			Stat  string `json:"stat"`
			Auth  *Auth  `json:"auth,omitempty"`
			Error *Error `json:"err,omitempty"`
		} `json:"rsp"`
	}

	if err := json.Unmarshal(resp, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token response: %w", err) // Handle unmarshaling errors.
	}

	if response.Rsp.Stat != "ok" { // Check if the API call was successful.
		if response.Rsp.Error != nil {
			return nil, fmt.Errorf("RTM API error: %d - %s", response.Rsp.Error.Code, response.Rsp.Error.Msg) // Return RTM API error.
		}
		return nil, fmt.Errorf("RTM API returned non-ok status: %s", response.Rsp.Stat) // Return non-ok status error.
	}

	if response.Rsp.Auth == nil {
		return nil, fmt.Errorf("no auth information in response") // Handle missing auth information.
	}

	return response.Rsp.Auth, nil // Return the authentication information.
}

// DocEnhanced: 2025-03-26
