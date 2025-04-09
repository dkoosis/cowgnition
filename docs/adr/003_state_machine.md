# Technical Assessment: Using an FSM for MCP Connection State Management

**Status:** Proposed for Consideration

**Date:** 2025-04-09

## 1. Context

- **Current State:** The server (`internal/mcp/mcp_server.go`) processes incoming MCP messages sequentially within a connection loop. Protocol state (e.g., initialized, awaiting response) is managed implicitly through the flow of control and checks within handlers. There is no explicit state machine enforcing the sequence of operations defined by the MCP specification (e.g., ensuring `initialize` occurs before `tools/call`).
- **Validation:** The existing validation middleware (`internal/middleware/validation.go`) focuses on validating the _structure_ of individual messages against the JSON schema, not the _sequence_ or _context_ in which they arrive.
- **Problem:** Relying on implicit state management can lead to:
  - Difficulty enforcing strict protocol sequences, potentially allowing invalid operations depending on the connection phase.
  - Less clear error reporting when sequence violations occur.
  - Potentially complex conditional logic spread across handlers or the main loop to check the implicit state.
  - Challenges in reasoning about the connection's exact state at any given point.

## 2. Proposed Solution (Conceptual)

Integrate a Go FSM library (e.g., "stateless" as previously discussed, or alternatives like `looplab/fsm`) to explicitly model and manage the state of each MCP client connection.

- **State Machine Definition:** Define distinct states representing the MCP connection lifecycle (e.g., `Uninitialized`, `Initializing`, `Ready`, `ProcessingToolCall`, `AwaitingResponse`, `Closing`).
- **Events/Triggers:** Define triggers corresponding to receiving specific MCP messages (e.g., `ReceiveInitialize`, `ReceiveToolCall`, `ReceiveNotification`) or internal server events (e.g., `SendResponse`, `Timeout`).
- **Transitions:** Configure valid transitions between states based on triggers (e.g., `Uninitialized` -> `Initializing` on `ReceiveInitialize`). Define actions (like calling specific handlers) associated with transitions or state entries/exits.
- **Integration:** Instantiate an FSM for each client connection. Incoming messages would be fed as events/triggers to the connection's FSM. The FSM's current state would dictate whether the message is valid in the current context and which actions (like calling handlers or sending responses) are permitted.

## 3. Consequences / Assessment Factors

#### Architecture Change Implications

- **Connection Management:** Requires instantiating and managing an FSM instance per client connection within `internal/mcp/mcp_server.go`. Adds statefulness to connection handling.
- **Message Routing:** The primary message loop would likely interact with the FSM first (e.g., `fsm.Fire(eventName, messageData)`). The FSM transition logic would then invoke the appropriate handlers (e.g., `handleToolsCall`) or middleware chain.
- **Handler Interaction:** Handlers might become simpler as they could potentially assume the connection is in a valid state for their operation (enforced by the FSM). Alternatively, handlers might need to interact with the FSM to trigger state changes upon completion.
- **Middleware Interaction:** The FSM would likely sit _after_ the structural validation middleware but _before_ the core method dispatch/routing logic. Middleware might need context about the FSM state or vice-versa.
- **Error Handling:** State transition errors (e.g., receiving a message invalid for the current state) would need mapping to appropriate JSON-RPC errors.

#### Reliability

- **Pros:**
  - Significantly improves protocol sequence enforcement, reducing the chance of processing messages out of order or in an invalid context.
  - Provides a clear mechanism for rejecting invalid sequences with specific errors.
  - Centralizes state transition logic, potentially reducing bugs compared to scattered conditional checks.
- **Cons:**
  - Introduces complexity in defining the FSM; errors in the state machine definition itself could lead to incorrect behavior.
  - Requires careful handling of concurrent events or messages if the FSM transitions involve asynchronous operations.

#### Readability/Understandability

- **Pros:**
  - Explicitly defined states and transitions can make the intended protocol flow much clearer than implicit logic spread throughout the code. Visualizing the FSM diagram can aid understanding.
- **Cons:**
  - Requires developers to understand FSM concepts and the specific library chosen.
  - The FSM definition itself can become large and complex for protocols with many states/transitions.
  - Debugging might involve tracing FSM state changes in addition to code execution flow.

#### Maintainability

- **Pros:**
  - Centralizes state logic, making it potentially easier to modify or extend the protocol handling (e.g., adding a new state for a specific operation) by updating the FSM definition.
  - Clear separation between state management (FSM) and action implementation (handlers).
- **Cons:**
  - Changes to the protocol might require significant updates to the FSM definition (states, events, transitions).
  - Adds a dependency on the chosen FSM library, requiring updates and maintenance alongside it.

#### Developer Experience

- **Pros:**
  - Can provide clearer boundaries and expectations for handler implementations (pre-conditions based on state).
  - Abstracting state logic can simplify other parts of the codebase.
- **Cons:**
  - Adds a learning curve for the FSM library and concepts.
  - May require more upfront design effort to define the state machine correctly.
  - Testing needs to include validating state transitions in addition to handler logic.

#### Performance

- **Overhead:** Introducing an FSM adds some computational overhead per message/event to determine the current state, check valid transitions, and execute entry/exit/transition actions.
- **Impact:** For typical FSM libraries and protocols like MCP, this overhead is generally expected to be very small (microseconds) compared to network I/O, JSON parsing/marshalling, schema validation, and actual handler logic (like RTM API calls). It's unlikely to be a significant performance bottleneck unless the FSM logic itself becomes extremely complex or involves blocking operations (which should be avoided in transition actions). Memory usage increases slightly due to storing the state machine instance per connection.
