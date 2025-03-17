// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import (
	"time"
)

// Status represents the current authentication status.
type Status int

const (
	// StatusUnknown indicates that the authentication status is not determined yet.
	StatusUnknown Status = iota
	// StatusNotAuthenticated indicates that the user is not authenticated.
	StatusNotAuthenticated
	// StatusPending indicates that authentication is in progress.
	StatusPending
	// StatusAuthenticated indicates that the user is authenticated.
	StatusAuthenticated
	// StatusExpired indicates that the authentication has expired.
	StatusExpired
)

// String returns a string representation of the auth status.
func (s Status) String() string {
	switch s {
	case StatusUnknown:
		return "Unknown"
	case StatusNotAuthenticated:
		return "Not Authenticated"
	case StatusPending:
		return "Authentication Pending"
	case StatusAuthenticated:
		return "Authenticated"
	case StatusExpired:
		return "Authentication Expired"
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

// Flow represents an ongoing authentication flow.
type Flow struct {
	Frob       string
	StartTime  time.Time
	Permission Permission
	AuthURL    string
	ExpiresAt  time.Time
	CallerInfo string // Optional information about who initiated the flow
}

// IsExpired checks if the authentication flow has expired.
func (f *Flow) IsExpired() bool {
	return time.Now().After(f.ExpiresAt)
}

// TimeRemaining returns the time remaining before the flow expires.
func (f *Flow) TimeRemaining() time.Duration {
	if f.IsExpired() {
		return 0
	}
	return time.Until(f.ExpiresAt)
}

// ErrorMsgEnhanced: 2024-02-29
