# CowGnition Implementation Roadmap

## Top Priority

JSON-RPC and Error Handling Cleanup Recommendations
Recommended Approach

Standardize on jsonrpc2 library types:

Gradually replace usages of custom types with sourcegraph/jsonrpc2 library types
Remove the custom types in jsonrpc_types.go
Update handler functions to work with library types directly

Clean up error handling:

Ensure all error creation uses the helper functions from cgerr package
Remove duplicate error code definitions
Centralize error conversion logic in one place
Remove unused or redundant error utility functions

#2 Error Handling Tidying Opportunities

Remove duplicated error conversion logic:

internal/mcp/errors/utils.go and internal/jsonrpc/jsonrpc_handler.go both contain error conversion logic.
We could centralize all error conversion in internal/mcp/errors/utils.go and use it consistently.

Standardize error creation patterns:

Some places use direct errors.New/Wrap while others use the helper functions like cgerr.NewToolError.
We should standardize on using the helper functions for better consistency.

Consolidate error codes:

There are error codes defined in multiple places (internal/mcp/errors/codes.go and internal/httputils/response.go).
We should have a single source of truth for error codes.

Remove unused error functions:

Some error utility functions in internal/mcp/errors/utils.go are not used throughout the codebase.
We can simplify by removing them or marking them as deprecated.

#3 JSON-RPC2 Library Tidying Opportunities

Remove custom JSON-RPC types:

internal/jsonrpc/jsonrpc_types.go defines several types that duplicate functionality from sourcegraph/jsonrpc2.
These could be removed and replaced with direct usage of the library's types.

Standardize on a single Message handling approach:

The codebase has multiple ways of handling JSON-RPC messages: using custom types and using library types.
We should standardize on using the jsonrpc2.Request and jsonrpc2.Response types directly.

Remove redundant error mapping:

There's redundant mapping between custom error types and jsonrpc2 error objects.
We could simplify by using the ToJSONRPCError function consistently.

Consolidate transport implementations:

There are multiple transport implementations that could be simplified.
Standardize on a clear pattern for all transports (stdio, HTTP) using the jsonrpc2 library.

Specific Files to Clean Up

internal/jsonrpc/jsonrpc_types.go:

This file contains custom types like Request, Response, and Notification that duplicate the functionality from the jsonrpc2 library.
Most of this file could be removed in favor of using the library's types directly.

internal/mcp/errors.go:

This file only re-exports errors from internal/mcp/errors/ package.
Could be removed and direct imports of cgerr used instead.

internal/jsonrpc/jsonrpc_handler.go:

Contains custom error conversion logic that duplicates ToJSONRPCError from the errors package.
Should be refactored to use the centralized error conversion.

internal/httputils/response.go:

Contains error code definitions that overlap with internal/mcp/errors/codes.go.
Should be refactored to use the canonical error codes.

# CowGnition MCP Implementation Roadmap: Urgent Fixes

## Priority Tasks for Codebase Cleanup and Standardization

### 1. Complete State Machine Implementation ✅ (Mostly Complete)

- [x] Implement `qmuntal/stateless` for connection management
- [x] Define state transitions and handlers
- [x] Integrate connection manager with server logic
- [ ] Add comprehensive tests for state machine behavior
- [ ] Review error handling in state transitions
- [ ] Ensure proper cleanup of resources in all states

### 2. Standardize Error Handling ✅ (Mostly Complete)

- [x] Consolidate on `cockroachdb/errors` package for error operations
- [x] Define consistent error categories and codes
- [x] Create helper functions for error conversion and wrapping
- [x] Document error handling approach in `error_handling_guidelines.md`
- [ ] Add tests for error conversion and handling
- [ ] Review error messages for consistency and helpfulness

### 3. Standardize on jsonrpc2 Library ⚠️ (In Progress)

- [x] Adopt `sourcegraph/jsonrpc2` as the core JSON-RPC library
- [x] Create adapter for HTTP transport
- [x] Create adapter for stdio transport
- [ ] Remove vestigial custom JSON-RPC implementation in `internal/jsonrpc/jsonrpc_types.go`
- [ ] Ensure all code paths use `jsonrpc2` types directly
- [ ] Add integration tests for JSON-RPC communication

### 4. Documentation and Testing ⚠️ (Needs Work)

- [x] Document architectural decisions in `docs/decision_log.md`
- [x] Document error handling approach in `error_handling_guidelines.md`
- [ ] Ensure consistent file and function documentation across codebase
- [ ] Increase unit test coverage, particularly for core components
- [ ] Add integration tests for end-to-end protocol flow
- [ ] Create user-facing documentation for setup and configuration

## Next Steps

### 1. RTM API Integration

- [ ] Complete integration with Remember The Milk API
- [ ] Implement task resources and tools
- [ ] Add proper error handling for API failures
- [ ] Test with real RTM accounts

### 2. Claude Desktop Integration

- [ ] Test integration with Claude Desktop
- [ ] Verify proper handling of MCP protocol messages
- [ ] Ensure correct authentication flow with RTM
- [ ] Document setup process for users

### 3. Structured Logging Implementation

- [x] Implement structured logging with `slog`
- [x] Define log levels and categories
- [ ] Ensure consistent logging format across components
- [ ] Add contextual information to log entries
- [ ] Configure log level via configuration

### 4. Configuration System

- [ ] Complete `koanf`-based configuration system
- [ ] Implement configuration validation
- [ ] Support multiple configuration sources
- [ ] Document configuration options

## Implementation Strategy

Start small and validate each change incrementally:

1. Focus on completing the JSON-RPC standardization first
2. Add tests for the state machine implementation
3. Complete RTM API integration
4. Test with Claude Desktop
5. Improve documentation and user experience

For each component, follow this approach:

1. Write tests for the intended behavior
2. Implement the changes
3. Verify with real-world use cases
4. Document the implementation

## Expected Benefits

- **Simplified codebase** with fewer parallel implementations
- **Improved maintainability** through standardized patterns
- **Better error handling** with rich context for debugging
- **More robust state management** using a proven library
- **Faster development** by leveraging established libraries
