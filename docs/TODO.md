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

1. JSON-RPC and Error Handling Cleanup

Status: In Progress (~60% complete)

✅ Basic integration with sourcegraph/jsonrpc2 library is in place
✅ Core error handling framework using cockroachdb/errors is established
⚠️ Need to remove custom JSON-RPC type definitions (jsonrpc_types.go not visible in shared files but mentioned in TODOs)
⚠️ Need to consolidate error codes between internal/mcp/errors/codes.go and internal/httputils/response.go
⚠️ Need to standardize error creation patterns across codebase (some places use direct errors.New/Wrap, others use helper functions)

2. State Machine Implementation

Status: Mostly Complete (~80% complete)

✅ Using qmuntal/stateless library for connection management
✅ States and transitions defined in internal/mcp/connection/state.go
✅ Manager implementation in internal/mcp/connection/manager.go
✅ Connection adapters implemented
❌ Missing comprehensive tests for state machine behavior
⚠️ Need to review error handling in state transitions
⚠️ Need to ensure proper resource cleanup in all states

3. Error Handling Standardization

Status: Mostly Complete (~75% complete)

✅ Well-defined error categories and codes
✅ Helper functions for error creation and conversion
✅ Comprehensive documentation in error_handling_guidelines.md
❌ Missing tests for error conversion and handling
⚠️ Need to review error messages for consistency and clarity
⚠️ Some places still use direct error creation instead of helpers

4. Documentation and Testing

Status: Needs Work (~40% complete)

✅ Architectural decisions documented in decision_log.md
✅ Error handling guidelines documented
⚠️ File and function documentation present but inconsistent
❌ Limited unit test coverage (only a few test files present)
❌ Missing integration tests for end-to-end protocol flow
❌ Missing user-facing documentation for setup and configuration

Next Steps 5. RTM API Integration

Status: Partially Implemented (~60% complete)

✅ Basic RTM API client implemented
✅ Authentication and token handling in place
✅ MCP provider for RTM authentication implemented
⚠️ Task resources implementation incomplete
⚠️ Task tools implementation incomplete
❌ Missing tests with real RTM accounts

6. Claude Desktop Integration

Status: Partially Implemented (~50% complete)

✅ Basic setup for integration implemented
✅ MCP protocol handling implemented
⚠️ Authentication flow needs testing
❌ Lacking end-to-end testing with Claude Desktop
❌ Missing comprehensive documentation for users

7. Structured Logging Implementation

Status: Mostly Complete (~80% complete)

✅ Implementation using slog in place
✅ Log levels and basic structure defined
✅ Logging integrated throughout the codebase
⚠️ Need to ensure consistent logging format across components
⚠️ Some log messages may need additional context

8. Configuration System

Status: Basic Implementation (~40% complete)

✅ Basic configuration structure implemented
❌ Not yet using koanf as mentioned in decision log
❌ Missing configuration validation
❌ Limited support for multiple configuration sources
❌ Missing documentation for configuration options

Suggested Next Actions

Complete the JSON-RPC standardization:

Remove custom JSON-RPC type definitions
Consolidate error code definitions
Standardize error handling patterns

Add tests for the state machine implementation:

Create comprehensive unit tests for state transitions
Test error handling pathways
Test resource cleanup

Enhance RTM integration:

Complete task resources implementation
Complete task tools implementation
Test with real RTM accounts

Improve documentation and testing coverage:

Standardize file and function documentation
Increase unit test coverage
Add integration tests for end-to-end flows
