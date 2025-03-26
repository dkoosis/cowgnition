// Package mcp defines common error variables used within the MCP server.
// file: internal/mcp/errors.go
package mcp

import "errors"

// MCP error definitions
// These variables provide standard error instances for common error conditions
// within the MCP application, allowing for consistent error handling and checking.
var (
	ErrResourceNotFound = errors.New("resource not found") // ErrResourceNotFound: Indicates that the requested resource could not be found.
	ErrToolNotFound     = errors.New("tool not found")     // ErrToolNotFound: Indicates that the specified tool could not be found.
	ErrInvalidArguments = errors.New("invalid arguments")  // ErrInvalidArguments: Indicates that the provided arguments were invalid.
)

// DocEnhanced: 2025-03-26
