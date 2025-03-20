// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import "time"

// Status represents the authentication status of the RTM service.
type Status int

const (
	// StatusUnknown means the authentication status has not been determined.
	StatusUnknown Status = iota
	// StatusNotAuthenticated means the user is not authenticated.
	StatusNotAuthenticated
	// StatusAuthenticating means authentication is in progress.
	StatusAuthenticating
	// StatusAuthenticated means the user is authenticated.
	StatusAuthenticated
	// StatusExpired means the authentication has expired.
	StatusExpired
	// StatusFailed means the authentication attempt failed.
	StatusFailed
)

// String returns a string representation of the auth status.
func (s Status) String() string {
	switch s {
	case StatusUnknown:
		return "Unknown"
	case StatusNotAuthenticated:
		return "Not Authenticated"
	case StatusAuthenticating:
		return "Authentication Pending"
	case StatusAuthenticated:
		return "Authenticated"
	case StatusExpired:
		return "Authentication Expired"
	case StatusFailed:
		return "Authentication Failed"
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

// String returns a string representation of the permission level.
func (p Permission) String() string {
	return string(p)
}

// AuthFlow represents an ongoing authentication flow with RTM.
type AuthFlow struct {
	Frob       string
	AuthURL    string
	StartTime  time.Time
	Permission Permission
	ExpiresAt  time.Time
}
