# CowGnition - MCP Server Implementation Roadmap

This document outlines the implementation steps for the CowGnition MCP server, which connects Claude Desktop to Remember The Milk through the Model Context Protocol. It's structured as a series of prompts optimized for efficient collaboration with an AI coding assistant.

## Optimal Work Session Format

1. **Review**: Begin by reviewing this TODO document and the current state of the repository
2. **Plan**: Identify the next task and break it down into smaller components
3. **Implement**: Write code for one component at a time, following Go best practices
4. **Test**: Verify the implementation works as expected
5. **Document**: Update comments, README, and this TODO document
6. **Commit**: Save changes with descriptive commit messages

## Collaboration Guidelines

When working with AI on this project:

1. Always start by reviewing this TODO document to identify the next tasks
2. Ensure you're working with the current repository state
3. Follow Go best practices:
   - Use comprehensive comments (with function documentation)
   - Implement thorough error handling with wrapping
   - Write idiomatic Go code (using standard libraries when possible)
   - Use consistent naming conventions
   - Create small, focused functions with clear responsibilities
4. Update this TODO document after completing each section
5. Commit changes in logical units with descriptive messages

## Development Workflow

1. Use MCP Inspector for rapid testing:
   ```
   mcp dev --command ./cowgnition --args "serve --config configs/config.yaml"
   ```

2. Monitor logs during development:
   ```
   tail -n 20 -F ~/Library/Logs/Claude/mcp*.log
   ```

3. Test with Claude Desktop:
   - Create `~/Library/Application Support/Claude/developer_settings.json` with `{"allowDevTools": true}`
   - Install server via `mcp install --name "RTM" --command ./cowgnition --args "serve --config configs/config.yaml"`
   - Use Command-Option-Shift-i to open DevTools for debugging
   - Restart Claude Desktop after configuration changes

4. Debugging cycle:
   - Make code changes
   - Run tests (when implemented)
   - Test with Inspector
   - Check logs
   - Verify in Claude Desktop

## Implementation Roadmap

### 1. Core MCP Server Framework

```
Let's implement the core MCP server framework following the Model Context Protocol specifications. We need to:

1. Complete the MCPServer implementation in internal/server/server.go:
   - Fix any issues in the existing implementation
   - Ensure all required MCP endpoints are properly implemented
   - Add proper error handling and logging

2. Implement the main entrypoint in cmd/server/main.go:
   - Load configuration from file
   - Initialize and start the MCP server
   - Set up proper signal handling for graceful shutdown

3. Verify the server implements these core MCP capabilities:
   - initialize: Server configuration and capability declaration
   - list_resources: Listing available resources
   - read_resource: Reading resource content
   - list_tools: Listing available tools
   - call_tool: Executing tool functionality

Focus on creating a clean, modular implementation with well-defined interfaces.
```

### 1a. MCP Development Tooling & Quality Assurance

```
Set up and integrate MCP debugging and quality assurance tools to enhance development workflow:

1. Add structured logging with Zap:
   - Replace standard log package with Zap
   - Implement structured log format with timestamps, request IDs, and severity levels
   - Create custom logging middleware for HTTP handlers
   - Add MCP protocol notification logging

2. Implement Inspector integration for testing:
   - Create test script for MCP Inspector
   - Document Inspector usage workflow in README.md
   - Add common test cases for RTM functionality

3. Improve error diagnostics:
   - Enhance error context in API interactions
   - Implement more robust error recovery in HTTP handlers
   - Add diagnostic logging for initialization and auth flows

4. Set up Claude Desktop development environment:
   - Create developer_settings.json with DevTools enabled
   - Document log file locations and monitoring commands
   - Add development workflow section to README
   - Establish working directory best practices
```

### 2. RTM Authentication Flow

```
Enhance the authentication flow for Remember The Milk. Implement proper frob handling, token management, and authentication persistence. Add support for permission levels and token refresh.
```

### 3. Task Resources Implementation

```
Implement resources for tasks. This includes task listing with filtering (all, today, tomorrow, week), individual task retrieval, and proper formatting for MCP context exposure. Use proper pagination for large task lists.
```

### 4. List Resources Implementation

```
Implement resources for RTM lists. This includes listing all lists, filtering by type (smart lists, regular lists), and retrieving list metadata. Include list statistics like task counts.
```

### 5. Tag Resources Implementation

```
Implement resources for RTM tags. This includes listing all tags, retrieving tasks by tag, and tag usage statistics. Add support for tag hierarchies if available in the RTM API.
```

### 6. Task Management Tools

```
Implement tools for task management: creation, completion, deletion, due date management, and priority setting. Ensure proper error handling and validation for all operations.
```

### 7. List Management Tools

```
Implement tools for list management: creation, deletion, renaming, and archiving. Include support for task movement between lists and list organization.
```

### 8. Tag Management Tools

```
Implement tools for tag management: adding/removing tags from tasks, creating/deleting tags, and renaming tags. Add support for batch operations on multiple tasks.
```

### 9. Note Management Tools

```
Implement tools for note management: adding, editing, and deleting notes on tasks. Support rich text formatting if available in the RTM API.
```

### 10. Search and Filter Tools

```
Implement search and filter tools using RTM's query syntax. Support complex filters (priority, date ranges, text search) with proper formatting of results.
```

### 11. Testing and Documentation

```
Implement comprehensive testing at multiple levels:

1. Unit testing:
   - Create unit tests for core components using Go's testing package
   - Implement mocks for RTM API client
   - Test error handling and edge cases
   - Verify authentication flows
   - Test resource and tool handlers independently

2. Integration testing:
   - Create end-to-end tests using actual MCP protocol
   - Test server initialization and shutdown
   - Verify protocol compliance
   - Test with mock RTM responses

3. MCP Inspector testing:
   - Create test scripts for Inspector
   - Document manual testing procedures
   - Create test cases covering key functionality

4. Documentation:
   - Update README with complete usage examples
   - Create troubleshooting guide
   - Document authentication workflow with screenshots
   - Provide example prompts for Claude to use with RTM
   - Document API status and limitations
```

### 12. Client Integration Support

```
Add specific support for Claude Desktop integration. This includes installation instructions, usage examples, and troubleshooting tips. Create example prompts for common RTM operations.
```

### 13. Security and Performance Optimization

```
Enhance security, reliability, and performance:

1. Security enhancements:
   - Implement token encryption at rest
   - Add request validation and sanitization
   - Implement proper HTTP security headers
   - Sanitize log output to prevent sensitive data exposure
   - Add rate limiting for authentication attempts

2. Performance optimization:
   - Implement response caching for resources
   - Add conditional requests with ETags
   - Optimize large response handling
   - Implement connection pooling for RTM API
   - Profile and optimize hot code paths

3. Reliability improvements:
   - Add circuit breaker for RTM API calls
   - Implement graceful degradation for non-critical features
   - Add automatic recovery for transient errors
   - Implement proper timeout handling

4. Monitoring and telemetry:
   - Add structured logging with correlation IDs
   - Implement metrics collection (requests, errors, latency)
   - Create health check endpoint
   - Add diagnostic tools for debugging
   - Implement OpenTelemetry integration
```

## Completed Tasks

*Move completed tasks here with date of completion*