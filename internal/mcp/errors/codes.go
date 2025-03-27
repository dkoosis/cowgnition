// Package mcp/errors defines error types, codes, and utilities for MCP and JSON-RPC.
// file: internal/mcp/errors/codes.go
package errors

// Categories for grouping similar errors.
const (
	CategoryResource = "resource" // Resource-related errors
	CategoryTool     = "tool"     // Tool-related errors
	CategoryAuth     = "auth"     // Authentication-related errors
	CategoryConfig   = "config"   // Configuration-related errors
	CategoryRPC      = "rpc"      // JSON-RPC-related errors
	CategoryRTM      = "rtm"      // Remember The Milk API-related errors
)

// Error codes aligned with JSON-RPC 2.0 specification.
const (
	// Standard JSON-RPC 2.0 error codes (-32768 to -32000 reserved).
	CodeParseError     = -32700 // Invalid JSON received
	CodeInvalidRequest = -32600 // Invalid request object
	CodeMethodNotFound = -32601 // Method not found
	CodeInvalidParams  = -32602 // Invalid method parameters
	CodeInternalError  = -32603 // Internal JSON-RPC error

	// Custom application error codes (-32000 to -32099 for server errors).
	CodeResourceNotFound = -32000 // Requested resource not found
	CodeToolNotFound     = -32001 // Requested tool not found
	CodeInvalidArguments = -32002 // Invalid arguments provided
	CodeAuthError        = -32003 // Authentication error
	CodeRTMError         = -32004 // Remember The Milk API error
	CodeTimeoutError     = -32005 // Operation timed out
)

// UserFacingMessage returns a user-friendly message based on error code.
func UserFacingMessage(code int) string {
	switch code {
	case CodeParseError:
		return "Failed to parse JSON request"
	case CodeInvalidRequest:
		return "Invalid request format"
	case CodeMethodNotFound:
		return "Method not found"
	case CodeInvalidParams:
		return "Invalid method parameters"
	case CodeResourceNotFound:
		return "Requested resource not found"
	case CodeToolNotFound:
		return "Requested tool not found"
	case CodeInvalidArguments:
		return "Invalid arguments provided"
	case CodeAuthError:
		return "Authentication failed"
	case CodeRTMError:
		return "Error communicating with Remember The Milk"
	case CodeTimeoutError:
		return "Request timed out"
	default:
		return "Internal server error"
	}
}
