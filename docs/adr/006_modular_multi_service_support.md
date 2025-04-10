# ADR-00X: Modular Multi-Service Architecture

- **Status:** Draft
- **Date:** 2025-04-10

## Context

The CowGnition MCP server initially needs to integrate with the Remember The Milk (RTM) service. However, there is a requirement to easily support additional external services (e.g., calendars, issue trackers) in the future without significant refactoring of the core MCP handling logic.

The current architecture (as of the initial RTM implementation phase) centers around a single `internal/mcp.Handler` struct which holds dependencies and potentially clients for all integrated services. This monolithic approach could become difficult to manage, test, and extend as more services are added. Configuration is also currently specific to RTM.

## Decision

Implement a modular, service-oriented architecture based on a standard Go interface.

1.  **Define `Service` Interface:** Create a standard Go interface (e.g., `internal/services.Service`) that all external service integrations must implement. This interface will define methods for:

    - `Initialize(cfg ServiceConfig, logger Logger)`: Configure the service instance.
    - `GetName() string`: Return a unique service identifier (e.g., "rtm").
    - `GetTools() []mcp.Tool`: Return MCP Tools provided by the service.
    - `GetResources() []mcp.Resource`: Return MCP Resources provided by the service.
    - `GetPrompts() []mcp.Prompt`: Return MCP Prompts provided by the service.
    - `CallTool(...) (*mcp.CallToolResult, error)`: Handle tool calls routed to this service.
    - `ReadResource(...) (*mcp.ReadResourceResult, error)`: Handle resource reads routed to this service.
    - `GetPrompt(...) (*mcp.GetPromptResult, error)`: Handle prompt requests routed to this service.
    - `Shutdown() error`: Clean up service resources.

2.  **Service Implementations:** Each external service integration (e.g., RTM, Google Calendar) will be implemented in its own dedicated package (e.g., `internal/rtm/`, `internal/google_calendar/`). Each package will contain a struct implementing the `services.Service` interface and the specific client/API logic for that service.

3.  **Configuration:** Update `internal/config/config.go` to support a list or map of generic `ServiceConfig` objects, identifiable by a `type` field (e.g., "rtm"). Each `ServiceConfig` will contain the necessary details (API keys, endpoints, etc.) for that specific service instance.

4.  **Service Registry:** Implement a central registry (likely managed during server startup in `mcp_server.go` or `server_runner.go`) that:

    - Reads the list of service configurations.
    - Instantiates the appropriate `Service` implementation for each enabled configuration (using a factory or switch based on the `type`).
    - Initializes each service via its `Initialize` method.
    - Stores the active service instances (e.g., in a `map[string]services.Service`).

5.  **Refactor MCP Handlers:** Modify the core MCP handlers (`internal/mcp/handlers_*.go`) to:
    - Utilize the service registry instead of holding direct client references.
    - **Aggregate Capabilities:** Handlers like `handleToolsList`, `handleResourcesList`, `handlePromptsList` will iterate through all registered services, call the respective `Get*` methods, and combine the results. Tool/Resource/Prompt names should likely be prefixed (e.g., `rtm_getTasks`) to ensure uniqueness and aid routing.
    - **Route Requests:** Handlers like `handleToolCall`, `handleResourcesRead`, `handlePromptsGet` will inspect the request (e.g., tool name prefix, resource URI scheme) to identify the target service, retrieve it from the registry, and delegate the call to the service's specific handler method (`CallTool`, `ReadResource`, `GetPrompt`).

## Alternatives Considered

1.  **Monolithic Handler:** Continue adding client instances and logic directly to the main `mcp.Handler`. Rejected due to poor scalability, testability, and separation of concerns.
2.  **Generic Middleware:** Attempting to handle service interactions solely through middleware. Rejected as core service logic (API calls, state) doesn't fit the middleware pattern well; handlers still need access to specific service clients.

## Consequences

- **Positive:**
  - **Modularity:** Service logic is encapsulated within dedicated packages.
  - **Extensibility:** Adding new services involves creating a new package implementing the interface and updating configuration, minimizing changes to core MCP logic.
  - **Testability:** Individual services can be mocked or tested in isolation.
  - **Maintainability:** Clear separation of concerns makes the codebase easier to understand and modify.
  - **Clear Dependencies:** Core MCP handlers depend on the abstract `Service` interface, not concrete implementations.
- **Negative:**
  - **Increased Indirection:** Requests are routed through the registry to the specific service handler, adding a small layer of indirection.
  - **Interface Design:** Requires careful design of the `Service` interface to cover common needs without becoming overly complex.
  - **Configuration Complexity:** Configuration becomes slightly more complex to support a list/map of generic service blocks.
  - **Naming Conventions:** Requires establishing clear naming conventions for tools, resources, and prompts (e.g., using prefixes) to enable routing.
