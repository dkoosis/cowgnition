I'll transform the content from the paste.txt file into a clean Markdown file:

# Architecture Decision Record: Schema Validation Strategy (ADR 002 - Revised)

## Date

2025-04-06

## Status

Accepted

## Context

The CowGnition project is implementing a Model Context Protocol (MCP) server. Adherence to the protocol specification is critical for interoperability with MCP clients (like Claude Desktop) and the broader ecosystem. This requires validating messages against multiple layers:

1. **Transport Framing:** Ensuring complete messages are received (e.g., NDJSON lines). (Handled by Transport - ADR 001 discusses error logging).
2. **JSON Syntax:** Ensuring the received bytes constitute valid JSON (per RFC 8259).
3. **JSON-RPC 2.0 Structure:** Ensuring the valid JSON follows the generic JSON-RPC 2.0 specification for requests, responses, notifications, and errors (the "envelope").
4. **MCP Semantics:** Ensuring the structurally valid JSON-RPC message conforms to MCP's specific methods, and the `params` and `result` payloads match the definitions for those methods (the "content").

We need a robust, centralized, and maintainable strategy to perform validation covering layers 2, 3, and 4 efficiently before messages reach application logic. This aligns with standard practices in the MCP ecosystem, where official SDKs (like the TypeScript SDK using Zod) heavily rely on schema definitions. Relying on an official MCP schema minimizes ambiguity and ensures alignment.

## Decision

1. **Mechanism:** We will use **JSON Schema** as the formal, machine-readable definition language for validating MCP messages.
2. **Schema Source:** We will utilize the **official MCP JSON Schema**. By default, the schema content from `internal/schema/schema.json` will be **embedded directly into the application binary** at compile time using `go:embed`. A `SchemaOverrideURI` (file or http(s)) in the configuration can be used to load an external schema at runtime, overriding the embedded version. This single schema source (embedded or override) must define:
   * Base JSON structures confirming to JSON-RPC 2.0.
   * Specific MCP method names (e.g., via `enum` on the `method` field).
   * The exact structure (`properties`, `required` fields, types) for the `params` object of each supported MCP request method.
   * The exact structure for the `result` object of each supported MCP response.
3. **Validation Library:** We will use the `github.com/santhosh-tekuri/jsonschema/v5` Go library, which supports recent JSON Schema drafts (like 2020-12), for performing the validation.
4. **Implementation Layer:** Validation will be performed within a dedicated **Validation Middleware** layer, executed immediately after the Transport layer successfully reads a framed message and before the message is dispatched to the method router.
5. **Transport Scope:** The Transport layer (`NDJSONTransport`) will _not_ perform JSON parsing or any JSON-RPC/MCP structural validation. Its responsibility is limited to byte stream I/O and NDJSON framing.
6. **Error Handling:** The Validation Middleware will be responsible for mapping JSON syntax errors and JSON Schema validation failures to the appropriate standard JSON-RPC 2.0 error codes (`-32700`, `-32600`, `-32602`) and generating the error response to be sent back immediately, preventing further processing of invalid messages.

## Consequences

### Positive

- Ensures strict and comprehensive protocol compliance (JSON syntax, JSON-RPC 2.0 structure, MCP semantics) against the official specification using a single validation step per message.
- Centralizes complex validation logic, simplifying method handler implementations (which can assume schema-valid input).
- Provides clear, specific, and standardized error responses for protocol violations, improving debuggability for client developers.
- Leverages a standard, widely adopted mechanism (JSON Schema) for defining structural constraints.
- Decouples transport logic (I/O, framing) from protocol validation logic.
- Promotes consistency with official MCP SDKs (TS, Python, Java) which likely employ similar schema-driven validation.
- Default schema is embedded, ensuring availability without external dependencies. Override provides flexibility.
- Validation rules (defined in the external schema) can potentially be updated independently of server code, provided the schema structure remains compatible with server logic.

### Negative

- Introduces a runtime dependency on the `santhosh-tekuri/jsonschema/v5` library.
- Critical dependency on the availability, correctness, versioning, and maintenance of the MCP JSON Schema file/source (whether embedded or external). Schema changes could be breaking.
- Requires robust implementation for loading (handling override failures), compiling (potentially time-consuming at startup), and caching the JSON Schema. Error handling for schema loading/compilation failures is crucial.
- Adds a processing step (schema validation) for every message, introducing performance overhead (expected to be minor, but requires monitoring).
- Team members may need to familiarize themselves with JSON Schema principles and the specific structure of the MCP schema.

## Implementation Guidelines

### Schema Loading & Validator Component (`SchemaValidator`)

- Implement a component responsible for fetching/reading the official MCP JSON Schema from a configured URL or file path at application startup. **If no valid override is provided or loading fails, it MUST fall back to using the schema embedded in the binary via `go:embed`.**
- Use `jsonschema.NewCompiler()` to add the schema resource and `compiler.Compile()` to get compiled `*jsonschema.Schema` objects. This pre-processing is vital for performance.
- Handle potential loading/compilation errors robustly (e.g., log critical error, prevent server start if schema is essential).
- **Cache the compiled schemas in memory** within the validator instance for efficient access during request processing.
- Consider strategies for external schema updates (e.g., require server restart, periodic refresh with checks if using override).
- Provide a method like `Validate(ctx context.Context, messageType string, messageData []byte) error`. The `messageType` helps select the specific definition within the overall MCP schema (e.g., based on the MCP method name like `"initialize"`, `"resources/list"`, or a generic identifier like `"MCPRequest"`, `"MCPResponse"`).

### Schema Loading/Caching Flow

Here's a summary of the schema processing flow from build to runtime validation:

1. **Build Time:** The Go compiler embeds `internal/schema/schema.json` directly into the application binary.
2. **Runtime Init:** On server startup, the `SchemaValidator` is created.
3. **Load Data:** `validator.Initialize` first checks the configuration for an override URI. If present and valid, it attempts to load the schema from that external file/URL. Otherwise (or if override load fails), it uses the schema content embedded within the binary.
4. **Compile & Cache:** The loaded schema data is compiled *once* using `jsonschema.Compiler`. The resulting compiled schema objects are stored (cached) in memory within the `Validator` instance.
5. **Validation:** Subsequent `validator.Validate` calls efficiently use these cached, pre-compiled schema objects from memory.

### Validation Middleware

**Position:** Receives `[]byte` from `Transport.ReadMessage`. Output goes to Router/Dispatcher.

**Functionality:**
- **Basic JSON Check:** Optionally perform a quick `json.Valid()` check first. If it fails, immediately log and prepare a `-32700` JSON-RPC error response.
- **Identify Message Type:** Parse just enough of the JSON (e.g., using `json.RawMessage` and targeted unmarshalling) to determine the specific MCP message type (e.g., extract the method string for requests, or identify if it's a response based on result/error). This identified type is needed to select the correct schema definition for validation. Handle errors identifying the type (likely `-32600`).
- **Schema Validation:** Call `schemaValidator.Validate(ctx, identifiedType, messageBytes)`.
- **Error Handling:**
  - If `Validate` returns `nil`, pass the `msgBytes` (or potentially a partially parsed representation) to the next handler.
  - If `Validate` returns an error (e.g., `*jsonschema.ValidationError`):
    - Log the detailed violation server-side (include `validationError.Causes`, `KeywordLocation`, `InstanceLocation` for precise debugging).
    - Map the validation error to the appropriate JSON-RPC code (`-32600` for general structure, `-32602` for specific parameter issues).
    - Construct the JSON-RPC error response containing the code and a helpful message (potentially derived from the validation error details, but sanitized).
    - Signal that this error response should be sent back immediately (e.g., return the response bytes and a specific sentinel error, or use a middleware chain control flow).

### Schema Scope

The official MCP JSON Schema used must comprehensively define:

- Basic JSON types and structure.
- JSON-RPC 2.0 requirements: Presence and type of `jsonrpc: "2.0"`, `id`, `method`, `params`, `result`, `error`. Rules for combining `result`/`error`. Rules for Notifications (no `id`).
- MCP method names: Ideally using an `enum` constraint on the `method` field for requests/notifications.
- MCP params: The exact structure (properties, required fields, types, nested objects/arrays) for the `params` object specific to each defined MCP method.
- MCP result: The exact structure for the `result` object specific to each defined MCP method response.
- MCP error.data: Any expected structure for the optional `data` field in MCP-specific errors, if applicable.

## Related Specifications

- JSON-RPC 2.0 Specification
- MCP Specification (e.g., 2024-11-05)
- MCP Concepts Documentation (Defines method payloads)
- JSON Schema Specification (Draft 2020-12)

## References

- santhosh-tekuri/jsonschema/v5 Documentation
- JSON Schema Specification Home
- JSON-RPC 2.0 Specification
- MCP Specification Home
- MCP Documentation Home
- Zod Library (Used in MCP TypeScript SDK, relevant context for schema-driven approach)