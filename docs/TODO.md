# CowGnition MCP Implementation Roadmap: Clean Architecture Approach

**Status:** Active Development

## Project Philosophy

This implementation will prioritize:

- Idiomatic Go code using the standard library where suitable
- Strict adherence to the MCP specification via schema validation
- Clear error handling and robust message processing
- Testability built into the design from the start
- Simple but maintainable architecture with clear separation of concerns

---

## Phase 8: Schema Validation Improvements

**Status:** [ACTIVE - PARTIALLY COMPLETE]

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

- [PENDING] Add schema checksum generation and verification
- [PENDING] Implement schema metadata caching to skip recompilation when unchanged
- [COMPLETE] Add compile-time metrics and logging (Durations logged in debug) [cite: 1]
- [COMPLETE] Update schema source configuration to prioritize official URL sources [cite: 1]

#### Step 2: Outgoing Message Validation

- [COMPLETE] Add validation for outgoing responses [cite: 2]
- [COMPLETE] Create environment-specific validation modes (strict vs. performance) [cite: 2]
- [COMPLETE] Implement specific schema type selection based on message method [cite: 2]
- [COMPLETE] Add detailed logging for validation failures [cite: 2]

#### Step 3: Static Content Pre-validation

- [DEFERRED] Add startup validation for tool definitions (See Deferred Item below) [cite: 3]
- [DEFERRED] Add startup validation for resource definitions (See Deferred Item below) [cite: 3]
- [DEFERRED] Add startup validation for prompt definitions (See Deferred Item below) [cite: 3]
- [DEFERRED] Implement early warning/failure for invalid definitions (See Deferred Item below) [cite: 3]

#### Step 4: Validation Architecture Improvements

- [COMPLETE] Create helper functions to generate schema-compliant names
- [PENDING] Add schema versioning to track supported MCP versions
- [PENDING] Create comprehensive schema validation test suite (Basic tests exist, but not comprehensive suite)
- [PARTIAL] Add validation metrics and monitoring (Durations logged in debug, full metrics TBD) [cite: 2]

#### Step 5: Developer Experience Enhancements

- [PARTIAL] Improve error messages with actionable guidance (Messages exist, ongoing improvement) [cite: 2]
- [PARTIAL] Add debug mode for detailed validation feedback (Debug flag influences options) [cite: 2]
- [PENDING] Create documentation with common MCP patterns and constraints
- [PENDING] Implement automated compliance checking in CI pipeline

## Phase 9: Developer Experience & Extensibility

**Status:** [NEW PHASE]

- [ ] **Document Schema Validation Implementation:**
  - **Context:** Explain the _how_ and _why_ of the schema validation approach for better developer understanding (Ref: ADR 002).
  - **Action:** Create a new markdown file (e.g., `docs/schema_validation_details.md`) detailing the `SchemaValidator` component, the `ValidationMiddleware`, schema loading logic, error mapping, and relationship to ADR 002.
- [ ] **Improve Visibility of Validation Rules:**
  - **Context:** Make it easier for developers to see which schema and which naming rules are being enforced (Ref: ADR 002).
  - **Action:** Explicitly document the configuration options for the MCP schema source (file/URL). Expose the `DumpAllRules` function from `internal/schema/name_rules.go` via a CLI flag (e.g., `./cowgnition dump-naming-rules`) to show active naming constraints.
- [ ] **Add Optional Raw MCP Message Logging:**
  - **Context:** Provide a dedicated log for raw JSON-RPC messages to aid protocol-level debugging.
  - **Action:** Implement an optional logging mechanism (enabled by config/env var, e.g., `MCP_TRACE_LOG=mcp_trace.log`) that writes incoming/outgoing message bytes to a file. Add hooks in `internal/mcp/mcp_server.go` or the `Transport` layer.
- [ ] **Enhance Error Diagnostics with Fix Suggestions:**
  - **Context:** Improve developer experience by making errors more actionable (Ref: ADR 001).
  - **Action:** Identify common, diagnosable errors (e.g., `mcperrors.ErrRTMAuthFailure`). Modify the error creation logic (e.g., within `internal/rtm/`, `internal/mcp/mcp_errors/`) to add a specific `"suggestion"` key to the error's context map with a helpful hint (e.g., "Check RTM API Key/Secret"). Update `internal/mcp/mcp_server.go`'s `logErrorDetails` to display this suggestion in server logs.
- [ ] **Implement Defensive Precondition Checks:**
  - **Context:** Prevent errors proactively by adding checks before performing operations (Ref: ADR 001). This enhances robustness beyond schema validation (ADR 002).
  - **Action:** Review key handlers and service methods (e.g., RTM API call sites in `internal/rtm/`). Add checks for necessary preconditions (e.g., client initialized, user authenticated, required arguments non-nil). Return specific, diagnosable internal errors (e.g., `mcperrors.ErrRTMClientNotReady`) if preconditions fail, rather than letting the operation proceed and potentially cause a less specific downstream error.
- [ ] **Adopt Modular Service Architecture for Extensibility:**
  - **Context:** Make it easier for developers (including potential future contributors) to add support for new services beyond RTM (Ref: ADR 006 - Draft). The current structure makes adding services difficult.
  - **Action:** Refactor the application based on the principles outlined in ADR 006. Define a standard `Service` interface. Move RTM logic into a dedicated package implementing this interface. Implement a service registry in the server runner/MCP server. Modify core MCP handlers (`internal/mcp/handlers_*.go`) to use the registry for dispatching calls (e.g., `tools/call`, `resources/read`) to the appropriate service implementation. _(Note: ADR 006 is currently Draft and may need refinement before full implementation)_.
- [ ] **Enhance Schema Loading Feedback:**
  - **Context:** Improve developer visibility during server startup regarding schema validation setup.
  - **Action:** Modify `internal/schema/validator.go` and its usage in `cmd/server/server_runner.go` to log which schema source (embedded, file path, URL) was successfully loaded, log compilation time more prominently, and potentially add a startup "sanity check" validation against a known-good sample message[cite: 1].
- [ ] **Add Richer Validation Error Details:**
  - **Context:** Provide more specific information when schema validation fails, aiding developers in fixing malformed messages (Ref: ADR 002).
  - **Action:** Enhance `convertValidationError` in `internal/schema/validator.go` to extract more details from the underlying `jsonschema.ValidationError` (e.g., expected type/format) and add this information to the `data` field of the resulting JSON-RPC error response and server logs[cite: 1].
- [ ] **Implement "Dry Run" Validation CLI Command:**
  - **Context:** Allow developers to quickly check JSON message validity without running the full server.
  - **Action:** Add a new subcommand to `cmd/main.go` (e.g., `validate-message`) that takes a file path argument, reads the JSON content, initializes the `SchemaValidator`, and runs validation, printing success or detailed errors.

## Phase 5: Testing Framework

**Status:** [INCOMPLETE]

- [PENDING] Create comprehensive test suite:
  - [PENDING] Unit tests for components
  - [PENDING] Schema compliance tests
  - [PARTIAL] Integration tests using `net.Pipe` (In-memory transport test exists)
  - [PENDING] Fuzzing tests for robustness
  - [PENDING] Benchmark tests for performance

## Phase 6: Observability

**Status:** [PARTIALLY COMPLETE]

- [PARTIAL] Include connection ID and request ID in all logs (Some request IDs logged, not all)
- [PENDING] Add metrics:
  - [PENDING] Request counts and latencies
  - [PENDING] Error rates by type
  - [PENDING] Active connections
  - [PENDING] Schema validation failures
  - [PENDING] Create exportable metrics

## Phase 7: Security & Robustness

**Status:** [NEW PHASE]

- [PENDING] Add comprehensive input validation beyond schema validation
- [PENDING] Implement rate limiting for API calls
- [PENDING] Add secure token storage for RTM API credentials (See ADR 005)
- [PENDING] Implement proper error sanitization to avoid leaking sensitive information
- [PENDING] Add telemetry for security events

---

## Deferred Items

### Static Capability Pre-validation (Deferred from Phase 8, Step 3)

**Status:** [DEFERRED]
**Goal:** Ensure the server's own definitions for the Tools, Resources, and Prompts it exposes are compliant with the official MCP schema (`internal/schema/schema.json`) before the server starts accepting connections.
**Problem:** Currently, definitions (like tools in `internal/mcp/handlers_tools.go`) are used at runtime but are not validated against the loaded MCP schema during startup.
**Proposed Solution (Deferred):** During server initialization (e.g., in `cmd/server/http_server.go`), after loading the MCP schema via `internal/schema/validator.go`[cite: 1], iterate through the server's defined capabilities. For each definition, marshal it to JSON and use `validator.Validate()` with the appropriate schema type. If any definition fails validation, log a critical error and prevent the server from starting.
**Reason for Deferral:** Focus first on core request/response handling and basic validation architecture.

### Explicit Schema Naming for Outgoing Validation (Deferred Refinement of Phase 8, Step 2)

**Status:** [DEFERRED]
**Goal:** Ensure the `ValidationMiddleware` uses the exact, most specific schema definition (`schema.json`) when validating outgoing server responses, rather than relying on heuristics.
**Problem:** The current middleware receives marshalled response bytes without knowing precisely which schema definition it should conform to. The `determineSchemaType` function guesses based on naming conventions[cite: 2].
**Proposed Solution (Deferred):** Modify the architecture to allow the component generating the response (e.g., `handleToolsList`) to explicitly specify the schema definition name alongside the response data. This involves changing the `transport.MessageHandler` signature and middleware interfaces (`internal/middleware/chain.go`). Update handlers and error helpers to return the specific `SchemaType`. The `ValidationMiddleware` would then use this explicit type.
**Reason for Deferral:** Involves significant refactoring across handlers, middleware, and the server loop. Proceeding with other tasks first, relying on the existing heuristic and eventual static pre-validation.

---

## Completed Work

_(Consider moving fully completed Phase items here if the list gets too long)_

- Phase 8, Step 2: Outgoing Message Validation [cite: 2]
- Phase 8, Step 4: Helper functions for schema-compliant names
- Phase 8, Step 1: Partial compile-time metrics/logging & Schema source configuration [cite: 1]
