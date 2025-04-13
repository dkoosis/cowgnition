# 🐄 🧠 CowGnition

<img src="/docs/assets/cowgnition_logo.png" alt="CowGnition Logo" width="100" height="100">

CowGnition connects your [Remember The Milk](https://www.rememberthemilk.com/) (RTM) tasks with AI assistants like Claude Desktop via the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/).

---

**➡️ Looking to install and use CowGnition?**

- Check out the [**Installation & User Guide**](docs/USER_GUIDE.md)
- Download the [**Latest Release**](https://github.com/dkoosis/cowgnition/releases)

---

## Project Status

- **Status:** Active Development
- **Roadmap/Tasks:** See the [TODO.md](docs/TODO.md) file.

## What is CowGnition (Technically)?

CowGnition is an MCP server written in Go. It acts as a bridge, allowing MCP clients to interact with the Remember The Milk API by exposing RTM functionalities as MCP Tools and Resources. It aims for strict protocol compliance, robust error handling, and a maintainable codebase.

**Core Technologies & Architecture:**

- **Language:** Go (requires version 1.21 or later)
- **Protocol:** Model Context Protocol (MCP) over JSON-RPC 2.0
- **Transport:** Newline Delimited JSON (NDJSON) via Standard I/O (`internal/transport`)
- **Validation:** Strict MCP JSON Schema validation using `santhosh-tekuri/jsonschema/v5` via middleware (`internal/middleware`, See [ADR 002](docs/adr/002_schema_validation_strategy.md))
- **Error Handling:** Using `cockroachdb/errors` for context-rich errors and stack traces (See [ADR 001](docs/adr/001_error_handling_strategy.md))
- **Logging:** Structured logging via `log/slog` (`internal/logging`)
- **Configuration:** Primarily via environment variables, with potential for `yaml` file support (`internal/config`)
- **RTM Integration:** Custom RTM client (`internal/rtm`) handling API calls, authentication, and secure token storage (See [ADR 005](docs/adr/005_secret_management.md))
- _(Planned: Modular Service Architecture - See [ADR 006](docs/adr/006_modular_multi_service_support.md))_

## Getting Started (Development)

1.  **Prerequisites:** Go (version 1.21+), Make (optional, for convenience)
2.  **Clone:** `git clone https://github.com/dkoosis/cowgnition.git && cd cowgnition`
3.  **Build:** `make build` (or `go build -o cowgnition ./cmd/...`)
4.  **Test:** `make test` (or `go test ./...`)

## Configuration (Development)

CowGnition primarily uses environment variables for configuration:

- `RTM_API_KEY`: **Required.** Your RTM API key. Get one from [RTM Developer](https://www.rememberthemilk.com/services/api/).
- `RTM_SHARED_SECRET`: **Required.** Your RTM shared secret.
- `LOG_LEVEL`: Optional (`debug`, `info`, `warn`, `error`). Defaults to `info`.
- `COWGNITION_TOKEN_PATH`: Optional. Path for storing the RTM auth token (defaults to OS-specific config dir like `~/.config/cowgnition/rtm_token.json`).

_(A `cowgnition.yaml` file might be used for development overrides - check `internal/config/config.go` for details if implemented)._

## Running Locally (Development)

```bash
# Ensure environment variables are set (or use a .env file if supported)
export RTM_API_KEY="YOUR_KEY"
export RTM_SHARED_SECRET="YOUR_SECRET"
export LOG_LEVEL="debug" # Optional: for more verbose logging

# Run the server (it will listen on stdio)
./cowgnition serve
# Or use the Makefile:
# make run
You can then test it using tools like the MCP Inspector or by configuring a development instance of Claude Desktop to point to your locally built binary (using its absolute path in claude_desktop_config.json).ContributingWe welcome contributions! Please see the Contributing Guide for details on code style, workflow, and more
```
