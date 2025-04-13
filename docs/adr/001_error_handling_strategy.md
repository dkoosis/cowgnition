# Architecture Decision Record: Error Handling Strategy (ADR 001)

## Date

2025-04-07 _(Original: 2025-04-06)_

## Context

The CowGnition project is implementing a Model Context Protocol (MCP) server that integrates with Remember The Milk (RTM). For this implementation, we need a robust error handling strategy that provides:

1.  **Structured Error Types**: Domain-specific error types for different categories.
2.  **Context-Rich Errors**: Including relevant context data with all errors using `cockroachdb/errors`.
3.  **Consistent Wrapping**: Preserving the error chain for accurate stack traces.
4.  **Standardized Error Codes**: Consistent categorization, aligning with JSON-RPC 2.0 and application needs.
5.  **Stack Trace Preservation**: Capturing origin points for debugging (server-side only).
6.  **Clear Logging**: Adherence to MCP logging specifications for observability.

Additionally, we need to ensure compliance with:

- JSON-RPC 2.0 error response format requirements.
- Model Context Protocol (MCP) best practices for error reporting (e.g., distinguishing tool execution errors).
- MCP logging specification recommendations.

## Decision

1.  We will use the `github.com/cockroachdb/errors` package for internal error handling, combined with custom error types for specific domains (Transport, MCP, RTM Service, etc.).
2.  Validation errors identified by **Validation Middleware (ADR 002)** will be mapped immediately to standard JSON-RPC errors (`-32700`, `-32600`, `-32602`) and returned, preventing further processing.
3.  Errors occurring _during the execution of MCP Tools_ (`tools/call` handlers) will be caught within the handler, logged internally with full detail, and reported to the client via a successful JSON-RPC response containing an `mcp.CallToolResult` with `isError: true` and sanitized error content.
4.  All other internal application errors caught by central error handling middleware will be logged in detail server-side (following MCP specs) and mapped to appropriate JSON-RPC error responses (`-32603` or custom `-320xx` codes) with sanitized messages for the client.
5.  Structured logging (`log/slog` recommended) adhering to the MCP logging specification fields will be used for all error logging.

## Implementation Guidelines

Detailed implementation patterns, Go code examples, security checklists, guidance for developers and AI assistants, and notes on bovine humor are documented separately in:

**[`001_ERROR_HANDLING_PROMPT.md`](001_ERROR_HANDLING_PROMPT.md)**

Developers **must** consult this guide when implementing or reviewing error handling code. It covers specific usage of `cockroachdb/errors`, required logging fields, error mapping logic, `tools/call` error handling, security best practices, and our pun policy.

### 7. Testing Error Handling

When writing tests that assert specific error conditions, **avoid** relying on matching substrings within the `error.Error()` string output (e.g., using `strings.Contains` or `testify/assert.Contains` on the error string). This approach is brittle and prone to breaking due to minor changes in error formatting or wrapping.

Instead, **prefer** the following robust patterns:

1.  **Check Error Type:** Use `errors.As` to verify that the returned error is, or wraps, the expected custom error type (e.g., `*schema.ValidationError`, `*transport.Error`, `*mcperrors.RTMError`). This confirms the general category of the error.
2.  **Check Specific Error Fields (Codes/Types):** After confirming the type with `errors.As`, assert on specific fields within the custom error struct to verify the _reason_ for the error. Use the defined constants for codes or types (e.g., `assert.Equal(t, schema.ErrValidationFailed, validationErr.Code)`).
3.  **Check Sentinel Errors:** For specific, fixed error conditions represented by exported variables, use `errors.Is`.

This approach leverages the structured error types defined in the project, aligns with standard Go error handling practices, and creates more maintainable and reliable tests.

**Example Test Snippet:**

```go
    // ... (previous test setup)
    err := functionUnderTest() // Function that returns an error

    // Assert an error occurred
    require.Error(t, err, "Expected an error from functionUnderTest")

    // Check if the error is the specific custom type we expect
    var specificErr *schema.ValidationError // Replace with actual expected type
    require.True(t, errors.As(err, &specificErr), "Error should be (or wrap) the expected type")

    // Assert on the specific error code within the custom type
    assert.Equal(t, schema.ErrValidationFailed, specificErr.Code, "Error code should match expected reason")

    // Optionally, assert on other relevant fields like InstancePath
    assert.Contains(t, specificErr.InstancePath, "/expected/path/field", "Instance path should indicate the location")

## Error Handling Architecture: Transport and MCP Error Relationship

### Overview

This document elaborates on the relationship between error types defined in our two main error handling modules:

1.  Transport-level errors (`internal/transport/errors.go`)
2.  MCP-level errors (`internal/mcp/errors/errors.go`)

### Error Domain Separation

Our architecture maintains a clear separation between errors that occur at different layers:

**Transport Layer Errors**

- **Domain**: Low-level communication issues
- **Location**: `internal/transport/errors.go`
- **Examples**: Message size limits, connection closures, timeouts, JSON parsing failures
- **Primary Consumers**: Internal transport handling components

**MCP Layer Errors**

- **Domain**: Protocol and application-level issues
- **Location**: `internal/mcp/errors/errors.go`
- **Examples**: Authentication failures, RTM API failures, resource access issues
- **Primary Consumers**: MCP handlers and client responses

### Error Flow Through The System

The error flow follows a consistent pattern:

1.  **Error Origin**: Errors are created at their respective layers.
2.  **Error Propagation**: Errors propagate up through middleware or handlers.
3.  **Error Translation**: Middleware or handlers map errors to appropriate JSON-RPC responses or `CallToolResult` structures.
4.  **Error Response**: Client receives standard JSON-RPC error or a successful response containing `CallToolResult`.

### When To Use Which Error Type

- **Use Transport Errors When:** Handling raw IO, message framing, initial JSON parsing, connection lifecycle.
- **Use MCP Errors When:** Processing MCP requests, handling auth, interacting with external services (RTM), dealing with application resources/state.

### Error Mapping Guidelines

Transport errors typically map to JSON-RPC `-32700` (Parse Error) or `-32600` (Invalid Request). Other internal application errors map to `-32603` (Internal Error) or custom `-320xx` codes, unless they are Tool execution errors (see Decision #3).

### Error Context Guidelines

1.  **Transport Errors**: Include technical details (message size, timeouts, connection IDs).
2.  **MCP Errors**: Include application context (resource IDs, request params, auth details - _never_ credentials).

### Future Considerations

1.  **Error Telemetry**: Enhance tracking across layers.
2.  **Error Recovery**: Add specific recovery paths.
3.  **Client-Friendly Details**: Improve error messages (potentially with puns).

## JSON-RPC & MCP Error Codes

The following standard JSON-RPC 2.0 error codes (referenced by MCP) will be used:

| Code             | JSON-RPC Meaning                    | Typical MCP Server Use Case                                                                  | Generated By                               |
| :--------------- | :---------------------------------- | :------------------------------------------------------------------------------------------- | :----------------------------------------- |
| -32700           | Parse Error                         | Invalid JSON received (Syntax error in the message)                                          | Initial JSON parsing (before middleware)   |
| -32600           | Invalid Request                     | Message is not a valid Request/Notification object (e.g., missing `jsonrpc`, `method`)       | Validation Middleware (Schema check)       |
| -32601           | Method Not Found                    | Requested MCP method (e.g., `resources/list`, `unknown/tool`) is not implemented/supported   | Router/Dispatcher or Validation Middleware |
| -32602           | Invalid Params                      | Method parameters are invalid (wrong type, missing required field for the _specific_ method) | Validation Middleware (Schema check)       |
| -32603           | Internal Error                      | Unspecified server-side error during handler processing (not validation or tool errors)      | Central Error Handler (Post-Handler)       |
| -32000 to -32099 | Implementation-defined server error | Custom _application_ errors (e.g., RTM API Failure, Resource Not Found, Access Denied)       | Central Error Handler (Post-Handler)       |

*Note: Errors during `tools/call` execution are generally *not* returned using these codes, but within the successful response's `CallToolResult`.*

## Consequences

### Positive

- Rich debugging information with stack traces and structured context (server-side).
- Consistent pattern for error creation, propagation, and handling.
- Clear mapping between internal errors and client responses (JSON-RPC or `CallToolResult`).
- Compliance with MCP logging specifications.
- Correctly handles tool execution errors as per MCP best practices.
- Potentially amusing error messages.

### Negative

- Learning curve for `cockroachdb/errors`.
- Need for discipline in applying patterns (especially distinguishing tool vs. other errors).
- Slightly increased dependency footprint.
- Requires careful mapping/sanitization logic.
- Risk of unfunny or inappropriate puns if not applied judiciously.

## Related Specifications

1.  JSON-RPC 2.0 Specification
2.  MCP Specification (e.g., 2024-11-05 version for concepts)
3.  [MCP Logging Specification (2024-11-05)](https://spec.modelcontextprotocol.io/specification/2024-11-05/server/utilities/logging/)

## References

1.  [`cockroachdb/errors` Documentation](https://pkg.go.dev/github.com/cockroachdb/errors)
2.  [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)
3.  [MCP Specification - Concepts](https://www.google.com/search?q=https://modelcontextprotocol.io/docs/concepts/)
4.  [MCP Specification - Logging](https://spec.modelcontextprotocol.io/specification/2024-11-05/server/utilities/logging/)
```
