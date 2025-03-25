Here's the revised decision log with the requested changes:

```markdown
# Decision Log

This document records significant architectural, design, and tooling decisions made for the CowGnition project. Its purpose is to provide context for these choices and prevent revisiting settled discussions.

## Architecture

### Layered Architecture

- Adopted a layered architecture separating concerns (Command, Server, Service, Client, Auth, Config layers).
- Improves maintainability through clear separation of responsibilities, enhances testability by allowing components to be tested in isolation, and increases code clarity by defining explicit boundaries.
- [1] The Clean Architecture by Robert C. Martin

## Folder Organization

### Standard Go Project Layout

- Adhering to the [Standard Go Project Layout](https://github.com/golang-standards/project-layout).
- Promotes consistency across Go projects, reduces onboarding time for new developers, and follows established patterns.
- **Observed Structure (Top Level)**:
```

cowgnition/
├── Makefile
├── README.md
├── cmd/
├── configs/
├── docs/
├── go.mod
├── go.sum
├── internal/
├── pkg/
├── scripts/
└── test/

```

### Test Directory Structure
* Organized the `test/` directory hierarchically by test type and domain.
* Scales naturally as project grows, supports multiple API integrations, and provides clear separation between test types.
* Flat structures or simply mirroring package structure were rejected as they wouldn't adequately separate test concerns.
* **Resulting Structure**:
```

test/
├── README.md
├── conformance/ # Protocol conformance tests
├── fixtures/ # Static test data/responses
├── helpers/ # Test helper functions
│ ├── common/ # Shared helpers
│ ├── mcp/ # MCP-specific helpers
│ └── rtm/ # RTM-specific helpers
├── integration/ # Integration tests
├── mocks/ # Mock implementations
├── unit/ # Unit tests
├── util/ # General test utilities
└── validators/ # Validation logic for tests

```

## Core Development Tooling

### Standard Toolset
* Standardized on Go, `golangci-lint`, `goimports`, `gotestsum`, and `staticcheck`.
* Provides consistent formatting, comprehensive linting, enhanced test output, and advanced static analysis, all leading to higher code quality and better developer experience.

## Additional Tooling

### Language Server
* Using `gopls` for all developers.
* Consistent code intelligence features (completion, navigation, refactoring) across all editors and IDEs.

### Debugger
* Using `dlv` (Delve).
* Robust debugging capabilities with support for breakpoints, variable inspection, and goroutine analysis.

### Mocking Framework
* Using `golang/mock/mockgen` exclusively for generating mocks.
* Consistent mocking approach with type safety and IDE integration.
* Manual mocks were rejected to reduce boilerplate and prevent inconsistent implementations.

### File Watcher
* Using `entr` for file watching.
* Simple, reliable file change detection for development workflows like auto-testing.

### Dependency Injection
* Using manual Dependency Injection.
* Simpler to understand and debug at our current project scale without introducing code generation complexity.
* `google/wire` was rejected to avoid the overhead of generated code and additional build complexity.

### CLI Framework
* `spf13/cobra` for CLI functionality when needed.
* Rich command-line interface support with subcommands, flags, and documentation generation.

### Configuration Management
* `spf13/viper` for advanced configuration needs.
* Unified configuration from multiple sources (files, environment, flags) with automatic binding.

### Logging Framework
* `uber-go/zap` for structured logging.
* High-performance, low-allocation logging with structured data support and level filtering.

## Testing Frameworks & Tooling

### Primary Testing Framework
* Using the standard Go `testing` package with table-driven tests.
* Follows idiomatic Go patterns, requires no external dependencies, and provides a simple yet powerful testing approach.
* BDD frameworks like `ginkgo`/`gomega` were rejected in favor of the more straightforward standard library approach.

### Test Execution & Reporting
* Using `gotestsum` for test execution and reporting.
* More readable test output, better CI integration via JUnit XML, and watch mode for development.
* [2] gotestsum documentation

## Linting

### Linter Tool
* Using `golangci-lint`.
* Single tool that aggregates many specialized linters, with unified configuration and faster execution.

### Core Enabled Linters
* Enabled set includes `govet`, `staticcheck`, `gosec`, `errcheck`, `ineffassign`, and `gocritic`.
* Comprehensive coverage of potential issues from correctness and security to style and performance.

### Shadow Variable Detection
* Removed the deprecated `govet.check-shadowing` option from `.golangci.yml`.
* Resolved deprecation warnings without requiring immediate tool upgrades.
* Upgrading to use the new `shadow` linter was deferred due to version compatibility issues.

## Error Handling

### JSON-RPC Error Structure
* Using JSON-RPC 2.0 compliant error structure throughout the application.
* Consistent error handling approach that satisfies MCP protocol requirements while providing structured error information to clients.
* [3] JSON-RPC 2.0 Specification

### Error Handling Practices
* Adopted standard Go error handling: explicit checks, wrapping with context, early returns, and custom errors.
* Follows the Go philosophy of "Errors are values", provides rich context for debugging, and maintains code readability.
* Ignoring errors or overusing `panic` was rejected as it leads to fragile, hard-to-debug code.

## MCP Protocol Implementation

### Protocol Conformance Testing
* Implemented a comprehensive suite of MCP conformance tests.
* Ensures protocol compliance, detects regressions immediately, and simplifies validation when adding new features.

## Authentication

### Token Storage
* Implemented secure token storage with optional encryption.
* Protects sensitive authentication data while maintaining flexibility for development environments.
* Plain text storage in production was rejected for security reasons.

## Service Integration

### RTM API Client Structure
* Created a layered client structure with separate transport, authentication, and domain-specific modules.
* Improves maintainability through separation of concerns and simplifies testing of individual components.

---

### References

[1]: https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html "The Clean Architecture by Robert C. Martin"
[2]: https://github.com/gotestyourself/gotestsum "gotestsum documentation"
[3]: https://www.jsonrpc.org/specification#error_object "JSON-RPC 2.0 Specification"
```
