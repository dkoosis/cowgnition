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

## Phase 8: Schema Validation Improvements

**Status:** [ACTIVE]

### Background

The current validation architecture primarily focuses on incoming messages but lacks comprehensive validation for outgoing responses. Recent client validation revealed tool naming pattern issues that our server-side validation didn't catch. We need to enhance our validation approach to ensure complete MCP specification compliance.

### Objectives

- Improve schema validation coverage to include outgoing messages
- Optimize schema compilation performance for faster startup
- Add early validation for static content (tools, resources)
- Establish metrics to measure validation performance
- Enable configurable validation approaches for different environments

### Implementation Steps

#### Step 1: Schema Caching & Performance Optimization

- [ ] Add schema checksum generation and verification
- [ ] Implement schema metadata caching to skip recompilation when unchanged
- [ ] Add compile-time metrics and logging
- [ ] Update schema source configuration to prioritize official URL sources

#### Step 2: Outgoing Message Validation

- [ ] Add validation for outgoing responses
- [ ] Create environment-specific validation modes (strict vs. performance)
- [ ] Implement specific schema type selection based on message method
- [ ] Add detailed logging for validation failures

#### Step 3: Static Content Pre-validation

- [ ] Add startup validation for tool definitions
- [ ] Add startup validation for resource definitions
- [ ] Add startup validation for prompt definitions
- [ ] Implement early warning/failure for invalid definitions

#### Step 4: Validation Architecture Improvements

- [ ] Create helper functions to generate schema-compliant names
- [ ] Add schema versioning to track supported MCP versions
- [ ] Create comprehensive schema validation test suite
- [ ] Add validation metrics and monitoring

#### Step 5: Developer Experience Enhancements

- [ ] Improve error messages with actionable guidance
- [ ] Add debug mode for detailed validation feedback
- [ ] Create documentation with common MCP patterns and constraints
- [ ] Implement automated compliance checking in CI pipeline

**Goal:** Achieve a basic "hello world" MCP conversation between CowGnition and Claude Desktop.

make the implementation rock-solid, high quality enough to server a reference implemenation for MCP servers in GO.

**Key Actions:**

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

## Static Capability Pre-validation (Deferred from Phase 8, Step 3)

Goal: Ensure the server's own definitions for the Tools, Resources, and Prompts it exposes are compliant with the official MCP schema (internal/schema/schema.json) before the server starts accepting connections.
Problem: Currently, definitions (like tools in internal/mcp/handlers_tools.go) are used at runtime (e.g., responding to tools/list) but are not validated against the loaded MCP schema during startup. Bugs or non-compliance in these definitions are caught late or might cause client issues.
Proposed Solution (Deferred):
During server initialization (e.g., in cmd/server/http_server.go), after loading the MCP schema via internal/schema/validator.go.
Iterate through the server's defined capabilities (e.g., the definedTools, definedResources, and any future definedPrompts lists).
For each definition, marshal it to JSON and use validator.Validate() with the appropriate schema type (mcp.SchemaTypeTool, mcp.SchemaTypeResource, etc.) from the official MCP schema.
If any definition fails validation, log a critical error and prevent the server from starting.
Future Consideration (Also Deferred): For supporting multiple services, these capability definitions might eventually be loaded from service-specific descriptions (like OpenAPI schemas) rather than being hardcoded in Go. The pre-validation step would still be crucial to ensure these loaded definitions comply with the MCP schema structure for Tools/Resources/Prompts.
Reason for Deferral: Focus first on core request/response handling and basic validation architecture before adding startup validation or multi-service support.

## Explicit Schema Naming for Outgoing Validation (Deferred Refinement of Phase 8, Step 2)

Goal: Ensure the ValidationMiddleware uses the exact, most specific schema definition (from schema.json) when validating outgoing server responses, rather than relying on heuristics like appending \_response.
Problem: The current middleware receives the marshalled response bytes ([]byte) without knowing precisely which schema definition it should conform to (e.g., ListToolsResult vs InitializeResult vs generic SuccessResponse). The determineSchemaType function guesses based on naming conventions.
Proposed Solution (Deferred):
Modify the architecture to allow the component generating the response (e.g., handleToolsList) to explicitly specify the schema definition name alongside the response data.
This would likely involve changing the transport.MessageHandler signature and middleware interfaces (internal/middleware/chain.go) to pass a custom struct (e.g., { Data []byte; SchemaType string }) instead of just []byte.
Update all MCP handlers and error helpers to return this struct, specifying the correct SchemaType (e.g., mcp.SchemaTypeTool + "\_response").
The ValidationMiddleware would then use this explicitly provided SchemaType for outgoing validation, eliminating guesswork.
The server loop (internal/mcp/mcp_server.go) would extract the Data field for sending over the transport.
Reason for Deferral: This involves significant refactoring across handlers, middleware, and the server loop. Decided to proceed with other tasks first and rely on the existing (less precise) outgoing validation heuristic for now. Static pre-validation (Item #1 above), once implemented, would mitigate some risks by ensuring the Go structs themselves match the schema.

---

## Completed Work

Key implementation details:

- Use structured logging with consistent context
- Include connection ID and request ID in all logs
- Create exportable metrics
