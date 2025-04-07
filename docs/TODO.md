# CowGnition MCP Implementation Roadmap: Clean Architecture Approach

## Critically Assess Suggestions

File & Directory Naming Assessment Report
Project Context Summary
The CowGnition project is an MCP server that connects Remember The Milk tasks with Claude Desktop and other MCP clients, using JSON-RPC 2.0 and NDJSON over stdio or TCP [cite: cowgnition.zip/readme.md]. It focuses on integrating task management with AI assistants [cite: cowgnition.zip/readme.md].

Analysis Scope
Target Language: Go
Included: All files and directories in the provided folder.
Excluded: None explicitly specified.
Overall Summary Metrics
Total Items Analyzed (Files/Dirs): 35
Items Flagged for Improvement: 10
Generic: 4
Unclear/Ambiguous: 1
Inconsistent: 5
Misleading: 0
Items with Potential Name Clashes Identified
Files: errors.go, errors.go, transport.go
Directories: errors
Detailed Findings & Suggestions
Item: internal/mcp/errors/errors.go (Type: File)
Assessment: Acceptable
Justification: While clear within its directory, errors.go is a common name and could clash if the mcp directory were nested.
Suggestions:
Rename to: mcp_errors.go - Rationale: Adds mcp prefix to avoid potential clashes and increase specificity.
Item: internal/transport/errors.go (Type: File)
Assessment: Acceptable
Justification: Similar to the above, errors.go is generic and could clash if transport is nested.
Suggestions:
Rename to: transport_errors.go - Rationale: Adds transport prefix for clarity.
Item: internal/rtm/auth.txt (Type: File)
Assessment: Needs Improvement - Inconsistent
Justification: Inconsistent file naming. Go files use .go extension, not .txt . auth suggests content but lacks specificity.
Suggestions:
Rename to: auth.go - Rationale: Corrects file extension to .go to follow Go conventions.
Rename to: rtm_auth.go - Rationale: More specific, clarifies it's RTM auth logic.
Item: internal/rtm/client.txt (Type: File)
Assessment: Needs Improvement - Inconsistent
Justification: Same as above. Inconsistent file extension and lacks specificity.
Suggestions:
Rename to: client.go - Rationale: Corrects file extension to .go .
Rename to: rtm_client.go - Rationale: More specific, clarifies it's RTM client logic.
Item: internal/rtm/provider.txt (Type: File)
Assessment: Needs Improvement - Inconsistent
Justification: Same as above.
Suggestions:
Rename to: provider.go - Rationale: Corrects file extension to .go .
Rename to: rtm_provider.go - Rationale: More specific.
Item: internal/rtm/token.txt (Type: File)
Assessment: Needs Improvement - Inconsistent
Justification: Same as above.
Suggestions:
Rename to: token.go - Rationale: Corrects file extension to .go .
Rename to: rtm_token.go - Rationale: More specific.
Item: internal/transport/transport.go (Type: File)
Assessment: Acceptable
Justification: Generally clear, but could be more specific if other transport-related files exist.
Suggestions:
Rename to: ndjson_transport.go - Rationale: Clarifies the transport mechanism.
Item: scripts/check_file_length.sh (Type: File)
Assessment: Acceptable
Justification: Clear on its own.
Suggestions:
Rename to: check-file-length.sh - Rationale: Hyphens are preferred for CLI scripts.
Item: scripts/pkgdep.sh (Type: File)
Assessment: Needs Improvement - Generic
Justification: pkgdep is ambiguous.
Suggestions:
Rename to: list_internal_deps.sh - Rationale: More descriptive.
Item: cmd/main.go (Type: File)
Assessment: Needs Improvement - Generic
Justification: main.go is standard but not descriptive.
Suggestions:
Rename to: cowgnition.go - Rationale: More specific to the application.
// FileDirNamingAssessment:2025-04-07

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
