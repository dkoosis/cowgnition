// Package mcp defines common error types and variables used within the MCP server.
// file: internal/mcp/errors.go
package mcp

import (
	"github.com/dkoosis/cowgnition/internal/mcperror"
)

// Re-export the sentinel errors for backward compatibility
var (
	ErrResourceNotFound = mcperror.ErrResourceNotFound
	ErrToolNotFound     = mcperror.ErrToolNotFound
	ErrInvalidArguments = mcperror.ErrInvalidArguments
	ErrTimeout          = mcperror.ErrTimeout
)

// Re-export error checking functions for backward compatibility
var (
	IsResourceNotFoundError = mcperror.IsResourceNotFoundError
	IsToolNotFoundError     = mcperror.IsToolNotFoundError
	IsInvalidArgumentsError = mcperror.IsInvalidArgumentsError
)

// DocEnhanced: 2025-03-26
