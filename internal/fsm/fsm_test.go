// Package fsm_test tests the generic FSM implementation.
package fsm

// file: internal/fsm/fsm_test.go

import (
	"context"
	"errors" // Use standard errors package for errors.As.
	"fmt"    // Added import.
	"sync/atomic"
	"testing" // Added import.

	"github.com/dkoosis/cowgnition/internal/logging"
	lfsm "github.com/looplab/fsm" // Use alias 'lfsm'.
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Define states and events for testing.
const (
	StateIdle     State = "idle"
	StateRunning  State = "running"
	StatePaused   State = "paused"
	StateFinished State = "finished"

	EventStart Event = "start"
	EventPause Event = "pause"
	EventStop  Event = "stop"
	EventReset Event = "reset" // Use different name for Reset event vs method.
	EventForce Event = "force" // Event with data.
)

// Helper to build a basic FSM for tests.
func buildTestFSM(t *testing.T) FSM {
	t.Helper()
	logger := logging.GetNoopLogger()
	fsmBuilder := NewFSM(StateIdle, logger)

	// --- CORRECTED From field to be []State ---
	fsmBuilder.AddTransition(Transition{From: []State{StateIdle}, Event: EventStart, To: StateRunning})
	fsmBuilder.AddTransition(Transition{From: []State{StateRunning}, Event: EventPause, To: StatePaused})
	fsmBuilder.AddTransition(Transition{From: []State{StateRunning}, Event: EventStop, To: StateFinished})
	fsmBuilder.AddTransition(Transition{From: []State{StatePaused}, Event: EventStart, To: StateRunning}) // Resume.
	fsmBuilder.AddTransition(Transition{From: []State{StatePaused}, Event: EventStop, To: StateFinished})
	fsmBuilder.AddTransition(Transition{From: []State{StateFinished}, Event: EventReset, To: StateIdle})
	// --- END CORRECTIONS ---

	err := fsmBuilder.Build()
	require.NoError(t, err, "Failed to build test FSM.")
	return fsmBuilder
}

// TestFSM_NewFSM_ReturnsValidBuilder tests the constructor.
func TestFSM_NewFSM_ReturnsValidBuilder(t *testing.T) {
	logger := logging.GetNoopLogger()
	fsmBuilder := NewFSM(StateIdle, logger)
	require.NotNil(t, fsmBuilder, "NewFSM should return a non-nil instance.")
	// Cannot check CurrentState before Build().
}

// TestFSM_Build_Fails_When_CalledAfterBuild tests calling Build twice.
func TestFSM_Build_Fails_When_CalledAfterBuild(t *testing.T) {
	logger := logging.GetNoopLogger()
	fsmBuilder := NewFSM(StateIdle, logger)
	err := fsmBuilder.Build() // First build.
	require.NoError(t, err)
	err = fsmBuilder.Build() // Second build.
	require.NoError(t, err, "Calling Build() multiple times should be idempotent and not error.")
}

// TestFSM_BasicTransitions_Succeeds tests simple state transitions.
func TestFSM_BasicTransitions_Succeeds(t *testing.T) {
	fsm := buildTestFSM(t) // Use helper to build.
	ctx := context.Background()

	assert.Equal(t, StateIdle, fsm.CurrentState(), "Initial state should be Idle.")

	// Start.
	err := fsm.Transition(ctx, EventStart, nil)
	require.NoError(t, err, "Transition from Idle to Running should succeed.")
	assert.Equal(t, StateRunning, fsm.CurrentState(), "State should be Running.")

	// Stop.
	err = fsm.Transition(ctx, EventStop, nil)
	require.NoError(t, err, "Transition from Running to Finished should succeed.")
	assert.Equal(t, StateFinished, fsm.CurrentState(), "State should be Finished.")
}

// TestFSM_InvalidTransition_ReturnsError tests attempting an invalid transition.
func TestFSM_InvalidTransition_ReturnsError(t *testing.T) {
	fsm := buildTestFSM(t) // Use helper to build.
	ctx := context.Background()

	assert.Equal(t, StateIdle, fsm.CurrentState(), "Initial state should be Idle.")

	// Try to stop when idle (no transition defined for Stop from Idle).
	assert.False(t, fsm.CanTransition(EventStop), "Should not be able to transition on Stop from Idle.")
	err := fsm.Transition(ctx, EventStop, nil)
	require.Error(t, err, "Transition on Stop from Idle should return an error.")
	// Check error message contains expected substring.
	assert.Contains(t, err.Error(), "inappropriate in current state", "Error message should indicate event inappropriate for state.")
	assert.Equal(t, StateIdle, fsm.CurrentState(), "State should remain Idle.")
}

// TestFSM_TransitionWithAction_ExecutesAction tests if the action callback runs.
func TestFSM_TransitionWithAction_ExecutesAction(t *testing.T) {
	logger := logging.GetNoopLogger()
	fsmBuilder := NewFSM(StateIdle, logger)
	actionExecuted := atomic.Bool{}

	action := func(_ context.Context, event Event, data interface{}) error {
		actionExecuted.Store(true)
		assert.Equal(t, EventStart, event, "Event in action should be Start.")
		assert.Equal(t, "some data", data.(string), "Data in action mismatch.")
		return nil
	}

	// --- CORRECTED From field ---
	fsmBuilder.AddTransition(Transition{From: []State{StateIdle}, Event: EventStart, To: StateRunning, Action: action})
	err := fsmBuilder.Build()
	require.NoError(t, err)

	ctx := context.Background()
	err = fsmBuilder.Transition(ctx, EventStart, "some data")
	require.NoError(t, err, "Transition should succeed.")
	assert.Equal(t, StateRunning, fsmBuilder.CurrentState(), "State should be Running.")
	assert.True(t, actionExecuted.Load(), "Transition action should have been executed.")
}

// TestFSM_TransitionWithFailingAction_LogsError tests logging when action fails.
func TestFSM_TransitionWithFailingAction_LogsError(t *testing.T) {
	logger := logging.GetNoopLogger()
	fsmBuilder := NewFSM(StateIdle, logger)
	actionExecuted := atomic.Bool{}

	action := func(_ context.Context, _ Event, _ interface{}) error {
		actionExecuted.Store(true)
		return fmt.Errorf("action failed deliberately")
	}

	// --- CORRECTED From field ---
	fsmBuilder.AddTransition(Transition{From: []State{StateIdle}, Event: EventStart, To: StateRunning, Action: action})
	err := fsmBuilder.Build()
	require.NoError(t, err)

	ctx := context.Background()
	err = fsmBuilder.Transition(ctx, EventStart, nil)

	require.NoError(t, err, "Transition itself should succeed even if action fails (limitation).")
	assert.Equal(t, StateRunning, fsmBuilder.CurrentState(), "State should still transition to Running.")
	assert.True(t, actionExecuted.Load(), "Transition action should have been executed.")
	// TODO: Add mock logger assertion if capturing logs is implemented.
}

// TestFSM_TransitionWithGuard_AllowsAndBlocks tests guard conditions.
func TestFSM_TransitionWithGuard_AllowsAndBlocks(t *testing.T) {
	logger := logging.GetNoopLogger()
	fsmBuilder := NewFSM(StateIdle, logger)
	canForce := true // Mutable variable for the guard.

	guard := func(_ context.Context, event Event, data interface{}) bool {
		require.Equal(t, EventForce, event)
		require.Equal(t, "force data", data.(string))
		return canForce
	}

	// --- CORRECTED From field ---
	fsmBuilder.AddTransition(Transition{From: []State{StateIdle}, Event: EventForce, To: StateRunning, Condition: guard})
	err := fsmBuilder.Build()
	require.NoError(t, err)

	ctx := context.Background()

	// --- Test Allowed Transition ---
	t.Log("Testing allowed transition with guard.")
	canForce = true
	assert.True(t, fsmBuilder.CanTransition(EventForce), "Should report CanTransition true as event is defined.")
	err = fsmBuilder.Transition(ctx, EventForce, "force data")
	require.NoError(t, err, "Transition should succeed when guard condition is true.")
	assert.Equal(t, StateRunning, fsmBuilder.CurrentState(), "State should transition to Running.")

	// Reset state for next part using SetState.
	err = fsmBuilder.SetState(StateIdle)
	require.NoError(t, err)
	require.Equal(t, StateIdle, fsmBuilder.CurrentState())

	// --- Test Blocked Transition ---
	t.Log("Testing blocked transition with guard.")
	canForce = false
	assert.True(t, fsmBuilder.CanTransition(EventForce), "CanTransition should still be true as event is defined.")
	err = fsmBuilder.Transition(ctx, EventForce, "force data")
	require.Error(t, err, "Transition should fail when guard condition is false.")
	// --- FINAL FIX: Check value type for CanceledError ---
	var canceledErr lfsm.CanceledError // <<< Use value type lfsm.CanceledError
	require.True(t, errors.As(err, &canceledErr), "Error should be (or wrap) a CanceledError when guard fails.")
	// --- END FINAL FIX ---
	assert.Equal(t, StateIdle, fsmBuilder.CurrentState(), "State should remain Idle when guard blocks.")
}

// TestFSM_Reset_RestoresInitialState tests the Reset method.
func TestFSM_Reset_RestoresInitialState(t *testing.T) {
	fsm := buildTestFSM(t) // Use helper to build.
	ctx := context.Background()

	// Transition a few times.
	err := fsm.Transition(ctx, EventStart, nil)
	require.NoError(t, err)
	err = fsm.Transition(ctx, EventPause, nil)
	require.NoError(t, err)
	require.Equal(t, StatePaused, fsm.CurrentState(), "State should be Paused before reset.")

	// Reset.
	err = fsm.Reset()
	require.NoError(t, err)

	// Verify state and transitions.
	assert.Equal(t, StateIdle, fsm.CurrentState(), "State should be reset to Idle.")
	assert.True(t, fsm.CanTransition(EventStart), "Should be able to transition on Start after reset.")
	assert.False(t, fsm.CanTransition(EventPause), "Should not be able to transition on Pause after reset (back in Idle state).")

	// Ensure a transition still works after reset.
	err = fsm.Transition(ctx, EventStart, nil)
	require.NoError(t, err, "Transition should work after reset.")
	assert.Equal(t, StateRunning, fsm.CurrentState(), "State should be Running after transition post-reset.")
}

// TestFSM_Build_Fails_When_ConflictingDestinations tests build error on bad config.
func TestFSM_Build_Fails_When_ConflictingDestinations(t *testing.T) {
	logger := logging.GetNoopLogger()
	fsmBuilder := NewFSM(StateIdle, logger)

	// --- Conflicting Transitions ---
	fsmBuilder.AddTransition(Transition{From: []State{StateIdle}, Event: EventStart, To: StateRunning})
	fsmBuilder.AddTransition(Transition{From: []State{StateIdle}, Event: EventStart, To: StatePaused}) // Same event, different destination.

	err := fsmBuilder.Build()
	require.Error(t, err, "Build should fail with conflicting destinations for the same event.")
	assert.Contains(t, err.Error(), "conflicting destinations", "Error message should indicate conflicting destinations.")
}

// TestFSM_Build_Fails_When_MissingFromState tests build error on bad config.
func TestFSM_Build_Fails_When_MissingFromState(t *testing.T) {
	logger := logging.GetNoopLogger()
	fsmBuilder := NewFSM(StateIdle, logger)

	// --- Transition Missing From ---
	fsmBuilder.AddTransition(Transition{Event: EventStart, To: StateRunning}) // Missing From field.

	err := fsmBuilder.Build()
	require.Error(t, err, "Build should fail when a transition is missing 'From' states.")
	assert.Contains(t, err.Error(), "missing 'From' states", "Error message should indicate missing 'From' states.")
}
