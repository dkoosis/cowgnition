# ADR Draft: In-Memory Transport for Integration Testing

**Status:** Proposed for Future Consideration

**Date:** 2025-04-09

## 1. Context

- **Current Testing:** Integration testing the full MCP server (`internal/mcp/mcp_server.go`) interaction, including middleware (`internal/middleware/`) and transport (`internal/transport/transport.go`), currently requires either managing external client processes or complex mocking of the stdio/network transport layer.
- **Need:** Reliable, fast, and self-contained integration tests are crucial for ensuring protocol compliance and preventing regressions, as outlined in the goals for **Phase 5: Testing Framework**.
- **Reference:** The official MCP TypeScript SDK includes an `InMemoryTransport` class specifically designed for in-process testing, using linked pairs to simulate bidirectional communication.

## 2. Proposed Solution (Conceptual)

Implement an in-memory transport mechanism for the Go codebase that fulfills the `transport.Transport` interface.

- **Mechanism:** Create a pair of connected transport instances that communicate directly within memory, bypassing actual stdio or network I/O. This could be achieved using:
  - Go's standard `net.Pipe()`, which provides connected in-memory `net.Conn` interfaces that simulate network connections. The `NDJSONTransport` could potentially use these pipes as its `io.Reader`/`io.Writer`/`io.Closer`.
  - A custom implementation using Go channels to pass `JSONRPCMessage` objects directly between linked transport instances, similar in concept to the TypeScript `InMemoryTransport` example.
- **Usage:** In Go integration tests (`_test.go` files), create a linked pair of these in-memory transports. Connect one end to the `mcp.Server` instance and the other end to a test harness or a minimal client implementation running within the same test function. This allows sending requests and asserting responses through the full server stack in-process.

## 3. Consequences (Preliminary)

- **Positive:**
  - Enables robust integration testing of the full server stack (transport handling -> middleware -> handlers -> response generation -> transport writing) without external dependencies or processes.
  - Likely significantly faster execution compared to tests involving process management or real I/O.
  - Improves test reliability by eliminating external environment factors.
  - Simplifies test setup and teardown.
- **Negative:**
  - Requires implementation effort to create the in-memory transport that correctly mimics the behavior of the real transport (especially regarding message framing like NDJSON if using `net.Pipe` with `NDJSONTransport`).
  - Doesn't test the specifics of stdio handling or potential network issues, which might still require separate, true end-to-end tests (though fewer of them).
