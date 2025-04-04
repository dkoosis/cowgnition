# CowGnition Implementation Roadmap

## Top Priority

# CowGnition MCP Implementation Roadmap: Urgent Fixes

## Priority Tasks for Codebase Cleanup and Standardization

### 2. Complete State Machine Implementation

- [ ] Finalize the integration of `qmuntal/stateless` for connection management
- [ ] Eliminate redundant state tracking mechanisms outside of the state machine
- [ ] Ensure all state transitions are properly handled by the state machine
- [ ] Verify event handlers are correctly registered and functioning
- [ ] Remove any vestigial code from the previous implementation approach
- [ ] Write state transition tests to validate the new architecture

### 3. Standardize Error Handling

- [ ] Audit and document our chosen error handling patterns
- [ ] Consolidate on `cockroachdb/errors` package for rich error context
- [ ] Create consistent helpers for converting between domain errors and protocol errors
- [ ] Establish clear error category boundaries (domain vs. protocol vs. transport)
- [ ] Ensure proper error propagation across boundaries
- [ ] Implement consistent error logging with appropriate detail levels
- [ ] Add unit tests for error handling scenarios

### 4. Standardize on jsonrpc2 Library

- [ ] Remove vestigial custom JSON-RPC implementation code from `internal/jsonrpc/types.go`
- [ ] Ensure all JSON-RPC related code uses `sourcegraph/jsonrpc2` types directly
- [ ] Create clean adapter layers where needed (e.g., for error conversion)
- [ ] Update any remaining code that refers to the custom types
- [ ] Document the standard pattern for JSON-RPC interactions for future development

### 5. Documentation and Testing

- [ ] Document architectural decisions in `docs/decision_log.md`
- [ ] Update code comments to reflect the new patterns
- [ ] Create integration tests for the entire protocol flow
- [ ] Set up CI checks to prevent regressions

## Implementation Strategy

Start small and validate each change incrementally:

1. First fix the build errors to get a working baseline
2. Focus on the state machine implementation next
3. Standardize error handling and validate with tests
4. Finally, clean up any remaining custom JSON-RPC code

For each component, follow this approach:

1. Analyze the current implementation
2. Define the target architecture
3. Implement incremental changes with tests
4. Validate with real-world use cases
5. Document the patterns for future development

## Expected Benefits

- **Simplified codebase** with fewer parallel implementations
- **Improved maintainability** through standardized patterns
- **Better error handling** with rich context for debugging
- **More robust state management** using a proven library
- **Faster development** by leveraging established libraries

## Technical Details

For the `ToJSONRPCError` function implementation and other specifics, refer to the error logs and code snippets in our GitHub issues and PR discussions.

## Next PRIORITY: Implement State Machine Architecture for MCP Connection Handling

### (may have been completed, see list above)

1. Defining a ConnectionManager struct with explicit connection states (Unconnected, Initializing, Connected, Terminating, Error)
2. Implementing state transitions with appropriate validation
3. Creating a message dispatcher that routes messages based on current state and message method
4. Integrating structured logging throughout with connection and request IDs
5. Implementing proper error handling that distinguishes between protocol and system errors
6. Ensuring the transport layer maintains persistent connections throughout the state lifecycle"

Start by designing the core state machine interfaces and structs, then implement the state transition logic, followed by the message dispatch system. Each implementation should include comprehensive logging, error handling, and follow Go best practices.

### Log Entries

- review the log messages for clarity and usefullness
- review the handling of credentials for RTM and put in place strong defensive coding

## 1. Core JSON-RPC Implementation

- [ ] Create message dispatcher:
  - [ ] Notification handling

## 2. MCP Protocol Compliance

- [ ] Update MCP server to use JSON-RPC core:

  - [ ] Add validation for MCP-specific message formats

- [ ] Implement transport layer:

  - [ ] Add SSE/HTTP transport support (progress made: timeout handling implemented)

- [ ] Update initialization flow:
  - [ ] Implement proper capability negotiation
  - [ ] Add protocol version validation
  - [ ] Reference: [MCP Lifecycle](https://spec.modelcontextprotocol.io/specification/2024-11-05/basic/lifecycle/)

## 3. RTM API Integration

- [ ] Complete RTM Task Resources:

  - [ ] Implement list resources for viewing tasks
  - [ ] Add resources for searching tasks
  - [ ] Implement tag resources
  - [ ] Reference: [RTM API Methods](https://www.rememberthemilk.com/services/api/)

- [ ] Implement RTM Tools:
  - [ ] Task creation tool
  - [ ] Task completion tool
  - [ ] Task update tool (due dates, priority)
  - [ ] Tag management tool

## 4. Configuration Enhancements

- [ ] Implement koanf-based configuration system with the following features:

  - [ ] Clear configuration hierarchy (defaults → files → env vars → flags)
  - [ ] Multiple search paths with precedence rules
  - [ ] Secure handling of sensitive information
  - [ ] Strong validation with helpful error messages
  - [ ] Documentation for users and developers
  - [ ] we seek to load the config file from known locations. Let's do some defensive coding around that, to warn the user if there are several config files, with one taking precedence. such an warning would surely have save me heartache in the course of my progamming career.

  Implementation plan:

  1. Add koanf dependency to go.mod
  2. Create new package `internal/kconfig` to replace existing config
  3. Define configuration structure with appropriate types
  4. Implement file discovery with explicit search path order
  5. Add environment variable overrides with clear naming conventions
  6. Add validation logic for all configuration values
  7. Implement secure credential handling with masking in logs
  8. Create config file generator for new users
  9. Add helper functions for common config operations
  10. Update all existing code to use the new package
  11. Write comprehensive tests for the new configuration system

  Reference:

  - Koanf GitHub: https://github.com/knadh/koanf

## 5. Testing & Quality Assurance

- [ ] Add comprehensive tests:
  - [ ] Unit tests for all packages
  - [ ] Integration tests for MCP server
  - [ ] Test RTM API interaction (with mocks)
  - [ ] End-to-end tests with MCP Inspector

## 6. Security Enhancements

- [ ] API key and token management:

  - [ ] Implement secure token storage (improve current implementation)
  - [ ] Add token rotation support
  - [ ] Implement rate limiting
  - [ ] Add request validation

- [ ] Security auditing:
  - [ ] Audit dependencies
  - [ ] Review authentication flow
  - [ ] Validate input sanitization

## 7. Documentation & User Experience

- [ ] Documentation:
  - [ ] Add API documentation with OpenAPI/Swagger
  - [ ] Include examples for common operations
  - [ ] Document error codes and solutions
  - [ ] Create usage guides for client applications

## Implementation Strategy

1. Focus on one component at a time, getting it fully working before moving on
2. Start with core JSON-RPC implementation as the foundation
3. Build MCP protocol compliance on top of that foundation
4. Add RTM functionality incrementally
5. Use the MCP Inspector tool to test each component
6. Write tests for each component as we develop

## Testing Approach

- Use MCP Inspector for manual testing
- Write unit tests for each package
- Implement integration tests for end-to-end validation
- Test with real Claude Desktop integration
- Follow test-driven development where possible

## Structured Logging Initiative

- Research and select a structured logging library (e.g., zap, logrus)
- Define a base JSON schema for log entries
- Implement a log formatting utility
- Refactor existing logging to use the utility and structured format
- Design a mechanism for configuring log levels
- Implement middleware for HTTP request logging (if applicable)

## Error Handling Simplification

- [ ] Review and consolidate error handling approach:

  - [ ] Standardize on cockroachdb/errors for all error operations
  - [ ] Create consistent error wrapping patterns
  - [ ] Update error checking to use errors.Is consistently
  - [ ] Remove redundant error types where possible
  - [ ] Ensure all errors include appropriate context

  // TODO: internal/mcp/server.go Error handling simplification needed - The current approach uses three error packages:
  // 1. Standard "errors" (for errors.Is/As)
  // 2. "github.com/cockroachdb/errors" (for stack traces and wrapping)
  // 3. Custom "cgerr" package (for domain-specific errors)
  // This creates import confusion and makes error handling inconsistent.

  ## MCP

  Message handling
  Request processing

Validate inputs thoroughly
Use type-safe schemas
Handle errors gracefully
Implement timeouts
Progress reporting

Use progress tokens for long operations
Report progress incrementally
Include total progress when known
Error management

Use appropriate error codes
Include helpful error messages
Clean up resources on errors
​
Security considerations
Transport security

Use TLS for remote connections
Validate connection origins
Implement authentication when needed
Message validation

Validate all incoming messages
Sanitize inputs
Check message size limits
Verify JSON-RPC format
Resource protection

Implement access controls
Validate resource paths
Monitor resource usage
Rate limit requests
Error handling

Don’t leak sensitive information
Log security-relevant errors
Implement proper cleanup
Handle DoS scenarios
​
Debugging and monitoring
Logging

Log protocol events
Track message flow
Monitor performance
Record errors
Diagnostics

Implement health checks
Monitor connection state
Track resource usage
Profile performance
Testing

Test different transports
Verify error handling
Check edge cases
Load test servers

## MCP

MCP Implementation Roadmap

1. Protocol Analysis & Design

Study the MCP specification in depth: Understand the exact message formats, field names, and protocol flow
Analyze official SDK implementations: Look at how TypeScript, Python, and other language SDKs structure their code
Examine Go implementations of similar protocols: Learn from LSP implementations in Go
Document architecture decisions: Create clear design guidelines before writing any code

2. JSON-RPC 2.0 Foundation

Implement a robust JSON-RPC 2.0 layer: This should handle message serialization, parsing, and validation
Define proper error handling patterns: Align with JSON-RPC 2.0 error codes and MCP-specific error codes
Create connection handling abstractions: Support multiple transport types (stdio, HTTP) cleanly

3. Protocol Types & Schema

Define precise Go structs for all MCP message types: Ensure exact field name matching with JSON tags
Implement validation for all message types: Check required fields and format constraints
Create clear separation between protocol types and business logic: Use interfaces where appropriate

4. Core Protocol Flow Implementation

Implement initialization flow: Handle protocol version negotiation and capabilities exchange
Create a capability negotiation system: Track what features are supported by clients/servers
Implement resource handling: Define interfaces for resource providers and consumers
Implement tool support: Create a framework for defining and executing tools

5. Testing Strategy

Create unit tests for all protocol components: Test serialization, validation, and error handling
Implement integration tests: Test the full protocol flow
Create mock clients/servers: For testing connection handling
Set up testing with real MCP clients: Test with Claude Desktop

6. Documentation & Examples

Document all types and interfaces: Provide clear usage examples
Create example servers: Demonstrate resource and tool implementation
Provide debugging guidelines: How to troubleshoot protocol issues

7. Performance & Robustness

Implement timeout handling: Ensure all operations have appropriate timeouts
Add observability: Structured logging, metrics, traces
Handle edge cases: Connection drops, protocol errors, message size limits

Would you like to start with any particular section of this roadmap? I think the most sensible approach would be to begin with the JSON-RPC 2.0 foundation, as this forms the basis for all MCP communication.

```

```
