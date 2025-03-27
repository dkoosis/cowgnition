# CowGnition Implementation Roadmap

## 1. Core JSON-RPC Implementation

- [x] Create dedicated `internal/jsonrpc` package:

  - [x] Implement message parsing/validation (JSON-RPC 2.0 spec)
  - [x] Define request/response/notification structures
  - [x] Add proper error handling with standard codes
  - [ ] Implement timeout management
  - [x] Reference: [JSON-RPC 2.0 Specification](https://www.jsonrpc.org/specification)

- [x] Create message dispatcher:
  - [x] Method registration mechanism
  - [x] Request routing to appropriate handlers
  - [x] Response generation with proper ID matching
  - [ ] Notification handling

## 2. MCP Protocol Compliance

- [x] Update MCP server to use JSON-RPC core:

  - [x] Reimplement handlers using the JSON-RPC package
  - [x] Ensure all messages follow JSON-RPC 2.0 format
  - [x] Implement proper error responses
  - [ ] Add validation for MCP-specific message formats
  - [x] Reference: [MCP Specification](https://spec.modelcontextprotocol.io/)

- [x] Implement transport layer:

  - [x] Add proper stdio transport support
  - [ ] Add SSE/HTTP transport support
  - [x] Implement connection lifecycle management
  - [x] Reference: [MCP Transport Specification](https://spec.modelcontextprotocol.io/specification/2024-11-05/basic/transport/)

- [ ] Update initialization flow:
  - [ ] Implement proper capability negotiation
  - [ ] Add protocol version validation
  - [ ] Ensure proper shutdown procedure
  - [ ] Reference: [MCP Lifecycle](https://spec.modelcontextprotocol.io/specification/2024-11-05/basic/lifecycle/)

## 3. RTM API Integration

- [ ] Complete RTM Task Resources:

  - [ ] Implement list resources for viewing tasks
  - [ ] Add resources for searching tasks
  - [ ] Implement tag resources
  - [ ] Reference: [RTM API Methods](https://www.rememberthemilk.com/services/api/)

- [ ] Implement RTM Tools:
  - [ ] Task creation tool
  - [ ] Task completion tool
  - [ ] Task update tool (due dates, priority)
  - [ ] Tag management tool

## 4. Configuration Enhancements

- [ ] Implement file-based configuration (YAML/TOML)
- [ ] Add validation for configuration values
- [ ] Support for environment variable overrides (expand current implementation)
- [ ] Create configuration documentation

## 5. Testing & Quality Assurance

- [ ] Add comprehensive tests:

  - [ ] Unit tests for all packages
  - [ ] Integration tests for MCP server
  - [ ] Test RTM API interaction (with mocks)
  - [ ] End-to-end tests with MCP Inspector

- [ ] Implement structured logging and diagnostics:
  - [ ] Add consistent structured logging throughout
  - [ ] Log all errors with appropriate context
  - [ ] Add request/response logging for debugging
  - [ ] Implement log levels for production/development

## 6. Security Enhancements

- [ ] API key and token management:

  - [ ] Implement secure token storage (improve current implementation)
  - [ ] Add token rotation support
  - [ ] Implement rate limiting
  - [ ] Add request validation

- [ ] Security auditing:
  - [ ] Audit dependencies
  - [ ] Review authentication flow
  - [ ] Validate input sanitization

## 7. Documentation & User Experience

- [ ] Error messages:

  - [ ] Ensure all error messages are user-friendly
  - [ ] Add detailed developer error context
  - [ ] Implement consistent error formatting

- [ ] Documentation:
  - [ ] Add API documentation with OpenAPI/Swagger
  - [ ] Include examples for common operations
  - [ ] Document error codes and solutions
  - [ ] Create usage guides for client applications

## 8. Code Quality Improvements

- [ ] Enhance error message specificity:

  - [ ] Improve error context in `internal/mcp/tool.go`
  - [ ] Add more details to error messages in resource providers

- [ ] Expand documentation:

  - [ ] Add comprehensive documentation to `internal/mcp/types.go`
  - [ ] Add usage examples where appropriate

- [ ] Implement sophisticated error handling:
  - [ ] Create domain-specific error types for better error identification
  - [ ] Implement error wrapping with additional context
  - [ ] Add error codes and categorization
  - [ ] Consider implementing sentinel errors for key failure conditions

## Implementation Strategy

1. Focus on one component at a time, getting it fully working before moving on
2. Start with core JSON-RPC implementation as the foundation
3. Build MCP protocol compliance on top of that foundation
4. Add RTM functionality incrementally
5. Use the MCP Inspector tool to test each component
6. Write tests for each component as we develop

## Testing Approach

- Use MCP Inspector for manual testing
- Write unit tests for each package
- Implement integration tests for end-to-end validation
- Test with real Claude Desktop integration
- Follow test-driven development where possible

## Completed Items

- Basic MCP server framework established
- RTM authentication resource implemented
- Clean architecture foundation with separation of concerns
- Added stdio transport implementation
- Updated server to support both HTTP and stdio transports
- Added command-line flags for transport selection

# Assess More Sophisticated Error Handling System

A more sophisticated error handling system would go beyond simple error returns and include:

1. **Domain-Specific Error Types**: Instead of using generic errors, create specific error types for different domains of the application. For example:

   ```go
   type RTMAuthError struct {
       Code    int
       Message string
       Cause   error
   }

   func (e *RTMAuthError) Error() string {
       return fmt.Sprintf("RTM authentication error (%d): %s", e.Code, e.Message)
   }

   func (e *RTMAuthError) Unwrap() error {
       return e.Cause
   }
   ```

````

2. **Error Categorization**: Group errors into categories to make handling more consistent:

   ```go
   const (
       ErrCategoryAuth      = "authentication"
       ErrCategoryResource  = "resource"
       ErrCategoryTool      = "tool"
       ErrCategoryTransport = "transport"
   )

   type CategorizedError struct {
       Category string
       Message  string
       Cause    error
   }
   ```

3. **Error Wrapping Chain**: Implement a consistent pattern of wrapping errors with context as they travel up the call stack:

   ```go
   func ReadResource() error {
       result, err := provider.FetchData()
       if err != nil {
           return fmt.Errorf("%w: %s at %s",
               ErrResourceFetch,
               err.Error(),
               time.Now().Format(time.RFC3339))
       }
       // ...
   }
   ```

4. **Sentinel Errors**: Define package-level sentinel errors for known failure conditions that can be checked with `errors.Is()`:

   ```go
   var (
       ErrInvalidToken       = errors.New("invalid authentication token")
       ErrTokenExpired       = errors.New("authentication token expired")
       ErrResourceUnavailable = errors.New("resource temporarily unavailable")
   )
   ```

5. **Error Response Mapping**: Create a consistent system for mapping internal errors to external error responses, preserving helpful details while hiding sensitive information:
   ```go
   func mapErrorToResponse(err error) ErrorResponse {
       var authErr *RTMAuthError
       if errors.As(err, &authErr) {
           return ErrorResponse{
               Code:    "AUTH_ERROR",
               Message: "Authentication failed",
               Details: authErr.Message, // Only if not sensitive
           }
       }
       // Other error types...
   }
   ```

This approach would provide better error diagnostics, make error handling more consistent throughout the codebase, and improve the developer experience when troubleshooting issues.
````

## Here is a Prompt to Implement Error Management with cockroachdb/errors

**AI Prompt: Project-Wide Go Error Handling Enhancement with cockroachdb/errors & MCP Compliance**

**Role:** You are an expert Go developer specializing in robust error handling practices, large-scale code refactoring, and implementing JSON-RPC 2.0 based protocols like MCP.

**Goal:** Systematically refactor the provided Go project files (`.go`) to replace standard error handling (`fmt.Errorf`, `errors.New`, simple error returns) with enhanced, consistent practices using the `github.com/cockroachdb/errors` library. The aim is to significantly improve debuggability via stack traces, add structured context using properties, ensure consistent error propagation, AND ensure the internal error structure facilitates creating **compliant JSON-RPC 2.0 / MCP error responses** at the appropriate boundaries.

**Input:** You will be provided with the content of multiple Go source files (`.go`) constituting a project or a significant package, including (or alongside) files responsible for handling API requests/responses (e.g., HTTP handlers, JSON-RPC endpoints).

**Core Task:** Analyze the provided Go files, identify areas where errors are created or propagated, and refactor them according to the patterns below using `cockroachdb/errors`.

**Key Refactoring Patterns to Apply Consistently:**

1.  **Error Origins (Where errors are first created):**

    - Replace `fmt.Errorf("message %v", args...)` or `errors.New("message")` with `errors.Errorf("message %v", args...)` or `errors.Newf("message %v", args...)`.
      - **Benefit:** Captures a stack trace automatically.
    - Identify relevant local variables providing context (invalid inputs, IDs, states).
    - Attach this context as structured data using `errors.WithProperty(err, "key_name", variable)`. Use clear, consistent key names (e.g., `invalid_parameter`, `resource_id`, `external_service_status`).
      - **Benefit:** Makes context programmatically accessible for logging, internal handling, and importantly, _determining_ the correct JSON-RPC error details later.

2.  **Error Propagation (Returning errors up the stack):**

    - Replace simple `return err` _if context should be added_.
    - Replace `fmt.Errorf("context: %w", err)` or `fmt.Errorf("context: %v", err)` with `errors.Wrapf(err, "context message %v", args...)` or `errors.Wrap(err, "context message")`.
      - **Benefit:** Preserves original error type, adds higher-level context, and captures a stack trace at the wrapping site, creating a clear chain.

3.  **Imports:** Ensure `"github.com/cockroachdb/errors"` is correctly added to the import block of every modified file.

**Crucial Constraint - API/Service Boundaries & JSON-RPC/MCP Compliance:**

- Internal error details (full stack traces from `%+v`, potentially sensitive property values, verbose internal messages) **MUST NOT** leak across public API boundaries or service interfaces.
- **Specifically for JSON-RPC 2.0 / MCP Responses:** When translating internal Go errors into JSON-RPC error objects (likely happening in your HTTP handler or RPC endpoint code):
  - The boundary code MUST generate responses adhering strictly to the **`{ "code": integer, "message": string, "data": optional_value }`** structure.
  - Use standard codes (`-32700` to `-32603`) for JSON-RPC protocol issues (ParseError, InvalidRequest, etc.).
  - Use custom codes in the range **`-32000` to `-32099`** for all your application-specific errors (e.g., RTM AuthError, ResourceError, ValidationError). Refer to your defined `ErrorCode` constants.
  - The external `message` string must be concise, user-appropriate (if applicable), and contain NO internal implementation details or stack traces.
  - The external `data` field can contain structured, non-sensitive details derived from the internal error, adhering to any specific MCP/JMAP requirements if applicable.
  - **Guidance for Implementation:** The properties attached internally using `errors.WithProperty` are **key** here. Your boundary handling code should **inspect** the internal error (using `errors.Is`, `errors.As`, `errors.TryGetProperty`) to _determine_ the correct external `code`, `message`, and `data`. The raw internal error or its properties should **not** be directly placed into the external response fields. This refactoring should make it _easier_ to write that boundary logic correctly.
- **Logging:** Use `fmt.Printf("%+v", err)` or integrate with structured logging libraries for detailed **server-side** diagnostics _only_. Log the full internal error before translating it for the external response.
- **External Errors:** Use `err.Error()` _only if confirmed safe_ (rarely the case for detailed internal errors), or preferably use sanitized messages and data derived from inspecting the error.

**Output:**

1.  **Full Content of ALL Modified `.go` Files:** Provide the complete source code for each file that was changed.
2.  **Summary of Changes:** Include a brief summary describing:
    - The general refactoring patterns applied.
    - How the changes support creating compliant JSON-RPC 2.0 / MCP error responses (e.g., "added properties like `resource_id` to errors originating in `resource_store.go` to facilitate generating `ResourceError` codes and data at the handler").
    - Key benefits achieved (stack traces, structured context, consistent wrapping, easier boundary mapping).
    - Any notable challenges or assumptions made.
