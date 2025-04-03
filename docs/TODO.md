# CowGnition Implementation Roadmap

## Top Priority

Quality review:
Key Observations:

Custom JSON-RPC Types: You have defined your own set of structs for handling JSON-RPC messages (jsonrpc.Message, jsonrpc.Request, jsonrpc.Response, jsonrpc.Notification) and errors (jsonrpc.Error) within the internal/jsonrpc package.
Different Error Struct: The jsonrpc.Error struct defined in this file (Code int, Message string, Data json.RawMessage) is different from the jsonrpc2.Error struct used by the sourcegraph/jsonrpc2 library (Code int64, Message string, Data interface{}).
manager.go Uses sourcegraph/jsonrpc2: Critically, your internal/mcp/connection/manager.go file (as shown in previous snippets and implied by the error messages) imports and uses types from github.com/sourcegraph/jsonrpc2, specifically jsonrpc2.Conn, jsonrpc2.Request, and it expects a \*jsonrpc2.Error for the conn.ReplyWithError method.
The Mix-up:

The error undefined: cgerr.ToJSONRPCError arises because manager.go needs a function in your errors package (cgerr) that converts your application's internal errors into the specific \*jsonrpc2.Error type required by the sourcegraph/jsonrpc2 library's conn.ReplyWithError method.

The existence of your internal/jsonrpc/types.go doesn't change what manager.go needs. The function ToJSONRPCError that I provided for your errors/utils.go was specifically designed to:

Take your application errors (which use embedded details via cockroachdb/errors).
Use your helper functions (GetErrorCode, UserFacingMessage, etc.) to extract the relevant information.
Construct and return a \*jsonrpc2.Error (from the sourcegraph/jsonrpc2 library) suitable for manager.go.
Conclusion & Fix:

ToJSONRPCError is Still Needed: The ToJSONRPCError function defined in the previous answer (the one designed for your errors/utils.go using your specific error helpers) is still the correct function needed to resolve the undefined: cgerr.ToJSONRPCError error in manager.go. Please ensure that function is correctly added to your internal/mcp/errors/utils.go file and that the file is saved.
jsonrpc.Error vs jsonrpc2.Error: Your internal/jsonrpc package defines its own error type. This is fine, but it's not the type needed by the conn.ReplyWithError method used in manager.go. The ToJSONRPCError function correctly produces the required *jsonrpc2.Error. You may want to review where else internal/jsonrpc/types.go is used in your project to ensure consistency, but it doesn't invalidate the fix needed for manager.go.
jsonrpc2.ErrNotConnected: The fix for the undefined: jsonrpc2.ErrNotConnected error in manager.go (removing that part of the if condition in sendNotification) also still applies, as that error variable is not exported by the sourcegraph/jsonrpc2 library.
In summary: Please ensure the ToJSONRPCError function (which returns *jsonrpc2.Error) is present and saved in internal/mcp/errors/utils.go, and apply the correction for jsonrpc2.ErrNotConnected in internal/mcp/connection/manager.go. This should resolve the current compilation errors.

## Next PRIORITY: Implement State Machine Architecture for MCP Connection Handling

**Implementation Prompt for AI Assistant:**
"Help me implement the State Machine-based Event Handler architecture for MCP connection handling. Focus on the following key components:

### complete - needs quality review

✅ Created a new connection package in the internal/mcp directory with files:

types.go: Defines types and interfaces
manager.go: Core connection manager implementation
handlers.go: Request handler implementations
state.go: Connection state definitions and validation
utils.go: Utility functions

✅ Split the implementation across multiple files in that package:

Each file has a focused responsibility
Code is organized for better maintainability

✅ Made the connection manager use the MCP types from the parent package:

Created proper interfaces to adapt to the existing types
Ensured type safety between packages

✅ Updated the server integration to reference the new package:

Created server_connection.go with ConnectionServer implementation
Updated cmd/server/server.go to use the new ConnectionServer

### requierd (may have been completed, see list above)

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
