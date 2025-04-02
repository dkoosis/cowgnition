# Decision Log

This is a log of the decisions we make about tools, patterns, and libraries with a brief statement of the benefits we expect. The main value of this log is to prevent us from revisiting decisions we've already made.

## Error Handling

This project requires conformance to MCP's error-handling standards, which in turn are based on JSON-RPC2.0. this means... TODO

### cockroachdb/errors

cockroachdb/errors builds on Go's errors, adding stack traces and ways to attach key-value data (properties). This helps us find bugs faster with stack traces and lets us add needed details to errors for logs and JSON-RPC data fields. The library also suits networked programs like ours.
We considered errorx for its error grouping (traits), but cockroachdb/errors' property system seemed adequate, and its better debugging and network features were more important for this project.

## JSON-RPC2.0

### sourcegraph/jsonrpc2

## Configuration Management

### koanf

We will use koanf for configuration management instead of Viper. Koanf offers a simpler, lighter approach with no core dependencies. It supports all our needs: file discovery in multiple paths, environment variable handling, multiple format support, and clear override precedence.

Despite being newer than Viper, koanf follows Go idioms more closely and provides a more modular design. We believe this fits our project's current complexity level better than the heavier Viper alternative. Koanf's explicit layering of configuration sources will help solve our concerns about config file precedence and user confusion.

## Logging

+We will adopt structured logging using a JSON format. This will provide consistent formatting, make logs more readable, and ease integration with cloud logging systems. This approach will improve debugging and observability.

## MCP Connection Flow Architecture

We've chosen to implement a State Machine-based Event Handler architecture for the MCP connection flow. This architecture models the connection lifecycle as explicit states (Unconnected, Initializing, Connected, Terminating, Error) with clearly defined transitions between them. Message handling is implemented as an event-driven system that dispatches requests to appropriate handlers based on the current state and message type.

This approach was selected because it directly addresses our key requirements:

1. **Protocol Compliance**: The explicit state machine enforces the correct connection lifecycle and message sequencing as defined in the MCP specification.
2. **Developer Experience**: Structured logging with connection context, clear error handling distinguishing protocol vs. system errors, and transparent state transitions provide excellent debuggability.
3. **Future Extensibility**: The architecture naturally accommodates stateful features like resource subscriptions, progress tracking, and request cancellation.

We considered alternatives including a simpler request-response loop and an actor model, but the state machine approach better aligns with Go idioms and the stateful nature of the MCP protocol. It provides the right balance of structure, flexibility, and maintainability while remaining idiomatic Go.

### state machine library, or hand-made?

The ConnectionManager, responsible for handling the complex Model Context Protocol (MCP) connection lifecycle, currently uses a hand-rolled state machine. Given MCP's richness—including asynchronous notifications, subscriptions, and cancellations—this manual approach raises concerns about long-term maintainability, robustness, and boilerplate code (like explicit state checks in handlers). To mitigate these issues, the recommendation is to adopt a dedicated Go state machine library. This shift aims to leverage library benefits such as declarative state definitions, built-in transition validation, reduced boilerplate, and potentially better concurrency management.

Comparing popular options, qmuntal/stateless is recommended for closer initial assessment over the simpler looplab/fsm. While looplab/fsm is capable, the advanced features of qmuntal/stateless (like hierarchical states, guards, and a fluent configuration API) appear potentially better suited for modeling the intricacies suggested by the MCP schema and maintaining clarity as complexity grows. However, looplab/fsm remains a solid alternative if a simpler feature set proves sufficient after evaluation.
