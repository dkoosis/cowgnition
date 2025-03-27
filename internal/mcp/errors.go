// Package mcp defines common error types and variables used within the MCP server.
// file: internal/mcp/errors.go
package mcp

import (
	"github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// Re-export the sentinel errors for backward compatibility.
var (
	ErrResourceNotFound = mcp / errors.ErrResourceNotFound
	ErrToolNotFound     = mcp / errors.ErrToolNotFound
	ErrInvalidArguments = mcp / errors.ErrInvalidArguments
	ErrTimeout          = mcp / errors.ErrTimeout
)

// Re-export error checking functions for backward compatibility.
var (
	IsResourceNotFoundError = mcp / errors.IsResourceNotFoundError
	IsToolNotFoundError     = mcp / errors.IsToolNotFoundError
	IsInvalidArgumentsError = mcp / errors.IsInvalidArgumentsError
)

// DocEnhanced: 2025-03-26
