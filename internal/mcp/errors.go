// internal/mcp/errors.go
package mcp

import "errors"

// MCP error definitions
var (
	ErrResourceNotFound = errors.New("resource not found")
	ErrToolNotFound     = errors.New("tool not found")
	ErrInvalidArguments = errors.New("invalid arguments")
)
