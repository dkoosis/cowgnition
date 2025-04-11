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

- [PENDING] Add schema checksum generation and verification
- [PENDING] Implement schema metadata caching to skip recompilation when unchanged
- [PARTIAL] Add compile-time metrics and logging (Durations logged, full metrics TBD) [cite: uploaded:cowgnition/internal/schema/validator.go]
- [COMPLETE] Update schema source configuration to prioritize official URL sources [cite: uploaded:cowgnition/internal/schema/validator.go]

#### Step 2: Outgoing Message Validation

- [COMPLETE] Add validation for outgoing responses [cite: uploaded:cowgnition/internal/middleware/validation.go]
- [COMPLETE] Create environment-specific validation modes (strict vs. performance) [cite: uploaded:cowgnition/internal/middleware/validation.go]
- [COMPLETE] Implement specific schema type selection based on message method [cite: uploaded:cowgnition/internal/middleware/validation.go]
- [COMPLETE] Add detailed logging for validation failures [cite: uploaded:cowgnition/internal/middleware/validation.go]

#### Step 3: Static Content Pre-validation

- [PENDING] Add startup validation for tool definitions (See Deferred Item below) [cite: uploaded:cowgnition/docs/TODO.md]
- [PENDING] Add startup validation for resource definitions (See Deferred Item below) [cite: uploaded:cowgnition/docs/TODO.md]
- [PENDING] Add startup validation for prompt definitions (See Deferred Item below) [cite: uploaded:cowgnition/docs/TODO.md]
- [PENDING] Implement early warning/failure for invalid definitions (See Deferred Item below) [cite: uploaded:cowgnition/docs/TODO.md]

#### Step 4: Validation Architecture Improvements

- [COMPLETE] Create helper functions to generate schema-compliant names [cite: uploaded:cowgnition/internal/schema/name_rules.go]
- [PENDING] Add schema versioning to track supported MCP versions
- [PENDING] Create comprehensive schema validation test suite (Basic tests exist, but not comprehensive suite) [cite: uploaded:cowgnition/internal/mcp/mcp_server_test.go]
- [PARTIAL] Add validation metrics and monitoring (Durations logged in debug, full metrics TBD) [cite: uploaded:cowgnition/internal/middleware/validation.go]

#### Step 5: Developer Experience Enhancements

- [PARTIAL] Improve error messages with actionable guidance (Messages exist, ongoing improvement) [cite: uploaded:cowgnition/internal/middleware/validation.go]
- [PARTIAL] Add debug mode for detailed validation feedback (Debug flag influences options) [cite: uploaded:cowgnition/cmd/server/server_runner.go, uploaded:cowgnition/internal/middleware/validation.go]
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
  - **Action:** Refactor the application based on the principles outlined in ADR 006 [cite: uploaded:cowgnition/docs/adr/006_modular_multi_service_support.md]. Define a standard `Service` interface. Move RTM logic into a dedicated package implementing this interface. Implement a service registry in the server runner/MCP server. Modify core MCP handlers (`internal/mcp/handlers_*.go`) to use the registry for dispatching calls (e.g., `tools/call`, `resources/read`) to the appropriate service implementation. _(Note: ADR 006 is currently Draft and may need refinement before full implementation)_.
- [ ] **Enhance Schema Loading Feedback:**
  - **Context:** Improve developer visibility during server startup regarding schema validation setup.
  - **Action:** Modify `internal/schema/validator.go` and its usage in `cmd/server/server_runner.go` to log which schema source (embedded, file path, URL) was successfully loaded, log compilation time more prominently, and potentially add a startup "sanity check" validation against a known-good sample message.
- [ ] **Add Richer Validation Error Details:**
  - **Context:** Provide more specific information when schema validation fails, aiding developers in fixing malformed messages (Ref: ADR 002).
  - **Action:** Enhance `convertValidationError` in `internal/schema/validator.go` to extract more details from the underlying `jsonschema.ValidationError` (e.g., expected type/format) and add this information to the `data` field of the resulting JSON-RPC error response and server logs.
- [ ] **Implement "Dry Run" Validation CLI Command:**
  - **Context:** Allow developers to quickly check JSON message validity without running the full server.
  - **Action:** Add a new subcommand to `cmd/main.go` (e.g., `validate-message`) that takes a file path argument, reads the JSON content, initializes the `SchemaValidator`, and runs validation, printing success or detailed errors.

**Goal:** Achieve a basic "hello world" MCP conversation between CowGnition and Claude Desktop.

make the implementation rock-solid, high quality enough to server a reference implemenation for MCP servers in GO.

**Key Actions:**

## Phase 5: Testing Framework

**Status:** [INCOMPLETE]

- [PENDING] Create comprehensive test suite:
  - [PENDING] Unit tests for components
  - [PENDING] Schema compliance tests
  - [PARTIAL] Integration tests using `net.Pipe` (In-memory transport test exists) [cite: uploaded:cowgnition/internal/mcp/mcp_server_test.go]
  - [PENDING] Fuzzing tests for robustness
  - [PENDING] Benchmark tests for performance

## Phase 6: Observability

**Status:** [PARTIALLY COMPLETE]

- [PENDING] Include connection ID and request ID in all logs (Some request IDs logged, not all)
- [ ] Add metrics:
  - [PENDING] Request counts and latencies
  - [PENDING] Error rates by type
  - [PENDING] Active connections
  - [PENDING] Schema validation failures
  - [PENDING] Create exportable metrics

## Phase 7: Security & Robustness

**Status:** [NEW PHASE]

- [PENDING] Add comprehensive input validation beyond schema validation
- [PENDING] Implement rate limiting for API calls
- [PENDING] Add secure token storage for RTM API credentials
- [PENDING] Implement proper error sanitization to avoid leaking sensitive information
- [PENDING] Add telemetry for security events

## Static Capability Pre-validation (Deferred from Phase 8, Step 3)

**Status:** [PENDING]
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

**Status:** [PENDING]
Goal: Ensure the ValidationMiddleware uses the exact, most specific schema definition (from schema.json) when validating outgoing server responses, rather than relying on heuristics like appending \_response.
Problem: The current middleware receives the marshalled response bytes ([]byte) without knowing precisely which schema definition it should conform to (e.g., ListToolsResult vs InitializeResult vs generic SuccessResponse). The determineSchemaType function guesses based on naming conventions [cite: uploaded:cowgnition/internal/middleware/validation.go].
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
