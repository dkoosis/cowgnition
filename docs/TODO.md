# CowGnition MCP Implementation Roadmap: Clean Architecture Approach

**Status:** Active Development (with Interim Fixes Applied)

## Project Philosophy

This implementation will prioritize:

- Idiomatic Go code using the standard library where suitable
- Strict adherence to the MCP specification via schema validation (with known interim exceptions)
- Clear error handling and robust message processing
- Testability built into the design from the start
- Simple but maintainable architecture with clear separation of concerns

---

## ONGOING / RECENTLY ADDRESSED

### MCP Connection & Protocol Version Handling [Interim Fix Applied]

- **Issue**: Client (Claude Desktop) disconnects immediately after server's `initialize` response due to protocol version mismatch (`client=2024-11-05`, `server=2025-03-26`). Root cause traced to `schema.json` lacking a standard version identifier (`$id` or `title`), preventing reliable automatic detection by `internal/schema/validator.go`. See [MCP GitHub Issue #394](https://github.com/modelcontextprotocol/modelcontextprotocol/issues/394).
- **Interim Solution Implemented**:
  - `internal/mcp/handlers_core.go`: Modified `handleInitialize` to **force** reporting `protocolVersion: "2024-11-05"` in the response, bypassing schema detection for this field to ensure client compatibility.
  - Added explicit logging of client requested vs. server forced versions.
  - `internal/mcp/mcp_server.go`: Ensured `StrictOutgoing: false` is used in non-debug builds for validation middleware, allowing the known outgoing validation warning for the `initialize` response (due to schema/struct mismatch) to be logged without blocking the connection.
- **Status:** Connection should now establish. Requires testing.
- **Next Steps (Long Term):**
  - Advocate for adding version identifiers to the official `schema.json`.
  - Revert forced versioning in `handlers_core.go` once schema detection is reliable.
  - Investigate and fix the root cause of the outgoing schema validation warning for `initialize`.

### RTM Authentication Flow [Needs Verification]

- **Issue**: Previous reports indicated potential issues with the RTM auth flow completion (frob -> token).
- **Status**: Requires re-testing after recent changes, especially with `cmd/rtm_connection_test`. Ensure secure token storage (`internal/rtm/token_storage_secure.go`) works correctly.

### JSON-RPC Validation Test Fix [Needs Verification]

- **Issue**: Test `validation_identify_test.go:203` expectations might mismatch actual error messages.
- **Status**: Needs re-running tests (`make test`) to check if recent error handling changes resolved this.

---

## Phase 7: Security & Robustness

**Status:** [PARTIALLY ADDRESSED - NEW ITEM ADDED]

- [NEW] **Store RTM API Key/Secret Securely in OS Keychain:**
    - **Context:** Currently, `cowgnition setup` stores the RTM API Key/Secret in the `claude_desktop_config.json` `env` block, which is insecure if the host exposes environment variables. ADR 005 recommends using the OS keychain for *all* secrets.
    - **Action:**
        1. Modify `cowgnition setup` (`cmd/claude_desktop_registration.go`) to save the Key/Secret to the OS keychain (using `keyring` library) instead of writing them to the `env` block.
        2. Modify server startup (`internal/rtm/service.go` or `cmd/server/server_runner.go`) to load the Key/Secret *from* the OS keychain when initializing the RTM service/client.
        3. Deprecate/remove reliance on `RTM_API_KEY`/`RTM_SHARED_SECRET` environment variables set via `claude_desktop_config.json`.
    - **Reference:** [ADR 005](docs/adr/005_secret_management.md)
    - **Status:** `[PENDING]`
- [PENDING] Add comprehensive input validation beyond schema validation
- [PENDING] Implement rate limiting for RTM API calls
- [COMPLETE] Implement secure token storage for RTM *Auth Token* (using OS keychain/keyring via `internal/rtm/token_storage_secure.go`)
- [PENDING] Implement proper error sanitization to avoid leaking sensitive information
- [PENDING] Add telemetry for security events

---

## Phase 8: Schema Validation Improvements

**Status:** [PARTIALLY COMPLETE - INTERIM FIXES APPLIED]

### Background

Focuses on ensuring MCP compliance through robust schema validation. An interim fix is currently applied for protocol versioning due to limitations in the official schema file.

### Objectives

- Improve schema validation coverage (incoming/outgoing)
- Optimize schema compilation performance
- Establish metrics for validation performance
- Enable configurable validation modes

### Implementation Steps

#### Step 1: Schema Caching & Performance Optimization

- [PENDING] Add schema checksum generation and verification
- [PENDING] Implement schema metadata caching to skip recompilation when unchanged
- [COMPLETE] Add compile-time metrics and logging (Durations logged in debug)
- [PENDING] Update schema source configuration to prioritize official URL sources (Currently uses embedded or file URI override)

#### Step 2: Outgoing Message Validation

- [COMPLETE] Add validation for outgoing responses
- [COMPLETE] Create environment-specific validation modes (`StrictOutgoing`: `false` in normal mode, `true` in debug) - **Note:** Currently necessary to keep `false` in normal mode due to known `initialize` response warning.
- [COMPLETE] Implement specific schema type selection based on message method (with fallback logic)
- [COMPLETE] Add detailed logging for validation failures

#### Step 3: Static Content Pre-validation

- [DEFERRED] Add startup validation for tool definitions (See Deferred Item below)
- [DEFERRED] Add startup validation for resource definitions (See Deferred Item below)
- [DEFERRED] Add startup validation for prompt definitions (See Deferred Item below)
- [DEFERRED] Implement early warning/failure for invalid definitions (See Deferred Item below)

#### Step 4: Validation Architecture Improvements

- [COMPLETE] Create helper functions to generate schema-compliant names (`internal/schema/name_rules.go`)
- [PARTIAL] Add schema versioning detection (`internal/schema/validator.go` - detects from `$id`/`title` if present) - **Note:** Currently bypassed for `initialize` response via interim fix.
- [PENDING] Create comprehensive schema validation test suite (Basic tests exist)
- [PARTIAL] Add validation metrics and monitoring (Durations logged in debug, full metrics TBD)
- [PENDING] **Improve Schema Path Discovery:** Refactor logic to find local `schema.json` more robustly.
- [PENDING] **Make Schema URL Configurable:** Allow overriding schema source URL via config/env var.

#### Step 5: Developer Experience Enhancements

- [PARTIAL] Improve error messages with actionable guidance (Messages exist, ongoing improvement)
- [COMPLETE] Add debug mode for detailed validation feedback (Debug flag influences validation options)
- [PENDING] Create documentation with common MCP patterns and constraints
- [PENDING] Implement automated compliance checking in CI pipeline
- [PENDING] **Add Richer Validation Error Details:** Extract more detail (e.g., expected type) from `jsonschema.ValidationError` into JSON-RPC error data.
- [PENDING] **Implement "Dry Run" Validation CLI Command:** Add `validate-message <file>` command.

---

## Phase 9: Developer Experience & Extensibility

**Status:** [PENDING]

- [PENDING] **Document Schema Validation Implementation:** Create `docs/schema_validation_details.md`.
- [PENDING] **Improve Visibility of Validation Rules:** Document schema source config, add CLI flag to dump naming rules.
- [PENDING] **Add Optional Raw MCP Message Logging:** Implement `MCP_TRACE_LOG=file` option.
- [PENDING] **Enhance Error Diagnostics with Fix Suggestions:** Add `"suggestion"` context to key internal errors.
- [PENDING] **Implement Defensive Precondition Checks:** Add checks (auth, init state) before operations in handlers/services.
- [PENDING] **Adopt Modular Service Architecture for Extensibility:** Refactor based on ADR 006 (Draft).
- [PENDING] **Enhance Schema Loading Feedback:** Log loaded source, add startup sanity check.
- [PENDING] **Refactor `RunServer` Complexity:** Break down `cmd/server/server_runner.go:RunServer`.

---

## Phase 10: Feature Enhancements

**Status:** [PENDING]

- [PENDING] **Implement RTM Write Operations:** Add tools for `createTask`, `completeTask`, etc., including actual API calls in `internal/rtm/methods.go`.
- [PENDING] **Implement HTTP Transport:** Complete HTTP/SSE transport option.
- [PENDING] **Implement Resource Subscriptions:** Add actual subscribe/unsubscribe logic for `rtm://*` resources.

---

## Phase 5: Testing Framework

**Status:** [INCOMPLETE]

- [PENDING] Create comprehensive test suite:
  - [PENDING] Unit tests for components (Some exist, need more coverage)
  - [PENDING] Schema compliance tests
  - [COMPLETE] Integration tests using in-memory transport (`internal/mcp/mcp_server_test.go`)
  - [PENDING] Fuzzing tests for robustness
  - [PENDING] Benchmark tests for performance

---

## Phase 6: Observability

**Status:** [PARTIALLY COMPLETE]

- [PARTIAL] Include connection ID and request ID in logs (Some request IDs logged, not consistently everywhere)
- [PENDING] Add metrics:
  - [PENDING] Request counts and latencies
  - [PENDING] Error rates by type
  - [PENDING] Active connections
  - [PENDING] Schema validation failures
  - [PENDING] Create exportable metrics (e.g., Prometheus)

---

## Deferred Items

_(These items are important but deferred to focus on core functionality and stability)_

### Static Capability Pre-validation (Deferred from Phase 8)

**Status:** [DEFERRED]
**Goal:** Validate the server's own Tool/Resource/Prompt definitions against the loaded MCP schema at startup.
**Reason for Deferral:** Focus first on request/response handling and core validation.

### Explicit Schema Naming for Outgoing Validation (Deferred Refinement of Phase 8)

**Status:** [DEFERRED]
**Goal:** Have handlers explicitly specify the schema name for their responses, avoiding heuristics in the validation middleware.
**Reason for Deferral:** Requires significant refactoring of handler/middleware signatures. Relying on current heuristic and non-strict outgoing validation for now.

---

## Completed Work

_(Moved from previous phases)_

- Basic NDJSON Transport Implementation
- Initial MCP Handler Structure
- Basic RTM Client Scaffolding (Auth flow, GetLists)
- Initial Schema Validation Middleware (Incoming)
- Secure Token Storage for RTM Auth Token (Keychain/File)
- In-Memory Transport for Testing
- Addition of Outgoing Validation (with non-strict default)
- Forcing Protocol Version in `initialize` (Interim Fix)