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

## Phase 0: HIGHEST PRIORITY IMMEDIATE FOCUS

**Goal:** Achieve a basic "hello world" MCP conversation between CowGnition and Claude Desktop.

**Key Actions:**

1.  **[PENDING]** Integrate Validation Middleware Chain (`internal/middleware/chain.go`, `internal/middleware/validation.go`) into the main server loop (`internal/mcp/mcp_server.go`).
2.  **[PENDING]** Implement actual RTM Tool logic for `tools/list` and `tools/call`.
3.  **[PENDING]** Implement actual RTM Resource logic for `resources/list` and `resources/read`.
4.  **[PENDING]** Enhance logging to include connection/request IDs.

---

## Phase 1: Core Transport

**Status:** [COMPLETE]

- [x] Create NDJSON transport layer (`internal/transport/transport.go`)
- [x] Implement message reading with bounded buffer size (`MaxMessageSize`)
- [x] Add strict validation of JSON-RPC 2.0 message format (`ValidateMessage` in `transport.go`)
- [x] Support graceful error handling for malformed messages (`transport_errors.go`, middleware error responses)
- [x] Implement robust connection lifecycle management (`Close` method, EOF handling)
- [x] Create clean separation between transport and message handling (Separate packages)

---

## Phase 2: Schema Validation

**Status:** [PARTIALLY COMPLETE]

- [x] Create schema validator (`internal/schema/validator.go`)
- [x] Implement schema loading from multiple sources
- [x] Create validation middleware (`internal/middleware/validation.go`)
- [x] Generate detailed validation errors with context (`schema.ValidationError`)
- [x] Pre-compile schemas for performance (`Initialize` method)
- [ ] **[INCOMPLETE]** Integrate Validation Middleware into the server's main processing loop (`internal/mcp/mcp_server.go`)

---

## Phase 3: Request Router & Handler Framework

**Status:** [PARTIALLY COMPLETE]

- [x] Implement request router (map-based in `mcp_server.go`)
- [x] Create method registration framework (`registerMethods` in `mcp_server.go`)
- [x] Implement middleware support (`internal/middleware/chain.go`)
- [x] Add context propagation throughout handlers
- [x] Ensure proper error mapping to JSON-RPC format (`createErrorResponse` in `mcp_server.go`)
- [ ] **[INCOMPLETE]** Integrate Middleware Chain (including Validation) into the server's main processing loop (`internal/mcp/mcp_server.go`)

---

## Phase 4: Core MCP Method Implementations

**Status:** [PARTIALLY COMPLETE]

- [x] Implement protocol methods:
  - [x] `initialize`
  - [ ] `resources/list` - **[INCOMPLETE - Placeholder]** (Needs RTM logic)
  - [ ] `resources/read` - **[INCOMPLETE - Placeholder]** (Needs RTM logic)
  - [x] `tools/list` - **[PARTIALLY COMPLETE - Placeholder]** (Needs RTM tools)
  - [x] `tools/call` - **[PARTIALLY COMPLETE - Placeholder]** (Needs RTM tool logic)
  - [x] `ping`
  - [x] `shutdown`
- [ ] **[INCOMPLETE]** Implement RTM-specific resource/tool handlers.

---

## Phase 5: Testing Framework

**Status:** [INCOMPLETE]

- [ ] Create comprehensive test suite:
  - [ ] Unit tests for components
  - [ ] Schema compliance tests
  - [ ] Integration tests using `net.Pipe`
  - [ ] Fuzzing tests for robustness
  - [ ] Benchmark tests for performance

---

## Phase 6: Observability

**Status:** [PARTIALLY COMPLETE]

- [x] Add structured logging (`internal/logging/`):
  - [x] Connection lifecycle events
  - [x] Request/response details
  - [x] Validation failures
  - [x] Error handling
  - [ ] **[INCOMPLETE]** Include connection ID and request ID in all logs
- [ ] Add metrics:
  - [ ] Request counts and latencies
  - [ ] Error rates by type
  - [ ] Active connections
  - [ ] Schema validation failures
  - [ ] **[INCOMPLETE]** Create exportable metrics

---

This updated section provides a clearer view of what's done and what remains. Ready for the next step!

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
