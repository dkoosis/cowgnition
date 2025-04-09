# 🧐 Semantic Naming & Conceptual Grouping Analysis

## 📊 Quantitative Summary

| Metric                         | Icon | Count |
| :----------------------------- | :--: | ----: |
| Primary Packages Analyzed      |  📦  |    11 |
| Ambiguous Names Flagged        |  🤔  |     4 |
| Generic Names Flagged          |  🏷️  |     9 |
| Low/Medium Cohesion Dirs       |  ⚠️  |     3 |
| Concepts w/ Inconsistent Terms |  ↔️  |     1 |

## ✨ Actionable Recommendations (Prioritized)

### 🔴 High Priority:

Rename cmd/server/http_server.go to something more accurate like runner.go or server_runner.go, as it handles the main server startup logic (defaulting to stdio) and not just HTTP. Rationale: The current name is misleading about the file's primary function and the server's default transport.

### 🟡 Medium Priority:

Rename cmd/setup.go to clarify its specific function of configuring Claude Desktop integration (e.g., claude_setup.go). Rationale: "setup" is ambiguous; the file has a specific integration purpose.

### 🟡 Medium Priority:

Consider moving generic helper functions from internal/mcp/helpers.go (like mustMarshalJSON) into more specific utility packages or directly into the files where they are primarily used, if feasible. Rationale: Reduces reliance on a generic "helpers" package, improving locality.

### 🟢 Low Priority:

If RTM transport logic is added to internal/rtm/transport/, ensure clear naming to distinguish it from internal/transport (which handles stdio/MCP transport). Rationale: Prevents potential naming ambiguity between different transport layers.

### 🟢 Low Priority:

Monitor the size and scope of internal/mcp. If it grows significantly complex, consider further separation (e.g., moving types.go to internal/mcp/types/). Rationale: Maintains package cohesion as the MCP implementation evolves.

## 🔬 Detailed Findings

### 📦 Package Conceptual Domains

cmd/: Application Entry Points (Contains specific commands like server and schema_test)
docs/: Documentation (Contains ADRs, assets, TODO, contributing guide, etc.)
internal/: Internal Application Logic (Core logic not intended for external use)
internal/config/: Configuration Management (Loading and handling application settings)
internal/logging/: Logging Infrastructure (Provides logging abstractions and implementations)
internal/mcp/: MCP Protocol Implementation (Core logic for handling MCP messages, includes handlers, types, server logic, etc.)
internal/mcp/mcp_errors/: MCP Specific Errors (Defines custom error types specific to MCP handling)
internal/middleware/: Request/Response Middleware (Processing steps like validation and chaining )
internal/rtm/: Remember The Milk (RTM) Integration (Placeholder for RTM logic, currently contains an empty transport/ dir )
internal/schema/: JSON Schema Handling (Loading, validating, and managing JSON schemas)
internal/transport/: Communication Transport Layer (Handles message sending/receiving via NDJSON, defines transport errors)
scripts/: Utility Scripts (Shell scripts for development tasks like checks and dependency listing)

### 🤔 Semantic Naming Issues

#### Ambiguous Names:

cmd/setup.go: Purpose (Claude Desktop config) isn't clear from the name alone.
internal/mcp/handler.go: Generic name for the file defining the core Handler struct.
internal/mcp/helpers.go: Generic name for utility functions (e.g., mustMarshalJSON).
internal/transport/transport.go vs internal/rtm/transport/: Potential for future ambiguity if RTM transport is implemented.
Generic Names (Mostly Acceptable Idioms):
internal/mcp/types.go: Acceptable within mcp package.
internal/mcp/mcp_errors/errors.go: Acceptable within mcp_errors package.
internal/transport/transport_errors.go: Acceptable within transport package.
internal/logging/logger.go, internal/logging/slog.go: Acceptable within logging package.
internal/config/config.go: Acceptable within config package.
internal/middleware/chain.go, internal/middleware/validation.go: Acceptable within middleware package.
internal/schema/validator.go: Acceptable within schema package.
cmd/main.go: Standard entry point name.
cmd/server/http_server.go: Name is misleading given its actual function (see recommendations).
(Misleading Names):
cmd/server/http_server.go: Misleading as it handles the main server runner logic (defaulting to stdio) rather than being exclusively about HTTP.

#### 🔗 Structural Cohesion & Consistency Issues

Directory Cohesion Assessment:

| Directory Path       | Primary Domain          | Detected Concepts Within                           | Cohesion Assessment                               |
| :------------------- | :---------------------- | :------------------------------------------------- | :------------------------------------------------ |
| cmd/                 | App Entry Points        | schema_test, server (runner), main, setup (claude) | Medium (Setup purpose unclear)                    |
| cmd/server/          | Server Runner           | http_server.go (misleading name)                   | Medium (Misleading name)                          |
| internal/config/     | Config Management       | config.go                                          | High                                              |
| internal/logging/    | Logging Infrastructure  | logger.go, slog.go                                 | High                                              |
| internal/mcp/        | MCP Protocol Impl.      | MCP handlers, types, helpers, server logic, errors | Medium (Mixes core logic, types, errors, helpers) |
| internal/middleware/ | Request/Response MW     | chain.go, validation.go                            | High                                              |
| internal/rtm/        | RTM Integration         | (Empty transport/ dir)                             | High (but Empty)                                  |
| internal/schema/     | JSON Schema Handling    | validator.go, schema.json, min_schema.json         | High                                              |
| internal/transport/  | Communication Transport | transport.go, transport_errors.go                  | High                                              |
| docs/adr/            | Architecture Decisions  | Specific ADR files                                 | High                                              |

### Terminology Inconsistencies:

Minor: "server" used for both the runner (cmd/server) and the MCP logic instance (internal/mcp/mcp_server.go). Context clarifies usage.
📝 Qualitative Summary
The project structure generally follows standard Go practices, separating command entry points (cmd/) from internal logic (internal/). Most internal sub-packages (config, logging, middleware, schema, transport) demonstrate high conceptual cohesion. The primary areas for improvement relate to clarifying ambiguous file names (cmd/setup.go, internal/mcp/handler.go, internal/mcp/helpers.go), correcting a misleading name (cmd/server/http_server.go), and managing the scope of the core internal/mcp package as it grows. Overall conceptual clarity based on naming and grouping is good, with specific points noted for refinement.

Timestamp: // ConceptualGroupingAssessmentVisual:2025-04-09
