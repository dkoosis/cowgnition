# ADR 003: State Machine for MCP Connection State (Revised)

- **Status:** Accepted
- **Date:** 2025-04-26 _(Original: 2025-04-09)_

## 1. Context

- **Current State:** The server (`internal/mcp/mcp_server.go`) processes incoming MCP messages sequentially within a connection loop. Protocol state (e.g., initialized, awaiting response) is managed implicitly through the flow of control and checks within handlers. There is no explicit state machine enforcing the sequence of operations defined by the MCP specification (e.g., ensuring `initialize` occurs before `tools/call`).
- **Validation:** The existing validation middleware (`internal/middleware/validation.go`) focuses on validating the _structure_ of individual messages against the JSON schema, not the _sequence_ or _context_ in which they arrive.
- **Problem:** Relying on implicit state management can lead to:
    - Difficulty enforcing strict protocol sequences, potentially allowing invalid operations depending on the connection phase.
    - Less clear error reporting when sequence violations occur.
    - Potentially complex conditional logic spread across handlers or the main loop to check the implicit state.
    - Challenges in reasoning about the connection's exact state at any given point.

## 2. Decision

Integrate the Go FSM library `github.com/looplab/fsm` to explicitly model and manage the state of each MCP client connection.

- **State Machine Definition:** Define distinct states representing the MCP connection lifecycle (e.g., `Uninitialized`, `Initializing`, `Initialized`, `ShuttingDown`, `Shutdown`) using constants in `internal/mcp/state/states.go`.
- **Events/Triggers:** Define triggers corresponding to receiving specific MCP messages (e.g., `EventInitializeRequest`, `EventClientInitialized`) or internal server events (e.g., `EventTransportErrorOccurred`) using constants in `internal/mcp/state/events.go`.
- **Transitions:** Configure valid transitions between states based on triggers using `FSM.AddTransition` during initialization. Define actions and guard conditions associated with transitions as needed.
- **Integration:** Instantiate an FSM (`internal/mcp/state.MCPStateMachine`) for each client connection. Incoming messages will be mapped to FSM events, and the FSM's state will be checked (`MCPStateMachine.ValidateMethod`) before routing the message. The core message handler (`mcp_server.handleMessageWithFSM`) will trigger FSM transitions (`FSM.Transition`).

## 3. Consequences / Assessment Factors

*(Original assessment factors remain largely the same, but the implementation guidance below addresses some potential cons)*

- **Architecture Change Implications:** Requires FSM instance per connection, message routing interacts with FSM first.
- **Reliability:** Improves protocol sequence enforcement, centralizes state logic. Risk exists if FSM definition is flawed.
- **Readability/Understandability:** Explicit states/transitions improve clarity. Requires understanding FSM concepts. Visualization helps.
- **Maintainability:** Centralized state logic aids modification. Protocol changes require FSM updates. Adds library dependency.
- **Developer Experience:** Clearer boundaries for handlers. Learning curve for FSM. Testing requires state transition validation.
- **Performance:** Minor overhead expected from FSM operations, unlikely to be a bottleneck compared to I/O or core logic. Concurrency needs careful handling (see guidance).

## 4. Implementation Guidance for `looplab/fsm`

Based on the library's features and known behaviors, follow these guidelines:

**4.1. FSM Definition & Structure:**

-   **Embed FSM:** Embed `*fsm.FSM` within the `MCPStateMachine` struct (`internal/mcp/state/machine.go`) to couple state with behavior.
-   **Constants:** Use `const` for defining states (`fsm.State`) and events (`fsm.Event`) as done in `internal/mcp/state/states.go` and `internal/mcp/state/events.go`. This improves type safety and maintainability.
-   **Transitions:** Define transitions clearly using `fsm.Transition` structs passed to `AddTransition` during `NewMCPStateMachine`. Ensure `From` is a slice (`[]fsm.State`).

**4.2. Callbacks & Guards:**

-   **Keep Callbacks Concise:** Callbacks (`Action` in `fsm.Transition`) should be short. Delegate complex logic to methods on `MCPStateMachine` or other components.
-   **Guard Conditions:** Implement guards using the `Condition` field in `fsm.Transition`. The condition function receives `ctx`, `event`, and `data`. Alternatively, use `before_<EVENT>` callbacks and call `e.Cancel(optionalError)` within the callback if the transition should be blocked.
-   **Context Propagation:** Pass `context.Context` into `FSM.Transition`. Ensure callbacks respect this context for cancellation and deadlines, especially if performing I/O or async operations. Be mindful of potential issues with `context.WithTimeout` propagation in async scenarios (see GitHub issue #90) and test thoroughly.

**4.3. Concurrency & Asynchronous Operations:**

-   **CRITICAL - Avoid Re-entrant Calls:** **Never** call `FSM.Event` or `FSM.SetState` directly from within *any* FSM callback (`Action`, `Condition`, or lifecycle callbacks like `enter_state`). This **will cause deadlocks** due to the library's internal mutex strategy.
-   **Decouple Callback Actions:** If a callback needs to trigger another state transition, use a separate goroutine, channel, or external event queue to decouple the trigger from the callback's execution context.
-   **Async Transitions (`e.Async()`):** If using `e.Async()` in a `leave_state` callback:
    -   Remember the FSM remains in the *source* state until `FSM.Transition()` is called later.
    -   The library **does not** provide a way to cancel the *user-initiated* asynchronous work. Your application code *must* manage the timeout and cancellation of the async task using the `context.Context`.
    -   Handle the `fsm.AsyncError` returned by `FSM.Event` when `e.Async()` is used.
-   **High Concurrency:** For scenarios with many concurrent events targeting a *single* FSM instance (less likely in the per-connection MCP model but possible elsewhere), be aware that the internal `eventMu` serializes calls. Consider architectural alternatives (one FSM per entity) or external queuing if this becomes a bottleneck.

**4.4. Error Handling (Production Code & Tests):**

-   **Check Return Values:** Always check the `error` returned by `FSM.Transition`.
-   **Use `errors.As`:** Use `errors.As(err, &targetErrorType)` to check for specific `looplab/fsm` error types (e.g., `lfsm.InvalidEventError`, `lfsm.UnknownEventError`, `lfsm.NoTransitionError`, `lfsm.CanceledError`). This is more robust than checking error strings.
-   **Inspect Error Fields:** After identifying the type with `errors.As`, inspect the error's fields (e.g., `invalidErr.Event`, `invalidErr.State`) for specific details.
-   **Map Errors:** In the MCP error handling layer (`internal/mcp/mcp_server_error_handling.go`), map these specific FSM errors to appropriate JSON-RPC 2.0 error codes (e.g., map `lfsm.InvalidEventError` to `-32001 ErrRequestSequence`). Include relevant details (like the FSM error type name) in the `data` field of the JSON-RPC error response for debugging.

**4.5. Testing:**

-   **Error Assertions:** Use `errors.As` in tests to assert that specific FSM errors are returned under the correct conditions (invalid transitions, failed guards, etc.). Avoid `assert.Contains(err.Error(), ...)` for FSM errors.
-   **State Assertions:** Verify the FSM is in the expected state (`fsm.CurrentState()`) after successful or failed transitions.
-   **Callback Verification:** Use mocks or atomic counters to verify that expected `Action` callbacks are executed.
-   **Guard Testing:** Test transitions both when the `Condition` should allow and block the transition, asserting the resulting state and error (`lfsm.CanceledError`).
-   **Concurrency Tests:** If relevant, write tests simulating concurrent calls to `FSM.Event` to check for race conditions or deadlocks (though internal locking should prevent races, deadlocks from callbacks are possible).
-   **Async Tests:** Test the full lifecycle of async transitions, including context cancellation handling for the user-initiated task.

**4.6. Documentation & Visualization:**

-   **Visualize:** Use the library's built-in visualizers (`fsm.Visualize(fsmInstance)` for Graphviz DOT format, or `fsm.VisualizeWithType(fsmInstance, fsm.MermaidStateDiagram)`) to generate diagrams of your state machine.
-   **Embed Diagrams:** Include generated diagrams (e.g., Mermaid diagrams rendered in Markdown) in documentation to clearly communicate the state machine's structure and transitions.
-   **Document States/Events:** Clearly document the purpose of each state and event defined for your specific FSM (e.g., the MCP connection FSM).

By adhering to these guidelines, particularly regarding error handling and concurrency pitfalls, the team can leverage `looplab/fsm` effectively while building a more maintainable and robust state management system for CowGnition's MCP connections.
