### Updated Draft 2: `docs/development_overview.md`

# CowGnition Development Overview

This document serves as the central knowledge base for developers working on the CowGnition project. It provides a comprehensive guide to all aspects of development, ensuring consistency and efficient onboarding.

For a record of _why_ certain decisions were made, please refer to the [Decision Log](decision_log.md).

## Project Architecture

CowGnition adopts a layered architecture inspired by Clean Architecture principles to separate concerns and improve maintainability and testability. (See [Decision Log](decision_log.md#architecture) for rationale).

### High-Level Components

[TODO: Add high-level component diagrams illustrating the relationships between different parts of the system (e.g., MCP Server, RTM Client, Config, Auth layers).]

### Directory Structure

We follow the idiomatic [Standard Go Project Layout](https://github.com/golang-standards/project-layout). (See [Decision Log](decision_log.md#folder-organization) for rationale and structure details).

cowgnition/
├── Makefile # Build, test, and utility tasks
├── README.md # Project entry point
├── cmd/ # Command-line applications (main entry points)
│ └── cowgnition/ # The main application binary
├── configs/ # Example configuration files
├── docs/ # Project documentation (this file, decision_log.md, etc.)
├── go.mod # Go module definition
├── go.sum # Dependency checksums
├── internal/ # Private application code, not importable by others
│ ├── auth/ # Authentication handling (RTM OAuth, token management)
│ ├── client/ # Clients for external services (e.g., RTM API)
│ ├── config/ # Configuration loading and management
│ ├── mcp/ # Model Context Protocol implementation (resources, tools)
│ ├── server/ # MCP server logic (HTTP handling)
│ └── service/ # Core business logic orchestrating components
├── pkg/ # Public libraries (if any, currently minimal/none)
├── scripts/ # Utility scripts (e.g., test runners)
└── test/ # Testing infrastructure (integration, conformance, mocks, etc.)

_(Refer to `decision_log.md` or the official standard for detailed descriptions of standard directories)_

### Key Data Flows

[TODO: Add descriptions and/or diagrams of key data flows, e.g., MCP request -> Server -> Service -> RTM Client -> RTM API -> Response flow.]

## Development Environment Setup

### Required Tools

Our standard toolset ensures consistency and quality. (See [Decision Log](decision_log.md#core-development-tooling) and [Decision Log](decision_log.md#additional-tooling) for rationale behind choices).

| Tool                                                             | Purpose                            | Installation                                                            | Chosen For...                 |
| ---------------------------------------------------------------- | ---------------------------------- | ----------------------------------------------------------------------- | ----------------------------- |
| [Go](https://go.dev/dl/) (>= 1.21)                               | The Go compiler and toolchain      | OS-specific package manager or download from go.dev                     | Core language                 |
| [golangci-lint](https://golangci-lint.run/usage/install/)        | Comprehensive linting tool         | `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest` | Aggregated linters, speed     |
| [goimports](https://pkg.go.dev/golang.org/x/tools/cmd/goimports) | Import organization and formatting | `go install golang.org/x/tools/cmd/goimports@latest`                    | Standard Go formatting        |
| [gotestsum](https://github.com/gotestyourself/gotestsum)         | Enhanced test output/reporting     | `go install gotest.tools/gotestsum@latest`                              | Readability, CI integration   |
| [staticcheck](https://staticcheck.io/docs/getting-started/)      | Advanced static analysis tool      | `go install honnef.co/go/tools/cmd/staticcheck@latest`                  | Deeper code analysis          |
| [gopls](https://pkg.go.dev/golang.org/x/tools/gopls)             | Go language server (for IDEs)      | `go install golang.org/x/tools/gopls@latest`                            | Consistent IDE experience     |
| [dlv](https://github.com/go-delve/delve)                         | Debugger                           | `go install github.com/go-delve/delve/cmd/dlv@latest`                   | Standard Go debugging         |
| [mockgen](https://github.com/golang/mock)                        | Mock generation                    | `go install github.com/golang/mock/mockgen@latest`                      | Consistent, type-safe mocks   |
| [entr](https://github.com/eradman/entr)                          | File watcher (optional workflow)   | OS-specific package manager (e.g., `brew install entr`)                 | Simple file watching          |
| `make`                                                           | Build automation tool              | Usually pre-installed on Linux/macOS; install via package manager       | Standard build runner         |
| _Preferred:_ [cobra](https://github.com/spf13/cobra)             | CLI framework                      | `go get ...` (if needed)                                                | Rich CLI features (if needed) |
| _Preferred:_ [viper](https://github.com/spf13/viper)             | Configuration                      | `go get ...` (if needed)                                                | Advanced config loading       |
| _Preferred:_ [zap](https://github.com/uber-go/zap)               | Logging                            | `go get ...` (if needed)                                                | Structured, high-perf logging |

### Setup Instructions

1.  **Install Go:** Ensure you have Go version 1.21 or later installed. Verify with `go version`.
2.  **Install Tools:** Install the required Go tools listed above using the `go install` commands. Ensure your `$(go env GOPATH)/bin` directory is in your system's `PATH`. Install `make` and `entr` via your OS package manager if needed.
3.  **Clone Repository:** `git clone https://github.com/cowgnition/cowgnition.git`
4.  **Navigate to Directory:** `cd cowgnition`
5.  **Environment Variables:** [TODO: List any required environment variables, e.g., for specific RTM API endpoints if not default, or development flags.]
6.  **Dependencies:** Dependencies are managed using Go modules. They will be downloaded automatically when building or testing. You can explicitly download them with `go mod download`.

### Dependency Management (Go Modules)

- Use Go modules (`go.mod`, `go.sum`).
- Run `go mod tidy` before committing.
- Pin dependencies; review updates regularly (`go list -u -m all`).
- We prefer manual Dependency Injection over tools like `wire`. (See [Decision Log](decision_log.md#dependency-injection)).

### Building the Project

Use the Makefile for common tasks:

```bash
# Build the main cowgnition binary (output to project root)
make build

# Clean build artifacts
make clean
Coding Standards and Practices
Code Formatting
Format code using goimports. Configure your editor for auto-save formatting. Checked by CI.
Linting
Use golangci-lint with .golangci.yml. (See Decision Log).
Run make lint locally before committing.
Core enabled linters: govet, staticcheck, gosec, errcheck, ineffassign, gocritic, etc. (See .golangci.yml for the full list).
Error Handling
Follow standard Go practices: explicit checks, wrapping (fmt.Errorf("... %w", err)), early returns. (See Decision Log).
Use the JSON-RPC 2.0 error structure for MCP server errors. (See Decision Log).
Documentation (GoDoc)
Document all exported items. Follow standard Go comment style (Effective Go, GoDoc). Include examples where helpful.
Function Size and Complexity
Aim for small, focused functions (<30-50 lines). Keep cyclomatic complexity low (<15). Use clear names.
Go Principles
Be guided by the Go Proverbs.
Testing
(See Decision Log for rationale on tool choices).

Test Commands (via Makefile)
Uses gotestsum for improved output.

Bash

# Run all unit and integration tests ('pkgname' format)
make test

# Run tests and generate HTML coverage report (coverage.html)
make test-coverage

# Run the MCP conformance test suite
make test-conformance
# (See ../test/conformance/README.md for details)

# Run tests for a specific package (standard go test)
go test ./internal/config/...

# Run a specific test function (standard go test)
go test ./internal/config -run TestLoadConfig
Test Output and Reporting
make test uses gotestsum --format pkgname.
make test-coverage generates coverage.html. Aim for >70% coverage.
CI uses gotestsum --junitfile report.xml.
Test Structure and Organization
(See Decision Log for rationale).

test/
├── README.md          # Overview
├── conformance/       # MCP Conformance tests -> See conformance/README.md
├── fixtures/          # Static test data
├── helpers/           # Reusable test helpers (use t.Helper())
├── integration/       # Tests requiring multiple components/services
├── mocks/             # Generated mocks (via mockgen)
└── validators/        # Custom validation logic for tests
Unit Tests: Alongside code (_test.go).
Integration Tests: In test/integration/.
Conformance Tests: In test/conformance/. See the specific MCP Conformance Test Documentation.
Test Design Principles
Write tests that are: Isolated, Predictable, Clear, Complete, Maintainable. Use table-driven tests.

Mocking
Use golang/mock/mockgen exclusively. (See Decision Log).
Generate mocks from interfaces into test/mocks/.
[TODO: Add command or make target for generating mocks, e.g., make mocks].
Contributing
Code Reviews
Submit PRs against develop [TODO: Verify branch].
Ensure tests (make test) and lint (make lint) pass. Include new tests.
Follow Coding Standards.
Expect review feedback.
Commit Messages
Use format area: brief description (e.g., mcp: implement list tasks tool). Reference issues ((closes #123)).
Branching Strategy
[TODO: Detail the project's branching strategy (e.g., Gitflow-like with main, develop, feature/, fix/ branches).]

Authentication Details
The authentication flow involves OAuth with Remember The Milk.
Tokens are stored securely. (See Decision Log).
RTM Client
The RTM API client is in internal/client/rtm/. It's structured with layers for transport, auth, and domain logic. (See Decision Log).
References
Effective Go
Go Code Review Comments
Standard Go Project Layout
Go Proverbs
Decision Log - Rationale for project choices.
Project Organization (Quick Start) - High-level overview.
Main README - User-facing setup and usage.
Remember The Milk
```

```

```
