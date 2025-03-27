// Package mcp defines common error types and variables used within the MCP server.
// file: internal/mcp/errors.go
package mcp

import cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"

// Re-export the sentinel errors for backward compatibility.
var (
	ErrResourceNotFound = cgerr.ErrResourceNotFound
	ErrToolNotFound     = cgerr.ErrToolNotFound
	ErrInvalidArguments = cgerr.ErrInvalidArguments
	ErrTimeout          = cgerr.ErrTimeout
)

// Re-export error checking functions for backward compatibility.
var (
	IsResourceNotFoundError = cgerr.IsResourceNotFoundError
	IsToolNotFoundError     = cgerr.IsToolNotFoundError
	IsInvalidArgumentsError = cgerr.IsInvalidArgumentsError
)

// DocEnhanced: 2025-03-26
