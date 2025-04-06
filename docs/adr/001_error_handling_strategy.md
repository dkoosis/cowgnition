# Architecture Decision Record: Error Handling Strategy

## Date
April 6, 2025

## Status
Accepted

## Context
The CowGnition project is implementing a Model Context Protocol (MCP) server that integrates with Remember The Milk. For this implementation, we need a robust error handling strategy that provides:

1. **Structured Error Types**: Domain-specific error types for different categories
2. **Context-Rich Errors**: Including relevant context data with all errors
3. **Consistent Wrapping**: Preserving the error chain
4. **Error Codes**: Consistent categorization
5. **Stack Trace Preservation**: Capturing origin points for debugging

Additionally, we need to ensure compliance with:
- JSON-RPC 2.0 error response format requirements
- MCP logging specification for error reporting

## Decision
We will use the `github.com/cockroachdb/errors` package as our primary error handling library, combined with custom error types specific to our application domains.

Key implementation patterns:
1. **Domain-Specific Error Types**: Create custom error types that embed `TransportError`, `MCPError`, etc.
2. **Context Attachment**: Use `errors.WithDetail()` to attach key-value pairs to errors
3. **Stack Capture**: Use `errors.WithStack()` when wrapping errors from external sources
4. **Consistent Wrapping**: Always wrap errors when crossing domain boundaries
5. **Central Error Processing**: Implement middleware that transforms internal errors to JSON-RPC responses

## Consequences

### Positive
- Rich debugging information with stack traces
- Consistent pattern for error context across the codebase
- Clear mapping between internal errors and JSON-RPC error responses
- Compliance with MCP logging specifications
- Better developer experience when troubleshooting issues

### Negative
- Learning curve for team members not familiar with cockroachdb/errors
- Need for discipline in consistently applying the patterns
- Slightly increased dependency footprint

## Implementation Guidelines

### Error Creation
```go
// For new errors at source
err := &TransportError{
    Code:    ErrMessageTooLarge,
    Message: fmt.Sprintf("message size %d exceeds maximum allowed size %d", size, maxSize),
    Context: map[string]interface{}{
        "messageSize": size,
        "maxSize":     maxSize,
        "timestamp":   time.Now().UTC(),
    },
}
return errors.WithStack(err)

// For wrapping existing errors
if err != nil {
    return errors.Wrap(err, "failed to parse message")
}
```

### Error Logging
All errors should be logged with:
- Error code
- Error message
- Stack trace
- All context fields
- Service name
- Request ID (when available)

This aligns with the MCP logging specification that requires structured error information.

### JSON-RPC Error Mapping
Internal application errors should be mapped to JSON-RPC 2.0 error responses:

```go
{
    "jsonrpc": "2.0",
    "id": requestID,
    "error": {
        "code": mappedJSONRPCCode,
        "message": userFriendlyMessage,
        "data": safeContextData
    }
}
```

Common mappings:
- Parse errors → -32700
- Invalid request → -32600
- Method not found → -32601
- Invalid params → -32602
- Internal errors → -32603
- Server-defined errors → -32000 to -32099

### Security Considerations
- Do not include sensitive information in error messages or context
- Stack traces should only be logged server-side, never sent to clients
- Sanitize any error details included in the JSON-RPC `data` field

## Related Specifications
This decision aligns with:
1. JSON-RPC 2.0 Specification
2. MCP Logging Specification (2024-11-05)

## References
1. [cockroachdb/errors Documentation](https://pkg.go.dev/github.com/cockroachdb/errors)
2. [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)
3. [MCP Specification - Logging](https://spec.modelcontextprotocol.io/specification/2024-11-05/server/utilities/logging/)
