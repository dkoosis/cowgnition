# CowGnition Implementation Roadmap

## TOP PRIORITY: Debug connection with Claude Desktop

### Log Entries

- we seek to load the config file from known locations. Let's do some defensive coding around that, to warn the user if there are several config files, with one taking precedence. such an warning would surely have save me heartache in the course of my progamming career.

- review the log messages for clarify and usefullness

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

- [ ] Implement file-based configuration (YAML/TOML)
- [ ] Add validation for configuration values
- [ ] Support for environment variable overrides (expand current implementation)
- [ ] Create configuration documentation

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

* Structured Logging Initiative:
* - Research and select a structured logging library (e.g., zap, logrus)
* - Define a base JSON schema for log entries
* - Implement a log formatting utility
* - Refactor existing logging to use the utility and structured format
* - Design a mechanism for configuring log levels
* - Implement middleware for HTTP request logging (if applicable)
* - AI Coding Assistant Prompt:
* - "Implement structured logging using the [chosen library name] library.
* - Define a JSON schema with fields like timestamp (RFC3339), severity, message, component, and data.
* - Create a log formatting utility to ensure consistent output.
* - Refactor existing log calls to use this utility.
* - Add a mechanism to configure log levels.
* - If the server uses HTTP, implement middleware to log requests."
    // TODO: internal/mcp/server.go Error handling simplification needed - The current approach uses three error packages:
    // 1. Standard "errors" (for errors.Is/As)
    // 2. "github.com/cockroachdb/errors" (for stack traces and wrapping)
    // 3. Custom "cgerr" package (for domain-specific errors)
    // This creates import confusion and makes error handling inconsistent.
    // Consider consolidating when implementing improved logging.
