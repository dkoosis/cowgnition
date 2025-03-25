### Updated Draft 3: `docs/PROJECT_ORGANIZATION.md`

```markdown
# CowGnition Project Organization (Quick Start)

Welcome to CowGnition! This document provides a high-level overview to help new developers understand the project structure and get started quickly.

For a comprehensive guide, please refer to the main [Development Overview](development_overview.md). For the user guide and basic setup, see the main [README.md](../README.md).

## Project Overview

CowGnition is a server that connects [Remember The Milk](https://www.rememberthemilk.com/) (RTM) to AI assistants like Claude Desktop using the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/). It allows users to manage their RTM tasks through natural conversation.

Architecturally, the project follows a layered approach to separate concerns.

## Directory Structure Essentials

The project uses the [Standard Go Project Layout](https://github.com/golang-standards/project-layout). Key directories:

cowgnition/
├── Makefile # Common tasks (build, test)
├── README.md # User setup & usage guide
├── cmd/ # Main application entry point
├── configs/ # Example configuration files
├── docs/ # Project documentation (you are here!)
├── internal/ # Core application code (private)
│ ├── ... (auth, client, config, mcp, server, service)
└── test/ # Integration/conformance tests, mocks, helpers

- **`cmd/cowgnition`**: Application entry point.
- **`internal/`**: Main Go code, organized by responsibility. Most development happens here.
- **`configs/`**: Example configurations.
- **`test/`**: Non-unit tests and support files. Unit tests (`_test.go`) are alongside code in `internal/`.
- **`docs/`**: Detailed documentation ([Development Overview](development_overview.md), [Decision Log](decision_log.md), etc.).

See the [Directory Structure section in the Development Overview](development_overview.md#directory-structure) for full details.

## Getting Started Guide

Minimal steps for a development environment:

1.  **Install Go:** Ensure Go (>= 1.21) is installed. ([go.dev/dl/](https://go.dev/dl/))
2.  **Clone:** `git clone https://github.com/cowgnition/cowgnition.git`
3.  **Navigate:** `cd cowgnition`
4.  **Build:** `make build`
5.  **Configure:** Create `configs/config.yaml` based on the example in the main [README.md](../README.md#configuration). You'll need RTM API keys from [Remember The Milk](https://www.rememberthemilk.com/services/api/keys.rtm).
6.  **Run Basic Tests:** `make test`

This allows running the server (`./cowgnition serve --config configs/config.yaml`) and basic development.

For full setup including all developer tools (linter, debugger, etc.), see the [Development Environment Setup in the Development Overview](development_overview.md#development-environment-setup).

## Contribution Workflow (Simplified)

1.  **Branch:** Create a branch off `develop` [TODO: Verify branch] (e.g., `feature/my-task-tool`).
2.  **Develop & Test:** Write code and add tests. Ensure `make test` passes.
3.  **Lint:** Ensure `make lint` passes.
4.  **Commit:** Use format `area: description` (e.g., `mcp: add list tags tool`).
5.  **PR:** Open a Pull Request against `develop` [TODO: Verify branch].
6.  **Review & Merge:** Address feedback; PR gets merged upon approval.

For detailed guidelines consult the [Contributing section in the Development Overview](development_overview.md#contributing).

---
```
