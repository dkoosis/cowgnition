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
