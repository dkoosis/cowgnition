````markdown
# Cowgnition MCP Server: Error Handling Guidelines

This document outlines the conventions and best practices for error handling within the Cowgnition MCP server project. Consistent and informative error handling is crucial for debugging, maintaining system reliability, and providing useful information to clients.

## 1. Core Principles

- **Use `cockroachdb/errors`**: We leverage the `cockroachdb/errors` package for rich error context, including stack traces, wrapping, and properties[cite: 57, 97]. All errors should utilize this package.
- **Provide Context**: Every error should provide enough context to understand where it occurred and why[cite: 95].
- **Categorize Errors**: Errors must be assigned to a category to facilitate filtering and handling[cite: 95, 97].
- **Use Error Codes**: When appropriate, use predefined error codes for programmatic error identification, especially for JSON-RPC responses[cite: 95, 97].
- **Properties for Debugging**: Attach relevant debugging information as error properties[cite: 95, 97]. Be cautious about including sensitive data.
- **Sentinel Errors**: Define sentinel errors for specific, predictable error conditions and use `errors.Is` for checking them[cite: 95, 97, 103].
- **JSON-RPC Compliance**: Ensure errors returned to clients adhere to the JSON-RPC 2.0 error object structure[cite: 94].

## 2. Error Categories

Errors are categorized to help in routing, logging, and handling. Here are the defined categories:

- `resource`: Errors related to resource operations (e.g., reading, access)[cite: 102].
- `tool`: Errors occurring during tool execution[cite: 102].
- `auth`: Authentication and authorization failures[cite: 102].
- `config`: Configuration-related issues[cite: 102].
- `rpc`: JSON-RPC protocol errors[cite: 102].
- `rtm`: Errors specific to the Remember The Milk API[cite: 102].

## 3. Error Codes

We use a combination of standard JSON-RPC 2.0 error codes and custom application-specific codes[cite: 95, 97, 102]:

| Category | Code   | Constant               | Usage                        |
| :------- | :----- | :--------------------- | :--------------------------- |
| RPC      | -32700 | `CodeParseError`       | JSON parsing errors          |
| RPC      | -32600 | `CodeInvalidRequest`   | Malformed requests           |
| RPC      | -32601 | `CodeMethodNotFound`   | Method doesn't exist         |
| RPC      | -32602 | `CodeInvalidParams`    | Parameter validation errors  |
| RPC      | -32603 | `CodeInternalError`    | Runtime/system errors        |
| Resource | -32000 | `CodeResourceNotFound` | Resource not found           |
| Tool     | -32001 | `CodeToolNotFound`     | Tool not found               |
| RPC      | -32002 | `CodeInvalidArguments` | Invalid arguments            |
| Auth     | -32003 | `CodeAuthError`        | Authentication errors        |
| RTM      | -32004 | `CodeRTMError`         | Remember The Milk API errors |
| RPC      | -32005 | `CodeTimeoutError`     | Operation timeouts           |

## 4. Error Creation Patterns

Follow these patterns when creating errors:

### 4.1. Resource Errors

```go
   cgerr.NewResourceError(
       fmt.Sprintf("Failed to [action] resource '%s'", name),
       err, // original error or nil
       map[string]interface{}{
           "resource_name": name,
           "args": args,
           // other relevant properties
       }
   )

   // Example
   return cgerr.NewResourceError(
       fmt.Sprintf("failed to read resource '%s'", name),
       err,
       map[string]interface{}{
           "resource_name": name,
           "args": args,
       }
   )
```
````

### 4.2. Tool Errors

```go
   cgerr.NewToolError(
       fmt.Sprintf("Failed to [action] tool '%s'", name),
       err, // original error or nil
       map[string]interface{}{
           "tool_name": name,
           "args": args,
           // other relevant properties
       }
   )

   // Example
   return cgerr.NewToolError(
       fmt.Sprintf("failed to execute tool '%s'", name),
       err,
       map[string]interface{}{
           "tool_name": name,
           "args": args,
       }
   )
```

### 4.3. Validation Errors

```go
   cgerr.NewInvalidArgumentsError(
       fmt.Sprintf("Invalid [parameter]: [reason]"),
       map[string]interface{}{
           "parameter": paramName,
           "value": value,
           // other relevant properties
       }
   )

   // Example
   return cgerr.NewInvalidArgumentsError(
       "invalid frob format: must be alphanumeric",
       map[string]interface{}{
           "argument": "frob",
           "expected": "alphanumeric string",
           "got": frob,
       }
   )
```

### 4.4. Authentication Errors

```go
   cgerr.NewAuthError(
       fmt.Sprintf("[Authentication failure description]"),
       err, // original error or nil
       map[string]interface{}{
           // relevant properties
       }
   )

   // Example
   return cgerr.NewAuthError(
       "failed to validate authentication token",
       err,
       map[string]interface{}{
           "token_path": s.storage.TokenPath,
       }
   )
```

### 4.5. General Errors

```go
   // Creation
   err := errors.Newf("functionName: [error description]")
   err = errors.Wrapf(origError, "functionName: [error description]")

   // Property addition
   err = errors.WithDetail(err, fmt.Sprintf("key:%v", value))

   // Error marking
   err = errors.Mark(err, ErrSentinel)
```

## 5. Error Checking

Always use `errors.Is` to check for sentinel errors:

```go
   // Before
   if err == ErrResourceNotFound {
       // Handle error
   }

   // After
   if errors.Is(err, ErrResourceNotFound) {
       // Handle error
   }
```

## 6. Additional Guidelines

- Include the function name in the error message[cite: 95].
- When wrapping errors, use `%w` with `errors.Wrap` or `errors.Wrapf`, not `fmt.Errorf`[cite: 95, 96].
- Add as many relevant properties as possible to errors[cite: 95, 96].
- For JSON-RPC errors, use the `cgerr.ToJSONRPCError` function to ensure proper formatting and prevent leaking sensitive information[cite: 94].
- Consider adding cow-themed puns or jokes to error messages, where appropriate and infrequent[cite: 98].

## 7. Example

```go
   // Bad
   return fmt.Errorf("couldn't connect")

   // Good
   return cgerr.NewAuthError(
       "Failed to establish connection to RTM",
       err,
       map[string]interface{}{
           "rtm_api_key": c.APIKey,
           "rtm_user": os.Getenv("RTM_USER"), // Example - be careful with secrets
       },
   )
```
