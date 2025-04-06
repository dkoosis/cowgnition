# CowGnition MCP Implementation Roadmap: Clean Architecture Approach

## Project Philosophy

This implementation will prioritize:

- Idiomatic Go code using the standard library where suitable
- Strict adherence to the MCP specification via schema validation
- Clear error handling and robust message processing
- Testability built into the design from the start
- Simple but maintainable architecture with clear separation of concerns

## Implementation Plan

### Phase 1: Core Transport (HIGHEST PRIORITY)

- [ ] Create NDJSON transport layer:
  - [ ] Implement message reading with bounded buffer size
  - [ ] Add strict validation of JSON-RPC 2.0 message format
  - [ ] Support graceful error handling for malformed messages
  - [ ] Implement robust connection lifecycle management
  - [ ] Create clean separation between transport and message handling

Key implementation details:

- Use standard library `bufio` for efficient line reading
- Implement size limits to prevent memory attacks
- Close connections on receipt of malformed data
- Create an abstraction for message dispatch to handlers

### Phase 2: Schema Validation

- [ ] Create schema validator:
  - [ ] Implement schema loading from multiple sources
  - [ ] Create validation middleware for all messages
  - [ ] Generate detailed validation errors with context
  - [ ] Pre-compile schemas for performance

Key implementation details:

- Use the official MCP JSON schema
- Support fallbacks (URL → local file → embedded)
- Cache compiled schemas for performance
- Add clear context to validation errors

### Phase 3: Request Router & Handler Framework

- [ ] Implement request router:
  - [ ] Create method registration framework
  - [ ] Implement middleware support
  - [ ] Add context propagation throughout handlers
  - [ ] Ensure proper error mapping to JSON-RPC format

Key implementation details:

- Use a simple but structured router
- Support middleware for cross-cutting concerns
- Create a clean pattern for handler dependencies
- Ensure consistent error handling

### Phase 4: Core MCP Method Implementations

- [ ] Implement protocol methods:
  - [ ] `initialize` - Server initialization
  - [ ] `resources/list` - List available resources
  - [ ] `resources/read` - Read a specific resource
  - [ ] `tools/list` - List available tools
  - [ ] `tools/call` - Call a specific tool
  - [ ] `ping` - Connection check
  - [ ] `shutdown` - Graceful shutdown

Key implementation details:

- Ensure all methods conform exactly to the MCP spec
- Implement RTM-specific resource/tool handlers
- Add comprehensive error handling
- Test against schema validation

### Phase 5: Testing Framework

- [ ] Create comprehensive test suite:
  - [ ] Unit tests for components
  - [ ] Schema compliance tests
  - [ ] Integration tests using `net.Pipe`
  - [ ] Fuzzing tests for robustness
  - [ ] Benchmark tests for performance

Key implementation details:

- Build a library of test messages (valid and invalid)
- Test error paths thoroughly
- Create framework for testing handler implementations
- Use Go 1.18+ fuzzing capabilities

### Phase 6: Observability

- [ ] Add structured logging:

  - [ ] Connection lifecycle events
  - [ ] Request/response details
  - [ ] Validation failures
  - [ ] Error handling

- [ ] Add metrics:
  - [ ] Request counts and latencies
  - [ ] Error rates by type
  - [ ] Active connections
  - [ ] Schema validation failures

Key implementation details:

- Use structured logging with consistent context
- Include connection ID and request ID in all logs
- Create exportable metrics

## Guidelines for Implementation

1. **Start Fresh**: Create a new branch and implement the architecture from scratch.

2. **Structured Approach**: Build each component in order, ensuring a solid foundation.

3. **Error Handling**: Use standard Go error handling with proper context.

4. **Testing First**: Write tests alongside or before implementing functionality.

5. **Keep It Simple**: Avoid unnecessary abstractions or dependencies.

6. **Document As You Go**: Add clear comments explaining design decisions.

7. **Review Progress**: Periodically validate against the MCP specification.

## Recommended Implementation Order

1. Start with the transport layer basics - reading and writing NDJSON messages
2. Add schema validation framework next
3. Implement the router and basic handler structure
4. Add the `initialize` method as your first complete handler
5. Add remaining MCP methods in order of complexity
6. Integrate with RTM API for resource/tool implementations
7. Enhance with observability and metrics

This plan will guide us in creating a clean, maintainable implementation of the MCP protocol focused on correctness and robustness, while avoiding the issues present in the current codebase.
