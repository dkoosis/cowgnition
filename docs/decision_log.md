# Decision Log

This is a log of the decisions we make about tools, patterns, and libraries with a brief statement of the benefits we expect. The main value of this log is to prevent us from revisiting decisions we've already made.

## Error Handling

This project requires conformance to MCP's error-handling standards, which in turn are based on JSON-RPC2.0. this means... TODO

### cockroachdb/errors

cockroachdb/errors builds on Go's errors, adding stack traces and ways to attach key-value data (properties). This helps us find bugs faster with stack traces and lets us add needed details to errors for logs and JSON-RPC data fields. The library also suits networked programs like ours.
We considered errorx for its error grouping (traits), but cockroachdb/errors' property system seemed adequate, and its better debugging and network features were more important for this project.

## JSON-RPC2.0

### sourcegraph/jsonrpc2
