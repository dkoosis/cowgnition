# CowGnition MCP Implementation Roadmap: Clean Architecture Approach

## Project Philosophy

This implementation will prioritize:

- Idiomatic Go code using the standard library where suitable
- Strict adherence to the MCP specification via schema validation
- Clear error handling and robust message processing
- Testability built into the design from the start
- Simple but maintainable architecture with clear separation of concerns

# CowGnition MCP Implementation Roadmap: Clean Architecture Approach

**Status:** Active Development

---

# CowGnition MCP Implementation Roadmap: Clean Architecture Approach

**Status:** Active Development

## Phase 0: HIGHEST PRIORITY IMMEDIATE FOCUS

**Goal:** Achieve a basic "hello world" MCP conversation between CowGnition and Claude Desktop.

**Key Actions:**

1. **[PARTIALLY COMPLETE]** Implement actual RTM Tool logic for `tools/list` and `tools/call`.

   - Basic structure is in place in `mcp_handlers.go` with enhanced placeholder implementations.
   - Need to implement actual RTM API client integration.

2. **[PENDING]** Implement actual RTM Resource logic for `resources/list` and `resources/read`.

   - Structure is in place but currently returns placeholders.
   - Need to implement actual RTM API client integration.

3. **[PENDING]** Enhance logging to include connection/request IDs.

   - Basic logging framework is in place.
   - Need to add connection ID and request ID context to all logs.

4. **[NEW]** Implement RTM API client for API interaction (`internal/rtm/client.go`).

   - This is a prerequisite for completing items 1 and 2 above.
   - Should handle auth, task operations, and resource retrieval.

5. **[NEW]** Create configuration flow for RTM API authentication.
   - Implement OAuth flow for RTM API.
   - Add secure token storage.

## Phase 4: Core MCP Method Implementations

**Status:** [PARTIALLY COMPLETE]

- [ ] **[PENDING]** Implement RTM-specific resource/tool handlers with actual API integration.

## Phase 5: Testing Framework

**Status:** [INCOMPLETE]

- [ ] Create comprehensive test suite:
  - [ ] Unit tests for components
  - [ ] Schema compliance tests
  - [ ] Integration tests using `net.Pipe`
  - [ ] Fuzzing tests for robustness
  - [ ] Benchmark tests for performance

## Phase 6: Observability

**Status:** [PARTIALLY COMPLETE]

- [ ] **[PENDING]** Include connection ID and request ID in all logs
- [ ] Add metrics:
  - [ ] Request counts and latencies
  - [ ] Error rates by type
  - [ ] Active connections
  - [ ] Schema validation failures
  - [ ] Create exportable metrics

## Phase 7: Security & Robustness

**Status:** [NEW PHASE]

- [ ] Add comprehensive input validation beyond schema validation
- [ ] Implement rate limiting for API calls
- [ ] Add secure token storage for RTM API credentials
- [ ] Implement proper error sanitization to avoid leaking sensitive information
- [ ] Add telemetry for security events

---

## Completed Work

### Core Transport - [COMPLETE]

- [x] Create NDJSON transport layer (`internal/transport/transport.go`)
- [x] Implement message reading with bounded buffer size (`MaxMessageSize`)
- [x] Add strict validation of JSON-RPC 2.0 message format (`ValidateMessage` in `transport.go`)
- [x] Support graceful error handling for malformed messages (`transport_errors.go`, middleware error responses)
- [x] Implement robust connection lifecycle management (`Close` method, EOF handling)
- [x] Create clean separation between transport and message handling (Separate packages)

### Schema Validation - [COMPLETE]

- [x] Create schema validator (`internal/schema/validator.go`)
- [x] Implement schema loading from multiple sources
- [x] Create validation middleware (`internal/middleware/validation.go`)
- [x] Generate detailed validation errors with context (`schema.ValidationError`)
- [x] Pre-compile schemas for performance (`Initialize` method)
- [x] Integrate Validation Middleware into the server's main processing loop (`internal/mcp/mcp_server.go`)

### Request Router & Handler Framework - [COMPLETE]

- [x] Implement request router (map-based in `mcp_server.go`)
- [x] Create method registration framework (`registerMethods` in `mcp_server.go`)
- [x] Implement middleware support (`internal/middleware/chain.go`)
- [x] Add context propagation throughout handlers
- [x] Ensure proper error mapping to JSON-RPC format (`createErrorResponse` in `mcp_server.go`)
- [x] Integrate Middleware Chain (including Validation) into the server's main processing loop

### Core MCP Method Implementations - [PARTIALLY COMPLETE]

- [x] Implement protocol methods:
  - [x] `initialize`
  - [x] `resources/list` (Structure in place, needs actual RTM integration)
  - [x] `resources/read` (Structure in place, needs actual RTM integration)
  - [x] `tools/list` (Structure in place with enhanced placeholders)
  - [x] `tools/call` (Structure in place with enhanced placeholders)
  - [x] `ping`
  - [x] `shutdown`

### Observability - [PARTIALLY COMPLETE]

- [x] Add structured logging (`internal/logging/`):
  - [x] Connection lifecycle events
  - [x] Request/response details
  - [x] Validation failures
  - [x] Error handling

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
