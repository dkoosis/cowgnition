# Development Notes

This document logs key decisions, tooling quirks, and findings to avoid revisiting the same issues in the future.

## Testing Strategy Pivot - March 2025

### Decision: Focus on Live API Testing

We've decided to prioritize testing against the live RTM API instead of perfecting mock-based tests for several reasons:

1.  **Uncertain API behavior**: The documentation may not perfectly reflect actual API behavior
2.  **Mock maintenance cost**: Continuously refining mocks is time-consuming with diminishing returns
3.  **Real-world validation**: Only live testing can confirm our integration actually works as expected

### Implementation approach:

- Create a configurable test framework supporting both live and mock testing
- Default to live API when credentials are available
- Fall back to basic mocks for CI and cases without credentials
- Focus test efforts on validating MCP server conformance with the spec

This approach saves development time and provides more useful verification of our integration.

## Linting Configuration

### GoLangCI-Lint Version Quirks

- **Issue**: The `shadow` linter isn't available in all versions of golangci-lint
- **Decision**: Remove the deprecated `govet.check-shadowing` option entirely rather than replacing it with the `shadow` linter
- **Date**: March 15, 2025
- **Resolution**: Modified `.golangci.yml` to remove the `check-shadowing` option from `govet` settings

### Missing RTM Service Methods

- **Issue**: Linter errors for undefined methods `IsAuthenticated` and `CleanupExpiredFlows` in `internal/rtm/service.go`
- **Decision**: Implement these methods to complete the RTM authentication flow
- **Date**: March 15, 2025
- **Resolution**: Added implementation for:
  - `IsAuthenticated()` - Verifies if the user has valid authentication
  - `CleanupExpiredFlows()` - Removes expired authentication flows
  - `StartAuthFlow()` - Initiates the RTM authentication process
  - `CompleteAuthFlow()` - Finalizes authentication with a frob

## Tooling Decisions

### Code Organization

- Package `rtm` should not import from `server` to avoid circular dependencies
- Authentication types and interfaces are defined in both `auth` and `rtm` packages to prevent cycles

### Build Configuration

- Using Go modules for dependency management
- Build tags used to separate MCP server implementations
- Version information injected at build time via ldflags

### Testing Strategy

- Unit tests focus on individual package functionality
- Integration tests use the real MCP protocol against mock RTM responses
- End-to-end tests verify authentication flows and API calls

## IDE Configuration

- `.editorconfig` ensures consistent formatting across editors
- VSCode settings configured for Go development with gopls
- Using goimports for automatic import organization

## Deployment Considerations

- MCP server needs to be registered with Claude Desktop
- Authentication tokens stored in user's home directory
- OAuth-like flow requires user interaction for initial setup

## Error Handling Strategy for MCP Server (JSON-RPC 2.0 Compliance) - _NEW_

- **Issue:** Need a robust and consistent error handling strategy that conforms to the JSON-RPC 2.0 specification mandated by MCP, while also providing high-quality error messages for developers, operations, and MCP clients.
- **Date:** (Insert Date Here - e.g., March 18, 2025)
- **Decision:** Implement a structured approach using a custom `ErrorResponse` struct that mirrors the JSON-RPC 2.0 error format, along with helper functions for generating and handling errors. This balances MCP compliance with Go's error handling best practices and detailed debugging needs.

### Key Components:

1.  **`ErrorResponse` Struct:** Represents the JSON-RPC 2.0 error structure:

    ```json
    {
      "jsonrpc": "2.0",
      "error": {
        "code": -32000,
        "message": "Server error",
        "data": {
          /* Optional: Additional error details */
        }
      },
      "id": 1
    }
    ```

2.  **`ErrorObject` Struct:** Represents the inner "error" object within the `ErrorResponse`.

3.  **`NewErrorResponse` Function:** Constructor for `ErrorResponse` instances.

4.  **Error Code Constants:** Define constants for both standard JSON-RPC error codes and custom application-specific error codes.

5.  **`Errorf` Function:** A helper function to create `ErrorResponse` instances with formatted messages, similar to `fmt.Errorf`.

6.  **`WriteHTTPError` Method:** Handles writing the `ErrorResponse` as an HTTP response, setting correct headers and status codes.

7.  **`DetailedError` Struct:** A custom struct to hold developer-focused error information (stack trace, original error, context) for internal logging and debugging, _without_ exposing these details to the client.

8.  **`withStackTrace` Function:** Creates a `DetailedError`, captures a stack trace, and includes provided context.

### Implementation Notes:

- Use `interface{}` (or `any`) for the `id` field in `ErrorResponse` to handle strings, numbers, and null values correctly.
- Utilize `json.RawMessage` for handling the `params` field in incoming requests.
- Log `DetailedError` instances for internal debugging, but _only_ send a generic `ErrorResponse` to the client to avoid information leakage.
- Use appropriate HTTP status codes (4xx for client errors, 5xx for server errors).
- Consider a middleware pattern for centralized error handling.
- Leverage structured logging for efficient log analysis.

### Benefits:

- **Full MCP Compliance:** Ensures adherence to the JSON-RPC 2.0 specification.
- **Idiomatic Go:** Integrates seamlessly with Go's `error` interface.
- **Rich Debugging:** Provides detailed error information for developers.
- **Secure:** Prevents exposure of internal implementation details to clients.
- **Maintainable and Scalable**: Provides a clear framework as more complex error scenarios are added.

This structured approach provides a clear, maintainable, and compliant solution for error handling in the MCP server. This decision document links the problem to the solution detailed in the code. It also highlights the key design considerations.
