// Package fsm provides a generic Finite State Machine implementation wrapper.
// It defines interfaces for states and events and wraps an underlying FSM library.
// file: internal/fsm/fsm.go
package fsm

import (
	"context"
	"strings"
	"sync" // Added import for mutex.

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	lfsm "github.com/looplab/fsm" // Use alias 'lfsm'
)

// State represents a state in the FSM.
type State string

// Event represents an event that can trigger a state transition.
type Event string

// TransitionAction defines the function signature for actions executed during transitions.
// It receives the context, the triggering event, and optional data.
type TransitionAction func(ctx context.Context, event Event, data interface{}) error

// GuardCondition defines the function signature for guard conditions on transitions.
// It receives the context, the triggering event, and optional data, returning true if the transition is allowed.
type GuardCondition func(ctx context.Context, event Event, data interface{}) bool

// Transition defines a transition rule between states.
// Now supports multiple 'From' states to better align with looplab/fsm.
type Transition struct {
	From      []State          // Source states for this transition.
	To        State            // The destination state.
	Event     Event            // The event triggering the transition.
	Action    TransitionAction // Optional action to execute on entering 'To' state due to this event.
	Condition GuardCondition   // Optional guard condition to check before allowing the event.
}

// FSM defines the interface for our finite state machine wrapper.
type FSM interface {
	// AddTransition stores a transition definition. Call Build() after adding all transitions.
	AddTransition(transition Transition) FSM
	// Build finalizes the FSM configuration and creates the underlying machine. Must be called after AddTransition(s).
	Build() error
	// CurrentState returns the current state. Requires Build() to have been called successfully.
	CurrentState() State
	// CanTransition checks if the event is defined for the current state. Requires Build().
	CanTransition(event Event) bool
	// Transition attempts to trigger a state transition. Requires Build().
	Transition(ctx context.Context, event Event, data interface{}) error
	// SetState allows manually setting the FSM state (use with caution). Requires Build().
	SetState(state State) error
	// Reset sets the state back to the initial state. Requires Build().
	Reset() error
}

// loopFSM implements the FSM interface using looplab/fsm.
type loopFSM struct {
	initialState State
	logger       logging.Logger
	transitions  []Transition
	fsm          *lfsm.FSM    // Underlying instance, nil until Build() is called.
	buildErr     error        // Stores error from Build().
	mu           sync.RWMutex // Protects access to fsm instance and buildErr.
	// These maps are now used only during the Build() process.
	callbackMap  lfsm.Callbacks
	eventDescMap map[string]lfsm.EventDesc
}

// NewFSM creates a new FSM builder instance with the specified initial state and logger.
// Call AddTransition() to define transitions, then call Build() to finalize.
func NewFSM(initialState State, logger logging.Logger) FSM {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	return &loopFSM{
		initialState: initialState,
		logger:       logger.WithField("component", "fsm_wrapper"),
		transitions:  make([]Transition, 0),
		// Don't initialize callbackMap/eventDescMap here, done in Build.
	}
}

// AddTransition stores a transition definition to be used during Build().
func (l *loopFSM) AddTransition(t Transition) FSM {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fsm != nil {
		l.logger.Error("Cannot AddTransition after Build() has been called.")
		if l.buildErr == nil {
			l.buildErr = errors.New("cannot AddTransition after Build")
		}
		return l
	}
	if len(t.From) == 0 {
		l.logger.Error("Transition definition missing 'From' states.", "event", t.Event, "to", t.To)
		if l.buildErr == nil {
			l.buildErr = errors.New("transition definition missing 'From' states")
		}
		return l // Prevent adding invalid transition.
	}
	l.transitions = append(l.transitions, t)
	l.logger.Debug("Stored transition definition.", "event", t.Event, "from", t.From, "to", t.To)
	return l
}

// Build finalizes the FSM configuration and creates the underlying looplab/fsm instance.
func (l *loopFSM) Build() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.fsm != nil {
		l.logger.Warn("Build() called again on an already built FSM.")
		return l.buildErr // Return previous build error, if any.
	}
	if l.buildErr != nil {
		l.logger.Error("Attempted to Build() FSM with configuration errors.", "error", l.buildErr)
		return l.buildErr // Return previous configuration error.
	}
	if len(l.transitions) == 0 {
		l.logger.Warn("Building FSM with no transitions defined.")
		// Proceed, but log warning. Might be valid in some simple cases.
	}

	l.logger.Info("Building FSM instance...", "initialState", l.initialState, "transition_count", len(l.transitions))

	// Reset build maps.
	l.callbackMap = make(lfsm.Callbacks)
	l.eventDescMap = make(map[string]lfsm.EventDesc)

	// Process stored transitions to prepare EventDesc and Callbacks for looplab/fsm.
	processedEvents := make(map[Event]struct{}) // Track events processed for callbacks.

	for i, t := range l.transitions {
		// --- Prepare EventDesc ---
		eventName := string(t.Event)
		toStateStr := string(t.To)
		fromStatesStr := make([]string, len(t.From))
		for j, s := range t.From {
			fromStatesStr[j] = string(s)
		}

		// looplab/fsm expects one EventDesc per unique Name.
		// All transitions for that event name must be listed in its Src array.
		// We'll build this up.
		desc, exists := l.eventDescMap[eventName]
		if !exists {
			desc = lfsm.EventDesc{Name: eventName, Dst: toStateStr} // First time seeing this event.
		} else if desc.Dst != toStateStr {
			// looplab/fsm's EventDesc has only one Dst. This implies our model
			// or looplab's doesn't support one event leading to different destinations
			// based on source state directly within a single EventDesc.
			// We might need separate event names or more complex callback logic if that's needed.
			// For now, log an error and potentially fail the build.
			err := errors.Newf("conflicting destinations ('%s' and '%s') for the same event ('%s'). Define separate events or use guards.", desc.Dst, toStateStr, eventName)
			l.logger.Error("Invalid FSM configuration.", "error", err)
			l.buildErr = err
			return l.buildErr
		}
		desc.Src = append(desc.Src, fromStatesStr...)
		l.eventDescMap[eventName] = desc

		// --- Prepare Callbacks (only once per event name/state) ---
		if _, alreadyProcessed := processedEvents[t.Event]; !alreadyProcessed {
			// Guard Condition -> before_<EVENT>
			if t.Condition != nil {
				callbackName := "before_" + eventName
				if _, cbExists := l.callbackMap[callbackName]; cbExists {
					l.logger.Warn("Overwriting existing 'before' callback. Multiple transitions use the same event name with conditions.", "event", eventName)
				}
				l.callbackMap[callbackName] = l.createGuardCallback(t) // Pass the specific transition 't'.
			}

			// Action -> enter_<STATE> (applied generally, filtered inside)
			if t.Action != nil {
				enterCallbackName := "enter_" + toStateStr
				// Chain actions if multiple transitions enter the same state.
				originalEnterCallback := l.callbackMap[enterCallbackName]
				// Store action associated with this specific transition rule index.
				l.callbackMap[enterCallbackName] = l.createActionCallback(i, originalEnterCallback)
			}
			processedEvents[t.Event] = struct{}{} // Mark this event name as processed for callbacks.
		} else {
			// If event already processed for callbacks, handle potential action chaining for enter_<STATE>.
			if t.Action != nil {
				enterCallbackName := "enter_" + toStateStr
				originalEnterCallback := l.callbackMap[enterCallbackName]
				l.callbackMap[enterCallbackName] = l.createActionCallback(i, originalEnterCallback)
			}
		}
	}

	// Convert map to slice for NewFSM.
	finalEvents := make([]lfsm.EventDesc, 0, len(l.eventDescMap))
	for _, desc := range l.eventDescMap {
		// Deduplicate Src states just in case.
		uniqueSrc := make(map[string]struct{})
		dedupedSrc := make([]string, 0, len(desc.Src))
		for _, s := range desc.Src {
			if _, exists := uniqueSrc[s]; !exists {
				uniqueSrc[s] = struct{}{}
				dedupedSrc = append(dedupedSrc, s)
			}
		}
		desc.Src = dedupedSrc
		finalEvents = append(finalEvents, desc)
	}

	// Create the underlying looplab/fsm instance.
	l.fsm = lfsm.NewFSM(string(l.initialState), finalEvents, l.callbackMap)
	l.logger.Info("FSM instance built successfully.")
	return nil // Success.
}

// createGuardCallback creates a looplab/fsm callback function for a guard condition.
func (l *loopFSM) createGuardCallback(t Transition) lfsm.Callback {
	// This callback is attached to "before_<EVENT>"
	return func(ctx context.Context, e *lfsm.Event) {
		// looplab calls the 'before_' callback regardless of the source state.
		// We must check if the *actual* source state matches one of the sources
		// defined in *this specific transition* (t.From).
		isRelevantSource := false
		for _, srcState := range t.From {
			if e.Src == string(srcState) {
				isRelevantSource = true
				break
			}
		}

		// Only evaluate the guard if the event is transitioning from one of the states
		// specified in *this* transition definition.
		if isRelevantSource {
			var eventData interface{}
			if len(e.Args) > 0 {
				eventData = e.Args[0]
			}

			l.logger.Debug("Checking guard condition.", "event", t.Event, "from", e.Src, "to", t.To)
			if !t.Condition(ctx, t.Event, eventData) {
				l.logger.Debug("Guard condition failed, cancelling transition.", "event", t.Event, "from", e.Src)
				// Use the specific transition's 'From' state in the error message for clarity.
				e.Cancel(errors.Newf("guard condition for event '%s' from state '%s' failed", t.Event, e.Src))
			} else {
				l.logger.Debug("Guard condition passed.", "event", t.Event, "from", e.Src)
			}
		}
		// If the source state didn't match t.From, this guard doesn't apply, so we don't cancel.
	}
}

// createActionCallback creates a looplab/fsm callback function for a transition action.
// It uses the transition index to look up the correct action and condition.
func (l *loopFSM) createActionCallback(transitionIndex int, nextCallback lfsm.Callback) lfsm.Callback {
	// This callback is attached to "enter_<STATE>"
	return func(ctx context.Context, e *lfsm.Event) {
		// Find the specific transition definition that triggered this entry event.
		// We need to check Event name and Source state.
		var matchedTransition *Transition
		l.mu.RLock() // Lock for reading transitions.
		for i := range l.transitions {
			// Use the index to find the exact transition rule this callback corresponds to.
			if i == transitionIndex {
				// Check if the event name and source state match the current event `e`.
				isRelevantSource := false
				for _, fromState := range l.transitions[i].From {
					if string(fromState) == e.Src {
						isRelevantSource = true
						break
					}
				}
				// Check if the event name matches.
				if string(l.transitions[i].Event) == e.Event && isRelevantSource {
					matchedTransition = &l.transitions[i]
					break
				}
			}
		}
		l.mu.RUnlock() // Unlock after reading.

		// Execute the action only if we found the matching transition rule.
		if matchedTransition != nil && matchedTransition.Action != nil {
			var eventData interface{}
			if len(e.Args) > 0 {
				eventData = e.Args[0]
			}
			l.logger.Debug("Executing transition action.", "event", matchedTransition.Event, "to_state", matchedTransition.To, "from_state", e.Src)
			err := matchedTransition.Action(ctx, matchedTransition.Event, eventData)
			if err != nil {
				l.logger.Error("Error executing transition action.", "event", matchedTransition.Event, "to_state", matchedTransition.To, "error", err)
				// Cannot easily roll back state here.
			}
		} else if matchedTransition != nil && matchedTransition.Action == nil {
			// Log if entry was due to this transition but it had no action.
			l.logger.Debug("Entered state via transition with no action.", "event", e.Event, "from_state", e.Src, "to_state", e.Dst)
		} else {
			// This can happen if multiple transitions lead to the same state,
			// and the one that actually triggered didn't have an action, but
			// another one (associated with this callback instance via index) did.
			// Or if the source/event didn't match the transitionIndex rule.
			l.logger.Debug("Entered state, but triggering transition did not match this specific action callback.", "event", e.Event, "from_state", e.Src, "to_state", e.Dst, "transitionIndexChecked", transitionIndex)

		}

		// Call the next callback in the chain if it exists.
		if nextCallback != nil {
			nextCallback(ctx, e)
		}
	}
}

// CurrentState returns the current state of the FSM. Requires Build().
func (l *loopFSM) CurrentState() State {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.fsm == nil {
		l.logger.Error("CurrentState() called before Build() or after build error.")
		return "" // Or a specific "uninitialized" state?
	}
	return State(l.fsm.Current())
}

// CanTransition checks if the given event can trigger a transition from the current state. Requires Build().
func (l *loopFSM) CanTransition(event Event) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.fsm == nil {
		l.logger.Error("CanTransition() called before Build() or after build error.")
		return false
	}
	// Note: looplab's Can() does not evaluate guard conditions.
	return l.fsm.Can(string(event))
}

// Transition triggers a state transition based on the event. Requires Build().
// Optional data can be passed to condition and action callbacks.
func (l *loopFSM) Transition(ctx context.Context, event Event, data interface{}) error {
	l.mu.RLock() // RLock initially to check if built
	if l.fsm == nil {
		l.mu.RUnlock()
		l.logger.Error("Transition() called before Build() or after build error.")
		return l.buildErr // Return build error if it exists
	}
	fsmInstance := l.fsm
	l.mu.RUnlock() // Unlock before potentially long-running Event call

	l.logger.Debug("Attempting transition.", "event", event, "from_state", l.CurrentState())
	var err error
	// Pass data as the first element in Args slice for callbacks
	args := []interface{}{}
	if data != nil {
		args = append(args, data)
	}

	// looplab/fsm's Event method handles thread safety internally.
	err = fsmInstance.Event(ctx, string(event), args...)

	// Error Handling: Check specific looplab/fsm error types.
	if err != nil {
		// Use errors.As for type checking if needed, or string contains for simplicity here.
		errMsg := err.Error()
		if errors.Is(err, &lfsm.NoTransitionError{}) || errors.Is(err, &lfsm.InvalidEventError{}) || errors.Is(err, &lfsm.UnknownEventError{}) {
			l.logger.Warn("Transition failed: Event/Transition not applicable for current state.", "event", event, "from_state", l.CurrentState(), "error", errMsg)
			// Return a more specific error type if needed by caller?
			return errors.Wrap(err, "transition not possible")
		} else if errors.Is(err, &lfsm.CanceledError{}) || strings.Contains(errMsg, "guard condition") {
			l.logger.Info("Transition cancelled by guard condition.", "event", event, "from_state", l.CurrentState())
			return errors.Wrap(err, "transition cancelled by guard condition")
			// --- CORRECTED ERROR TYPE HERE ---
		} else if errors.Is(err, &lfsm.InTransitionError{}) { // Was lfsm.NotTransitioningError
			l.logger.Error("Concurrency error during transition.", "event", event, "error", errMsg)
			// This indicates a potential issue with how the FSM is being used concurrently.
			return errors.Wrap(err, "FSM concurrency error")
		}
		// --- END CORRECTION ---

		// General transition failure.
		l.logger.Error("Transition failed.", "event", event, "from_state", l.CurrentState(), "error", err)
		return errors.Wrapf(err, "failed to transition on event '%s' from state '%s'", event, l.CurrentState())
	}

	l.logger.Debug("Transition successful.", "event", event, "new_state", l.CurrentState())
	return nil
}

// SetState allows manually setting the FSM state. Use with caution. Requires Build().
func (l *loopFSM) SetState(state State) error {
	l.mu.Lock() // Lock needed as we modify the underlying state.
	defer l.mu.Unlock()
	if l.fsm == nil {
		l.logger.Error("SetState() called before Build() or after build error.")
		return l.buildErr // Or return a specific error.
	}
	l.logger.Warn("Manually setting FSM state.", "target_state", state)
	// looplab/fsm SetState doesn't return error, but we check init status.
	l.fsm.SetState(string(state))
	return nil
}

// Reset sets the state back to the initial state. Requires Build().
func (l *loopFSM) Reset() error {
	l.logger.Info("Resetting FSM to initial state.", "initialState", l.initialState)
	// Use SetState to go back to the initial state.
	// Note: This does NOT re-run initial actions, just changes the current state marker.
	return l.SetState(l.initialState)
}
