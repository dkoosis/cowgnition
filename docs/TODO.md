# CowGnition MCP Implementation Roadmap: Clean Architecture Approach

**Status:** Active Development (with Interim Fixes Applied & Critical Issues Identified)

## Project Philosophy

This implementation will prioritize:

- Idiomatic Go code using the standard library where suitable
- Strict adherence to the MCP specification via schema validation (with known interim exceptions and required fixes)
- Clear error handling and robust message processing
- Testability built into the design from the start
- Simple but maintainable architecture with clear separation of concerns

---

## 🔥 Critical Issues / Immediate Fixes Needed 🔥

---

## ONGOING / RECENTLY ADDRESSED

### MCP Connection & Protocol Version Handling [Interim Fix Applied]

- **Issue**: Client (Claude Desktop) disconnects immediately after server's `initialize` response due to protocol version mismatch (`client=2024-11-05`, `server=2025-03-26`). Root cause traced to `schema.json` lacking a standard version identifier (`$id` or `title`), preventing reliable automatic detection by `internal/schema/validator.go`. See [MCP GitHub Issue #394](https://github.com/modelcontextprotocol/modelcontextprotocol/issues/394).
- **Interim Solution Implemented**:
  - `internal/mcp/handlers_core.go`: Modified `handleInitialize` to **force** reporting `protocolVersion: "2024-11-05"` in the response, bypassing schema detection for this field to ensure client compatibility.
  - Added explicit logging of client requested vs. server forced versions.
  - `internal/mcp/mcp_server.go`: Ensured `StrictOutgoing: false` is used in non-debug builds for validation middleware, allowing the known outgoing validation warning for the `initialize` response (due to schema/struct mismatch) to be logged without blocking the connection.
- **Status:** Connection should now establish *once critical fixes above are applied*. Requires testing.
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

**Status:** [PARTIALLY COMPLETE - ISSUES IDENTIFIED]

### Background

Focuses on ensuring MCP compliance through robust schema validation. Critical issues related to MCP spec compliance for error IDs and outgoing validation gaps have been identified.

### Objectives

- Improve schema validation coverage (incoming/outgoing)
- Ensure generated messages comply with the MCP specification (including error responses)
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
- [COMPLETE] Create environment-specific validation modes (`StrictOutgoing`: `false` in normal mode, `true` in debug) - **Note:** Currently necessary to keep `false` in normal mode due to known outgoing warnings.
- [COMPLETE] Implement specific schema type selection based on message method (with fallback logic)
- [COMPLETE] Add detailed logging for validation failures
- [PENDING] **Validate Internally Generated Error Responses:** Modify the error handling path (`handleProcessingError`) to pass generated error responses through the outgoing validation middleware before sending. This addresses the identified gap where server-generated errors currently bypass validation.
- [PENDING] **Investigate `list*` / `initialize` Outgoing Validation Warnings:** Determine why outgoing validation fails for `initialize` and `list*` responses even when the structure appears correct in logs/code. This might be a validator library issue or subtle schema mismatch.

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
  - [PENDING] **Protocol Compliance Tests:** Add specific tests verifying the server correctly handles the *entire* required MCP lifecycle sequence (`initialize`, `initialized`, `shutdown`, `exit`), including required notifications and error conditions, especially focusing on edge cases identified during debugging (e.g., `id: null` handling).
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

# TODO List for CowGnition RTM Issues

1. **Fix RTM Task Recurrence (`rrule`) Parsing Error**
    - **Issue:** The application fails with a JSON parsing error (`json: cannot unmarshal object into Go struct field ... rrule of type string`) when fetching RTM tasks. This occurs because the RTM API sometimes returns the `rrule` field as a JSON object (for complex recurrences) instead of the expected `string`.
    - **Location:** `internal/rtm/types.go`, specifically within the `rtmTaskSeries` struct definition.
    - **Action:** Modify the `RRule` field in the `rtmTaskSeries` struct. Change its type from `string` to something more flexible like `json.RawMessage` or `interface{}`. Update the task processing logic (likely within `GetTasks` in `internal/rtm/methods.go` or a helper function) to correctly handle both string and object types for the `rrule` data.
    - **Goal:** Prevent JSON parsing errors and correctly represent recurrence rules, regardless of their format in the RTM API response.

2. **Address Potential Large Task Volume ("Firehose") for `rtm://tasks`**
    - **Issue:** Fetching tasks via the default `rtm://tasks` resource might return a very large number of tasks, potentially overwhelming the client or being inefficient.
    - **Location:** Primarily affects the `ReadResource` method in `internal/rtm/service.go` when handling the `rtm://tasks` URI.
    - **Actions (Choose one or more):**
        - **(Recommended) Promote Filter Usage:** Ensure the existing filter parsing logic (`extractFilterFromURI` in `internal/rtm/service.go`) works correctly and encourage clients (like Claude Desktop) to use filtered URIs (e.g., `rtm://tasks?filter=status:incomplete`) for more targeted requests. This aligns with the RTM API's design.
        - **(Optional) Implement Server-Side Limiting:** Modify the `readTasksResourceWithFilter` function in `internal/rtm/service.go`. After fetching *all* tasks matching the (potentially empty) filter from RTM, add logic to truncate the list included in the final MCP response to a reasonable maximum (e.g., first 50-100 tasks), perhaps adding a note indicating that more tasks exist.
        - **(Optional/Advanced) Implement MCP Resource Pagination:** Define a custom pagination mechanism for the `rtm://tasks` resource (e.g., using `?cursor=` parameters). This would require significant changes to `ReadResource` and is not standard MCP.
    - **Goal:** Provide mechanisms to manage the volume of task data returned, improving performance and usability, primarily by leveraging RTM's filtering capabilities.

## Deferred Items

*(These items are important but deferred to focus on core functionality and stability)*

### Static Capability Pre-validation (Deferred from Phase 8)

**Status:** [DEFERRED]
**Goal:** Validate the server's own Tool/Resource/Prompt definitions against the loaded MCP schema at startup.
**Reason for Deferral:** Focus first on request/response handling and core validation.

### Explicit Schema Naming for Outgoing Validation (Deferred Refinement of Phase 8)

**Status:** [DEFERRED]
**Goal:** Have handlers explicitly specify the schema name for their responses, avoiding heuristics in the validation middleware.
**Reason for Deferral:** Requires significant refactoring of handler/middleware signatures. Relying on current heuristic and non-strict outgoing validation for now.

---

## Think about

# Integrating RTM Reflection APIs with Model Context Protocol

To integrate Remember The Milk's reflection APIs into a Model Context Protocol server, I would follow these steps:

## 1. Tool Discovery & Registration

Use RTM's reflection endpoints (`rtm.reflection.getMethodInfo.rtm` and related methods) to dynamically build tool definitions:

```javascript
async function buildToolsFromRTMReflection() {
  // Get all available methods
  const methods = await rtmClient.reflection.getMethods();
  
  // For each method, get detailed information
  const tools = await Promise.all(methods.map(async (method) => {
    const methodInfo = await rtmClient.reflection.getMethodInfo(method);
    
    // Transform RTM method info into MCP tool definition format
    return {
      name: method,
      description: methodInfo.description,
      parameters: transformRTMParamsToMCPSchema(methodInfo.parameters),
      returnType: transformRTMResponseToMCPSchema(methodInfo.response)
    };
  }));
  
  // Register tools with your MCP server
  registerToolsWithMCP(tools);
}
```

## 2. Tool Definition Enhancement

Improve tool definitions by adding examples and usage patterns:

```javascript
function enhanceToolDefinition(tool) {
  // Add example invocations
  tool.examples = generateExamplesForTool(tool);
  
  // Add error handling guidance
  tool.errorHandling = documentsErrorCasesForTool(tool);
  
  // Add typical usage scenarios
  tool.usageTips = generateUsageTipsForTool(tool);
  
  return tool;
}
```

## 3. Request/Response Handler

Create middleware to translate between MCP and RTM formats:

```javascript
async function handleMCPToolRequest(toolRequest) {
  // Extract RTM method name and parameters from MCP request
  const { method, params } = translateMCPRequestToRTM(toolRequest);
  
  // Call RTM API
  const rtmResponse = await rtmClient.callMethod(method, params);
  
  // Translate RTM response back to MCP format
  return translateRTMResponseToMCP(rtmResponse);
}
```

## 4. Context-Enhanced Invocation

Provide context when exposing tools to the LLM:

```javascript
function buildMCPToolContext(tools) {
  return {
    tools: tools,
    meta: {
      serviceDescription: "Remember The Milk task management API",
      bestPractices: [
        "Always authenticate before calling other methods",
        "Check for errors in the 'stat' field of responses",
        "Refresh authentication tokens when needed"
      ],
      commonWorkflows: [
        {
          description: "Creating and completing a task",
          steps: ["rtm.tasks.add", "rtm.tasks.complete"]
        }
      ]
    }
  };
}
```

## 5. Real-time Adaptation

Implement a feedback mechanism to improve tool usage:

```javascript
function handleToolUsageResult(result) {
  if (result.success) {
    // Record successful patterns
    learningSystem.recordSuccessPattern(result);
  } else {
    // Analyze failure and provide better guidance next time
    const improvedGuidance = errorAnalyzer.generateImprovedGuidance(result);
    toolDefinitions.updateWithImprovedGuidance(improvedGuidance);
  }
}
```

This approach leverages RTM's reflection capabilities to create dynamic, well-documented tools that can be effectively used by LLMs through the Model Context Protocol.

## Assess

Okay, let's look at your current tooling (.golangci.yml and Makefile) in the context of assessing simplification, decoupling, and opportunities for clarity, based on the goals outlined in your documents.

Your current setup is quite robust and already includes several tools that contribute significantly to these goals:

Core Linters: errcheck, govet, staticcheck, unused, ineffassign catch common errors, enforce best practices, and find dead code, all contributing to simplification and clarity.
Quality Linters: gosec (security), gocyclo (cyclomatic complexity), misspell (clarity), revive (style, naming, comments, complexity rules), unconvert, unparam aid simplification and quality.
Additional Linters: bodyclose, copyloopvar, dogsled, durationcheck, errorlint, godot, nilerr, thelper, tparallel address specific correctness and clarity issues.
Metrics: You have gocyclo enabled to measure cyclomatic complexity  and a check-line-length target in your Makefile.
Based on your desire to further assess Simplification, Decoupling, and Opportunities/Clarity, here are some additions you could consider:

1. Enable gocritic (in .golangci.yml)

Why: You currently have gocritic commented out. It offers a large number of valuable checks specifically aimed at simplification (e.g., ifElseChain, nestingReduce, valSwap), decoupling (e.g., exposedSyncMutex), and clarity (e.g., captLocal, commentFormatting, unnamedResult).
How: Uncomment - gocritic in the enable: list. Start with its default settings. If it proves too noisy initially, you can selectively disable specific checks within the linters-settings.gocritic block in .golangci.yml (e.g., disabled-checks: ["someCheck", "anotherCheck"]).
2. Enable depguard (in .golangci.yml)

Why: To enforce architectural boundaries and prevent unintended coupling between packages. This directly supports your decoupling goal, helping ensure modules remain independent as intended (e.g., preventing infrastructure code from depending directly on domain logic, or vice-versa).
How: Add - depguard to the enable: list. Configure allowed/denied dependencies in the linters-settings.depguard section based on your desired architecture.
3. Enable interfacebloat (in .golangci.yml)

Why: Detects large interfaces ("Monster Interfaces"). This aligns perfectly with the Interface Segregation Principle you identified as important for decoupling in your codebase analysis. Smaller interfaces promote better decoupling and testability.
How: Add - interfacebloat to the enable: list. You can configure the maximum number of methods allowed in an interface via linters-settings.interfacebloat.max-methods.
4. Enable maintidx (in .golangci.yml)

Why: Calculates the Maintainability Index, a composite metric reflecting complexity and code volume. While an indirect measure, tracking this metric over time can give you a quantitative view of whether your SDC efforts are improving overall maintainability.
How: Add - maintidx to the enable: list. Set a desired minimum index in linters-settings.maintidx.min-maintainability.
5. Add Dependency Visualization (in Makefile)

Why: Understanding package relationships is crucial for identifying coupling issues. Visualizing the dependency graph makes complex relationships much clearer than just reading import statements.
How: Add a new target to your Makefile. You'll need to install a tool like godepgraph  or gopkgview.
Example using godepgraph (requires Graphviz dot tool installed separately):
Makefile

# Requires: go install github.com/kisielk/godepgraph@latest

# Requires: graphviz (e.g., brew install graphviz or sudo apt-get install graphviz)

.PHONY: depgraph
depgraph: install-tools ## Generate dependency graph (requires godepgraph & graphviz)
 @printf "$(ICON_START) $(BOLD)$(BLUE)Generating dependency graph...$(NC)\n"
 @mkdir -p ./docs/diagrams
 @godepgraph -nostdlib -novendor . | dot -Tpng -o ./docs/diagrams/dependencies.png && \
     printf "   $(ICON_OK) $(GREEN)Dependency graph generated at ./docs/diagrams/dependencies.png$(NC)\n" || \
     (printf "   $(ICON_FAIL) $(RED)Failed to generate dependency graph (check godepgraph/dot installation)$(NC)\n" && exit 1)
 @printf "\n"
Update your help target to include depgraph.
  
Linters/Metrics Already Covered:

Cognitive Complexity: You have a revive rule for this (cognitive-complexity). You could switch to the dedicated gocognit linter  if you prefer its specific calculation or configuration, but revive likely covers it sufficiently.
Function Length: Also covered by a revive rule (function-length). The dedicated funlen linter  is an alternative if needed.
Recommendation:

Start with gocritic: It offers the broadest set of checks directly related to your goals. Enable it and see what it finds. Tune its configuration if necessary.
Add Dependency Visualization: The depgraph target will give you immediate visual feedback on coupling.
Consider depguard and interfacebloat next: These directly enforce decoupling principles.
Add maintidx later: Use it to track overall trends once you've addressed more specific issues.
Remember to introduce new linters incrementally to avoid overwhelming feedback. Integrate these checks into your regular workflow (like the all target) to continuously monitor simplification, decoupling, and clarity.

Sources and related content

## Completed Work

*(Moved from previous phases)*

- Basic NDJSON Transport Implementation
- Initial MCP Handler Structure
- Basic RTM Client Scaffolding (Auth flow, GetLists)
- Initial Schema Validation Middleware (Incoming)
- Secure Token Storage for RTM Auth Token (Keychain/File)
- In-Memory Transport for Testing
- Addition of Outgoing Validation (with non-strict default)
- Forcing Protocol Version in `initialize` (Interim Fix)
