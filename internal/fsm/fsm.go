package fsm

// file: internal/fsm/fsm.go

import (
	"context" // Keep fmt for errors.Newf.
	"fmt"
	"os" // <<< ADD os import for Printf to stderr.
	"reflect"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	lfsm "github.com/looplab/fsm" // Use alias 'lfsm'.
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
		return l.buildErr
	}
	if l.buildErr != nil {
		l.logger.Error("Attempted to Build() FSM with configuration errors.", "error", l.buildErr)
		return l.buildErr
	}
	if len(l.transitions) == 0 {
		l.logger.Warn("Building FSM with no transitions defined.")
	}

	l.logger.Info("Building FSM instance...", "initialState", l.initialState, "transition_count", len(l.transitions))

	l.callbackMap = make(lfsm.Callbacks)
	l.eventDescMap = make(map[string]lfsm.EventDesc)
	processedEvents := make(map[Event]struct{})

	for i, t := range l.transitions {
		eventName := string(t.Event)
		toStateStr := string(t.To)
		fromStatesStr := make([]string, len(t.From))
		for j, s := range t.From {
			fromStatesStr[j] = string(s)
		}

		desc, exists := l.eventDescMap[eventName]
		if !exists {
			desc = lfsm.EventDesc{Name: eventName, Dst: toStateStr}
		} else if desc.Dst != toStateStr {
			// Allow multiple 'From' states for the same event->destination pair,
			// but disallow the same event going to different destinations.
			err := errors.Newf("conflicting destinations ('%s' and '%s') for the same event ('%s'). Define separate events or use guards.", desc.Dst, toStateStr, eventName)
			l.logger.Error("Invalid FSM configuration.", "error", err)
			l.buildErr = err
			return l.buildErr
		}
		desc.Src = append(desc.Src, fromStatesStr...)
		l.eventDescMap[eventName] = desc

		// Register callbacks only once per unique event/state combination if needed,
		// or structure based on looplab's callback registration logic.
		// Here, we register based on event name for 'before' and target state for 'enter'.
		if _, alreadyProcessed := processedEvents[t.Event]; !alreadyProcessed {
			// Register 'before' callback (guard) if defined for this event.
			if t.Condition != nil {
				callbackName := "before_" + eventName
				if _, cbExists := l.callbackMap[callbackName]; cbExists {
					// This should ideally not happen if event names are unique triggers to destinations,
					// but log a warning if overwritten.
					l.logger.Warn("Overwriting existing 'before' callback (guard).", "event", eventName)
				}
				l.callbackMap[callbackName] = l.createGuardCallback(t) // Pass the specific transition t.
			}
			processedEvents[t.Event] = struct{}{} // Mark event as having its 'before' callback set up.
		}

		// Register 'enter' state callback (action) if defined.
		// This attaches the action to entering the 'To' state.
		// If multiple transitions lead to the same 'To' state, actions might need
		// internal logic to know which event triggered them if behaviour differs.
		// Our current createActionCallback uses the transition index to differentiate.
		if t.Action != nil {
			enterCallbackName := "enter_" + toStateStr
			originalEnterCallback := l.callbackMap[enterCallbackName] // Get potential existing callback for this state.
			l.callbackMap[enterCallbackName] = l.createActionCallback(i, originalEnterCallback)
		}
	}

	// Finalize Event Descriptions (deduplicate source states).
	finalEvents := make([]lfsm.EventDesc, 0, len(l.eventDescMap))
	for _, desc := range l.eventDescMap {
		uniqueSrc := make(map[string]struct{})
		dedupedSrc := make([]string, 0, len(desc.Src))
		for _, s := range desc.Src {
			if _, exists := uniqueSrc[s]; !exists {
				uniqueSrc[s] = struct{}{}
				dedupedSrc = append(dedupedSrc, s)
			}
		}
		desc.Src = dedupedSrc
		l.logger.Debug("Building event description", "event", desc.Name, "src", desc.Src, "dst", desc.Dst)
		finalEvents = append(finalEvents, desc)
	}

	// Create the underlying FSM instance.
	l.fsm = lfsm.NewFSM(string(l.initialState), finalEvents, l.callbackMap)
	l.logger.Info("FSM instance built successfully.")
	return nil
}

// createGuardCallback creates a looplab/fsm callback function for a guard condition.
func (l *loopFSM) createGuardCallback(t Transition) lfsm.Callback {
	// This callback runs *before* the event happens.
	return func(ctx context.Context, e *lfsm.Event) {
		// Check if the event name matches the transition this callback is for.
		// NOTE: This check might be redundant if looplab calls the correct 'before_event'.
		if e.Event != string(t.Event) {
			return
		}

		// Check if the source state matches one of the 'From' states for this transition.
		isRelevantSource := false
		for _, srcState := range t.From {
			if e.Src == string(srcState) {
				isRelevantSource = true
				break
			}
		}
		if !isRelevantSource {
			l.logger.Debug("Guard check skipped: Event source state does not match this transition's source.",
				"event", t.Event, "actualSrc", e.Src, "expectedFrom", t.From)
			return
		}

		// Extract data passed to Transition() method.
		var eventData interface{}
		if len(e.Args) > 0 {
			eventData = e.Args[0]
		}

		// Execute the actual guard condition function.
		l.logger.Debug("Checking guard condition.", "event", t.Event, "from", e.Src, "to", t.To)
		if !t.Condition(ctx, t.Event, eventData) {
			l.logger.Debug("Guard condition failed, cancelling transition.", "event", t.Event, "from", e.Src)
			// Cancel the transition with a specific error.
			e.Cancel(errors.Newf("guard condition for event '%s' from state '%s' failed", t.Event, e.Src))
		} else {
			l.logger.Debug("Guard condition passed.", "event", t.Event, "from", e.Src)
		}
	}
}

// createActionCallback creates a looplab/fsm callback function for a transition action.
// It chains actions if multiple transitions enter the same state.
func (l *loopFSM) createActionCallback(transitionIndex int, nextCallback lfsm.Callback) lfsm.Callback {
	// This callback runs *after* the transition is complete, upon entering the new state.
	return func(ctx context.Context, e *lfsm.Event) {
		var matchedTransition *Transition
		l.mu.RLock() // Lock for reading transitions slice.
		// Find the specific transition definition that caused entry into this state.
		// This requires matching the event and source state.
		for i := range l.transitions {
			// Check if this is the transition this specific callback was created for.
			if i == transitionIndex {
				isRelevantSource := false
				for _, fromState := range l.transitions[i].From {
					if string(fromState) == e.Src { // Compare event source with transition's source states.
						isRelevantSource = true
						break
					}
				}
				// Ensure the event name and destination state also match the event that just occurred.
				if string(l.transitions[i].Event) == e.Event && isRelevantSource && string(l.transitions[i].To) == e.Dst {
					matchedTransition = &l.transitions[i]
					break
				}
			}
		}
		l.mu.RUnlock()

		// Execute the action if found and defined.
		if matchedTransition != nil && matchedTransition.Action != nil {
			var eventData interface{}
			if len(e.Args) > 0 {
				eventData = e.Args[0]
			}
			l.logger.Debug("Executing transition action.", "event", matchedTransition.Event, "to_state", matchedTransition.To, "from_state", e.Src)
			err := matchedTransition.Action(ctx, matchedTransition.Event, eventData)
			if err != nil {
				// Log action errors but don't cancel the state transition (it already happened).
				l.logger.Error("Error executing transition action.", "event", matchedTransition.Event, "to_state", matchedTransition.To, "error", err)
			}
		} else if matchedTransition != nil && matchedTransition.Action == nil {
			l.logger.Debug("Entered state via transition with no action.", "event", e.Event, "from_state", e.Src, "to_state", e.Dst)
		} else {
			// This might happen if multiple transitions enter the same state, and this callback
			// instance wasn't the one matching the specific triggering transition index.
			l.logger.Debug("Entered state, but triggering transition did not match this specific action callback's index.",
				"event", e.Event, "from_state", e.Src, "to_state", e.Dst, "transitionIndexChecked", transitionIndex)
		}

		// Call the next chained callback if one exists (for other transitions entering this state).
		if nextCallback != nil {
			l.logger.Debug("Calling next chained action callback for state.", "state", e.Dst)
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
		return ""
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
	return l.fsm.Can(string(event))
}

// Transition triggers a state transition based on the event. Requires Build().
func (l *loopFSM) Transition(ctx context.Context, event Event, data interface{}) error {
	l.mu.RLock()
	if l.fsm == nil {
		l.mu.RUnlock()
		l.logger.Error("Transition() called before Build() or after build error.")
		return l.buildErr // Return potential build error.
	}
	fsmInstance := l.fsm
	currentState := State(fsmInstance.Current())
	l.mu.RUnlock()

	canTransitionCheck := fsmInstance.Can(string(event))
	l.logger.Debug("FSM Transition Attempt", "event", event, "from_state", currentState, "can_transition_check", canTransitionCheck)

	var err error
	args := []interface{}{}
	if data != nil {
		args = append(args, data)
	}

	// Call the underlying library's Event method.
	err = fsmInstance.Event(ctx, string(event), args...)

	if err != nil {
		// <<< --- TEMPORARY PRINTF LOGGING --- >>>
		// Use fmt.Fprintf to standard error, which is reliably captured by `go test -v`.
		fmt.Fprintf(os.Stderr, "\n>>> DEBUG: Entered Transition error block. Error: %v (%T)\n", err, err) // Print error and its type.

		errType := reflect.TypeOf(err)
		errKind := errType.Kind()
		errPkgPath := ""

		// Use Elem() if it's a pointer to get underlying type info.
		if errKind == reflect.Ptr {
			errElemType := errType.Elem()
			errPkgPath = errElemType.PkgPath()
			fmt.Fprintf(os.Stderr, ">>> DEBUG: Actual error type (Pointer): %s, Kind: %s, PkgPath: %s\n", errType.String(), errKind.String(), errPkgPath)
		} else {
			errPkgPath = errType.PkgPath()
			fmt.Fprintf(os.Stderr, ">>> DEBUG: Actual error type (Value): %s, Kind: %s, PkgPath: %s\n", errType.String(), errKind.String(), errPkgPath)
		}

		// Check against expected type *lfsm.CanceledError using reflection.
		expectedPtrType := reflect.TypeOf(&lfsm.CanceledError{})
		fmt.Fprintf(os.Stderr, ">>> DEBUG: Reflection type comparison (Pointer): actual=%s, expected=%s, match=%t\n", errType.String(), expectedPtrType.String(), expectedPtrType == errType)

		// Check against expected type lfsm.CanceledError using reflection.
		expectedValueType := reflect.TypeOf(lfsm.CanceledError{})
		fmt.Fprintf(os.Stderr, ">>> DEBUG: Reflection type comparison (Value): actual=%s, expected=%s, match=%t\n\n", errType.String(), expectedValueType.String(), expectedValueType == errType)

		// Check using errors.As for *lfsm.CanceledError.
		var canceledErrPtr *lfsm.CanceledError
		isCanceledPtr := errors.As(err, &canceledErrPtr)
		fmt.Fprintf(os.Stderr, ">>> DEBUG: errors.As(err, *lfsm.CanceledError): %t\n", isCanceledPtr)

		// Check using errors.As for lfsm.CanceledError (value type).
		var canceledErrVal lfsm.CanceledError
		isCanceledVal := errors.As(err, &canceledErrVal)
		fmt.Fprintf(os.Stderr, ">>> DEBUG: errors.As(err, lfsm.CanceledError): %t\n\n", isCanceledVal)

		// <<< --- END TEMPORARY PRINTF LOGGING --- >>>

		// For now, just return the error after logging to see the debug output.
		return err // Temporarily just return to see logs.

		/* // Original/Previous Handling Logic (commented out for now).
		// ... (complex error handling code was here) ...
		*/
	}

	// Log success if no error occurred.
	newState := State(fsmInstance.Current())
	l.logger.Debug("Transition successful.", "event", event, "old_state", currentState, "new_state", newState)
	return nil
}

// SetState allows manually setting the FSM state. Use with caution. Requires Build().
func (l *loopFSM) SetState(state State) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fsm == nil {
		l.logger.Error("SetState() called before Build() or after build error.")
		return l.buildErr // Return potential build error.
	}
	l.logger.Warn("Manually setting FSM state.", "target_state", state)
	l.fsm.SetState(string(state))
	return nil
}

// Reset sets the state back to the initial state. Requires Build().
func (l *loopFSM) Reset() error {
	l.logger.Info("Resetting FSM to initial state.", "initialState", l.initialState)
	return l.SetState(l.initialState)
}
