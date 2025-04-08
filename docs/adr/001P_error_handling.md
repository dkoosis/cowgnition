# Go Error Handling Implementation Guide

PromptID: ErrorHandlingGuide
VersionDate: 2025-04-07
Status: Active

## Purpose

This document provides detailed guidelines, code examples, and checklists for implementing the Go error handling strategy defined in **ADR 001: Error Handling Strategy** for the CowGnition project. Use this guide when writing new code or reviewing existing code involving error handling. This guide is also designed to be used as context for AI assistants helping with coding or review tasks.

While professionalism and clarity are paramount in error handling, remember this is CowGnition! We encourage occasional, well-judged, non-obtrusive, cow puns in user-facing error messages, provided it never sacrifices clarity or helpfulness. Use sparingly and wisely. Don't milk it.

## Core Libraries & Technologies

- Go (`fmt`, `log/slog`)
- `github.com/cockroachdb/errors`
- JSON-RPC 2.0
- Model Context Protocol (MCP)

## Related Documents

- [ADR 001: Error Handling Strategy](001_error_handling_strategy.md)
- [MCP Specification (Concepts, Logging)](link/to/mcp/spec)
- [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)
- [`cockroachdb/errors` Documentation](https://pkg.go.dev/github.com/cockroachdb/errors)

---

## 1. Creating & Wrapping Internal Application Errors

**(Goal:** Generate informative internal errors with context and stack traces using `cockroachdb/errors`.)

**Key Patterns:**

- **Wrap Errors:** Use `errors.Wrap` or `errors.Wrapf` when crossing logical boundaries or adding context to preserve the stack trace.
- **Add Context:** Attach structured key-value details using `errors.WithDetail(err, key, value)` or `errors.WithDetails(err, kv...)`.
- **Custom Error Types:** Define custom error types (e.g., `RTMError`) for domain-specific errors to aid mapping and handling. Implement the standard `error` interface and potentially `errors.Unwrap`.
- **Sentinel Errors:** Use exported `var` errors (e.g., `var ErrNotFound = errors.New(...)`) for fixed conditions, checkable with `errors.Is`.

**Example: Creating a Domain-Specific Error**

```go
package rtm

import (
	"fmt"
	"[github.com/cockroachdb/errors](https://github.com/cockroachdb/errors)"
)

// ErrCode defines internal application error codes for RTM interactions.
type ErrCode int

const (
	ErrRTMUnavailable ErrCode = iota + 1 // Example internal code
	ErrRTMAuthFailed
	ErrRTMTaskNotFound
)

// RTMError is a custom error type for RTM service failures.
type RTMError struct {
	Code    ErrCode // Internal code for mapping/logging
	Message string  // Human-readable message for internal logs
	Cause   error   // The underlying error, if any
}

// Error implements the standard error interface.
func (e *RTMError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("RTMError (Code: %d): %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("RTMError (Code: %d): %s", e.Code, e.Message)
}

// Unwrap provides compatibility with errors.Is/As for the Cause.
func (e *RTMError) Unwrap() error {
	return e.Cause
}

// Example function demonstrating error creation and wrapping
func accessRTM(apiKey string, taskID string) error {
	rtmLibError := errors.New("timeout connecting to RTM API host") // Simulated underlying error

	if rtmLibError != nil {
		appErr := &RTMError{
			Code:    ErrRTMUnavailable,
			Message: "Could not connect to Remember The Milk API",
			Cause:   rtmLibError,
		}
		errWithContext := errors.WithDetail(appErr, "rtm_task_id", taskID)
		errWithContext = errors.WithDetail(errWithContext, "rtm_endpoint_attempted", "[https://api.rememberthemilk.com/services/rest/](https://api.rememberthemilk.com/services/rest/)")
		// Wrap to add layer context and capture stack trace here
		return errors.Wrap(errWithContext, "failed retrieving RTM task details")
	}
	return nil
}
```
