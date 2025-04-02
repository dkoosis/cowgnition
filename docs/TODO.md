# CowGnition Implementation Roadmap

## TOP PRIORITY: Debug connection with Claude Desktop

These are errors.

### MCP.LOG

```
2025-04-02T00:45:31.246Z [info] [axe-handle] Initializing server...
2025-04-02T00:45:31.254Z [info] [axe-handle] Server started and connected successfully
2025-04-02T00:45:31.255Z [info] [axe-handle] Message from client: {"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"claude-ai","version":"0.1.0"}},"jsonrpc":"2.0","id":0}
2025-04-02T00:45:31.744Z [info] [axe-handle] Initializing server...
2025-04-02T00:45:31.749Z [info] [axe-handle] Server started and connected successfully
2025-04-02T00:45:31.749Z [info] [axe-handle] Message from client: {"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"claude-ai","version":"0.1.0"}},"jsonrpc":"2.0","id":0}
2025-04-02T00:46:31.259Z [info] [axe-handle] Message from client: {"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":0,"reason":"Error: MCP error -32001: Request timed out"}}
2025-04-02T00:46:31.260Z [info] [axe-handle] Client transport closed
2025-04-02T00:46:31.261Z [info] [axe-handle] Server transport closed
```

### MCP-SERVER-COWGNITIOPN.LOG

```
2025-04-02T01:33:38.587Z [cowgnition] [info] Initializing server...
2025-04-02T01:33:38.600Z [cowgnition] [info] Server started and connected successfully
2025-04-02T01:33:38.600Z [cowgnition] [info] Message from client: {"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"claude-ai","version":"0.1.0"}},"jsonrpc":"2.0","id":0}
2025/04/01 21:33:38 Configuration loaded successfully
2025/04/01 21:33:38 Starting CowGnition MCP server with stdio transport
2025/04/01 21:33:38 Server.startStdio: starting MCP server with stdio transport (debug enabled)
2025/04/01 21:33:38 Starting stdio JSON-RPC server with timeouts (request: 30s, read: 2m0s, write: 30s)
2025/04/01 21:33:38 Connected stdio transport (using NewPlainObjectStream)
2025/04/01 21:33:38 Received initialize request with params: {"capabilities":{},"clientInfo":{"name":"claude-ai","version":"0.1.0"},"protocolVersion":"2024-11-05"}
2025/04/01 21:33:38 MCP initialization requested by client: claude-ai (version: 0.1.0)
2025/04/01 21:33:38 Client protocol version: 2024-11-05
2025/04/01 21:33:38 Sending initialize response: {ServerInfo:{Name:cowgnition Version:1.0.0} Capabilities:map[resources:map[list:true read:true] tools:map[call:true list:true]] ProtocolVersion:2024-11-05}
2025-04-02T01:33:38.664Z [cowgnition] [info] Message from server: {"jsonrpc":"2.0","id":0,"result":{"server_info":{"name":"cowgnition","version":"1.0.0"},"capabilities":{"resources":{"list":true,"read":true},"tools":{"call":true,"list":true}},"protocolVersion":"2024-11-05"}}
2025-04-02T01:33:38.664Z [cowgnition] [info] Client transport closed
2025-04-02T01:33:38.665Z [cowgnition] [info] Server transport closed
2025-04-02T01:33:38.665Z [cowgnition] [info] Client transport closed
2025-04-02T01:33:38.665Z [cowgnition] [info] Server transport closed unexpectedly, this is likely due to the process exiting early. If you are developing this MCP server you can add output to stderr (i.e. `console.error('...')` in JavaScript, `print('...', file=sys.stderr)` in python) and it will appear in this log.
2025-04-02T01:33:38.665Z [cowgnition] [error] Server disconnected. For troubleshooting guidance, please visit our [debugging documentation](https://modelcontextprotocol.io/docs/tools/debugging) {"context":"connection"}
```

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
