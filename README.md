Okay, I can help rewrite the technical sections of the README to align with the architecture we've planned and include the requested links. I'll keep the introduction and general purpose sections mostly intact, as they seem appropriate.

Here's a revised README.md:

---

# CowGnition 🐄 🧠

<img src="/docs/assets/cowgnition_logo.png" alt="CowGnition Logo" width="100" height="100">

## Have you Herd?

CowGnition connects your [Remember The Milk](https://www.rememberthemilk.com/) (RTM) tasks with Claude Desktop and other MCP clients. This MCP server lets you ask your AI assistant to manage your to-do lists, look up due dates, and get insights about your tasks through simple conversations. The [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) allows conversational agents that are MCP clients to use "tools" and "resources" like those provided by this server to interact with services like RTM on your behalf.

## Quick Links

- [Architecture Decisions](#architectural-decisions-adrs) - Key design choices made for this project.
- [TODO](docs/TODO.md) - Current development tasks and roadmap items.

## Core Architecture

This MCP server is implemented in **Go** and follows modern development practices. Key architectural features include:

- **Protocol:** Implements **JSON-RPC 2.0** as required by MCP.
- **Transport:** Uses **Newline Delimited JSON (NDJSON)** over standard I/O (`stdio`) for communication with local clients (like Claude Desktop) or potentially raw TCP sockets. This transport is custom-built using Go's standard library (`net`, `bufio`, `encoding/json`) for efficiency and control.
- **Validation:** Incoming messages are rigorously validated against the official **MCP JSON Schema** using the `santhosh-tekuri/jsonschema/v5` library within a dedicated middleware layer. This ensures strict compliance with both JSON-RPC 2.0 structure and MCP-specific method/parameter definitions.
- **Error Handling:** Leverages the `cockroachdb/errors` library for rich, context-aware internal error handling and stack traces, mapped appropriately to JSON-RPC 2.0 error responses and detailed server-side logs.
- **Logging:** Employs structured logging (using `slog`) adhering to the recommendations in the MCP specification, providing detailed operational and error information.
- **Configuration:** Prioritizes simplicity, primarily using **environment variables** for configuration (like API keys).

## Architectural Decisions (ADRs)

Detailed explanations for key architectural choices are recorded in ADRs:

- [ADR 001: Error Handling Strategy](docs/001_error_handling_strategy.md) - Decision to use `cockroachdb/errors` and specific error handling patterns.
- [ADR 002: Schema Validation Strategy](docs/002_schema_validation_strategy.md) - Decision to use JSON Schema validation via middleware.
- _(Other ADRs for Transport, Logging, Configuration may be added here)_

## Project Status / TODO

Please see the [TODO.md](docs/TODO.md) file for the current development status, planned features, and known issues.

## Getting Started (Development)

1.  **Prerequisites:** Go (version 1.21 or later recommended).
2.  **Clone:** `git clone <repository-url>`
3.  **Build:** `go build -o cowgnition_server ./cmd/server/`
4.  **Run Tests:** `go test ./...`

## Configuration

Configuration is primarily handled via environment variables for simplicity:

- `RTM_API_KEY`: **Required.** Your Remember The Milk API key.
- `RTM_SHARED_SECRET`: **Required.** Your Remember The Milk shared secret.
- `LOG_LEVEL`: Optional. Set the logging level (e.g., `debug`, `info`, `warn`, `error`). Defaults to `info`.
- _(Add other variables as needed, e.g., for auth token storage path if not derived)_

Example:

```bash
export RTM_API_KEY="YOUR_RTM_API_KEY"
export RTM_SHARED_SECRET="YOUR_RTM_SHARED_SECRET"
./cowgnition_server
```

_(Note: A mechanism for handling the RTM authentication flow (likely OAuth) and storing tokens securely will be required. See TODO.)_

## Running / Connecting to Clients

The server primarily communicates via **stdio** when launched by a client like Claude Desktop.

**Example `claude_desktop_config.json` entry:**

```json
{
  "mcpServers": {
    "cowgnition-rtm": {
      "command": "/path/to/your/built/cowgnition_server",
      "args": [],
      "env": {
        "RTM_API_KEY": "YOUR_RTM_API_KEY_HERE",
        "RTM_SHARED_SECRET": "YOUR_RTM_SHARED_SECRET_HERE",
        "LOG_LEVEL": "debug"
      }
    }
  }
}
```

_Replace `/path/to/your/built/cowgnition_server` with the absolute path to the compiled binary._
_Ensure the API key and secret are correctly set, potentially using a more secure method than directly in the config file for sensitive keys._
_Restart Claude Desktop after modifying the configuration._

Once connected, the server status and its capabilities (Tools, Resources, Prompts it exposes) should appear in the client UI.

## MCP Implementation Details

CowGnition implements the server-side of the Model Context Protocol to interact with Remember The Milk. This involves:

- Handling the MCP **initialization handshake**.
- Exposing RTM functionalities as MCP **Tools** (e.g., `createTask`, `completeTask`, `getTasksByFilter`). The specific tools are defined by the server's implementation (see `TODO.md`). Tool definitions include JSON Schemas for their inputs.
- Potentially exposing RTM data views as MCP **Resources** (e.g., lists, tags, specific tasks). Resource definitions include URIs and MIME types.
- Responding correctly to core MCP requests like `tools/list`, `tools/call`, `resources/list`, `resources/read`.

The exact set of supported Tools and Resources is subject to ongoing development (see `TODO.md`).

## License

This project is licensed under the MIT License. See the LICENSE file for details.

## Thank You

- The Remember The Milk team for their wonderful task system and API.
- Anthropic for creating the Model Context Protocol.
- The Go community for excellent development tools and libraries.

Moo-ve your productivity forward with CowGnition and Claude! 🐄 🧠

---
