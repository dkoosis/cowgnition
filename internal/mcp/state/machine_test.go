// file: internal/mcp/state/machine_test.go
package state

import (
	"context"
	"os" // <<< Added import for os.Getenv
	"testing"

	"errors" // Use standard errors package for errors.As

	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // Import MCP errors.
	lfsm "github.com/looplab/fsm"                                     // Import looplab/fsm for error type checking
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create a new, configured MCP State Machine for testing.
func setupTestMCPStateMachine(t *testing.T) *MCPStateMachine {
	t.Helper()

	// --- MODIFICATION START ---
	// Initialize the actual logger based on environment (or default to info)
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info" // Default for tests if not set
	}
	logging.SetupDefaultLogger(logLevel)          // Setup the application's default logger
	logger := logging.GetLogger("mcp_state_test") // Get a logger instance
	// --- MODIFICATION END ---

	m, err := NewMCPStateMachine(logger) // Pass the real logger
	require.NoError(t, err, "Failed to create new MCP state machine for test.")
	require.NotNil(t, m, "NewMCPStateMachine should return a non-nil instance.")
	return m
}

// TestMCPStateMachine_NewMCPStateMachine_Succeeds checks basic creation and initial state.
func TestMCPStateMachine_NewMCPStateMachine_Succeeds(t *testing.T) {
	m := setupTestMCPStateMachine(t)
	assert.Equal(t, StateUninitialized, m.CurrentState(), "Initial state should be Uninitialized.")
}

// TestMCPStateMachine_ValidTransitions_Succeeds tests the primary successful lifecycle path.
func TestMCPStateMachine_ValidTransitions_Succeeds(t *testing.T) {
	m := setupTestMCPStateMachine(t)
	ctx := context.Background()

	// Uninitialized -> Initializing (on initialize request)
	err := m.Transition(ctx, EventInitializeRequest, nil)
	require.NoError(t, err, "Transition on EventInitializeRequest should succeed.")
	assert.Equal(t, StateInitializing, m.CurrentState(), "State should be Initializing.")

	// Initializing -> Initialized (on client initialized notification)
	err = m.Transition(ctx, EventClientInitialized, nil)
	require.NoError(t, err, "Transition on EventClientInitialized should succeed.")
	assert.Equal(t, StateInitialized, m.CurrentState(), "State should be Initialized.")

	// Initialized -> Initialized (on standard request/notification - EXPECT NoTransitionError)
	err = m.Transition(ctx, EventMCPRequest, nil)
	// --- MODIFICATION START: Adjust type check ---
	var noTransitionErr lfsm.NoTransitionError // <<< Change: Use value type, not pointer
	require.Error(t, err, "Self-transition on EventMCPRequest should return an error.")
	require.True(t, errors.As(err, &noTransitionErr), "Error for self-transition should be NoTransitionError.") // <<< errors.As now correctly checks for value type
	// --- MODIFICATION END ---
	assert.Equal(t, StateInitialized, m.CurrentState(), "State should remain Initialized after EventMCPRequest.")

	err = m.Transition(ctx, EventMCPNotification, nil)
	// --- MODIFICATION START: Adjust type check ---
	// var noTransitionErr lfsm.NoTransitionError // <<< Re-use or redeclare if needed, variable scope applies
	require.Error(t, err, "Self-transition on EventMCPNotification should return an error.")
	require.True(t, errors.As(err, &noTransitionErr), "Error for self-transition should be NoTransitionError.") // <<< errors.As now correctly checks for value type
	// --- MODIFICATION END ---
	assert.Equal(t, StateInitialized, m.CurrentState(), "State should remain Initialized after EventMCPNotification.")

	// Initialized -> ShuttingDown (on shutdown request) - This should still be NoError
	err = m.Transition(ctx, EventShutdownRequest, nil)
	require.NoError(t, err, "Transition on EventShutdownRequest should succeed.") // Actual state change
	assert.Equal(t, StateShuttingDown, m.CurrentState(), "State should be ShuttingDown.")

	// ShuttingDown -> Shutdown (on exit notification)
	err = m.Transition(ctx, EventExitNotification, nil)
	require.NoError(t, err, "Transition on EventExitNotification should succeed.")
	assert.Equal(t, StateShutdown, m.CurrentState(), "State should be Shutdown.")
}

// TestMCPStateMachine_ValidateMethod_AllowsCorrectSequence tests allowed method calls per state.
func TestMCPStateMachine_ValidateMethod_AllowsCorrectSequence(t *testing.T) {
	m := setupTestMCPStateMachine(t)
	ctx := context.Background() // Needed for state transitions

	// State: Uninitialized
	assert.NoError(t, m.ValidateMethod("initialize"), "Initialize should be allowed in Uninitialized state.")

	// Transition: Uninitialized -> Initializing
	_ = m.Transition(ctx, EventInitializeRequest, nil)
	require.Equal(t, StateInitializing, m.CurrentState())

	// State: Initializing
	assert.NoError(t, m.ValidateMethod("notifications/initialized"), "notifications/initialized should be allowed in Initializing state.")

	// Transition: Initializing -> Initialized
	_ = m.Transition(ctx, EventClientInitialized, nil)
	require.Equal(t, StateInitialized, m.CurrentState())

	// State: Initialized
	assert.NoError(t, m.ValidateMethod("tools/list"), "tools/list should be allowed in Initialized state.")
	assert.NoError(t, m.ValidateMethod("resources/read"), "resources/read should be allowed in Initialized state.")
	assert.NoError(t, m.ValidateMethod("$/cancelRequest"), "$/cancelRequest should be allowed in Initialized state.")
	assert.NoError(t, m.ValidateMethod("shutdown"), "shutdown should be allowed in Initialized state.")
	assert.NoError(t, m.ValidateMethod("exit"), "exit should be allowed in Initialized state.")
	assert.NoError(t, m.ValidateMethod("unknownMethod"), "Unknown methods should be allowed in Initialized state (router handles validity).")

	// Transition: Initialized -> ShuttingDown
	// Handle potential NoTransitionError for the self-transitions first
	_ = m.Transition(ctx, EventMCPRequest, nil)      // Ignore error
	_ = m.Transition(ctx, EventMCPNotification, nil) // Ignore error
	_ = m.Transition(ctx, EventShutdownRequest, nil) // Actual transition
	require.Equal(t, StateShuttingDown, m.CurrentState())

	// State: ShuttingDown
	assert.NoError(t, m.ValidateMethod("exit"), "exit should be allowed in ShuttingDown state.")

	// Transition: ShuttingDown -> Shutdown
	_ = m.Transition(ctx, EventExitNotification, nil)
	require.Equal(t, StateShutdown, m.CurrentState())
	// State: Shutdown (Terminal) - Most methods should be disallowed implicitly by ValidateMethod.
}

// TestMCPStateMachine_ValidateMethod_RejectsIncorrectSequence tests disallowed method calls.
func TestMCPStateMachine_ValidateMethod_RejectsIncorrectSequence(t *testing.T) {
	m := setupTestMCPStateMachine(t)
	ctx := context.Background() // Needed for state transitions

	// State: Uninitialized
	err := m.ValidateMethod("tools/list")
	require.Error(t, err, "tools/list should be rejected in Uninitialized state.")
	assertErrorCode(t, mcperrors.ErrRequestSequence, err)

	err = m.ValidateMethod("shutdown")
	require.Error(t, err, "shutdown should be rejected in Uninitialized state.")
	assertErrorCode(t, mcperrors.ErrRequestSequence, err)

	err = m.ValidateMethod("exit")
	require.Error(t, err, "exit should be rejected in Uninitialized state.")
	assertErrorCode(t, mcperrors.ErrRequestSequence, err)

	err = m.ValidateMethod("notifications/initialized")
	require.Error(t, err, "notifications/initialized should be rejected in Uninitialized state.")
	assertErrorCode(t, mcperrors.ErrRequestSequence, err)

	// Transition: Uninitialized -> Initializing
	_ = m.Transition(ctx, EventInitializeRequest, nil)
	require.Equal(t, StateInitializing, m.CurrentState())

	// State: Initializing
	err = m.ValidateMethod("initialize")
	require.Error(t, err, "initialize should be rejected in Initializing state.")
	assertErrorCode(t, mcperrors.ErrRequestSequence, err)

	err = m.ValidateMethod("tools/list")
	require.Error(t, err, "tools/list should be rejected in Initializing state.")
	assertErrorCode(t, mcperrors.ErrRequestSequence, err)

	// Transition: Initializing -> Initialized
	_ = m.Transition(ctx, EventClientInitialized, nil)
	require.Equal(t, StateInitialized, m.CurrentState())

	// State: Initialized
	err = m.ValidateMethod("initialize")
	require.Error(t, err, "initialize should be rejected in Initialized state.")
	assertErrorCode(t, mcperrors.ErrRequestSequence, err)

	// Transition: Initialized -> ShuttingDown
	// Handle potential NoTransitionError for the self-transitions first
	_ = m.Transition(ctx, EventMCPRequest, nil)      // Ignore error
	_ = m.Transition(ctx, EventMCPNotification, nil) // Ignore error
	_ = m.Transition(ctx, EventShutdownRequest, nil) // Actual transition
	require.Equal(t, StateShuttingDown, m.CurrentState())

	// State: ShuttingDown
	err = m.ValidateMethod("tools/list")
	require.Error(t, err, "tools/list should be rejected in ShuttingDown state.")
	assertErrorCode(t, mcperrors.ErrRequestSequence, err)

	err = m.ValidateMethod("shutdown")
	require.Error(t, err, "shutdown should be rejected in ShuttingDown state.")
	assertErrorCode(t, mcperrors.ErrRequestSequence, err)

	// Transition: ShuttingDown -> Shutdown
	_ = m.Transition(ctx, EventExitNotification, nil)
	require.Equal(t, StateShutdown, m.CurrentState())

	// State: Shutdown (Terminal)
	err = m.ValidateMethod("tools/list")
	require.Error(t, err, "tools/list should be rejected in Shutdown state.")
	assertErrorCode(t, mcperrors.ErrRequestSequence, err)

	err = m.ValidateMethod("initialize")
	require.Error(t, err, "initialize should be rejected in Shutdown state.")
	assertErrorCode(t, mcperrors.ErrRequestSequence, err)
}

// TestMCPStateMachine_Reset_ReturnsToUninitialized tests the Reset method.
func TestMCPStateMachine_Reset_ReturnsToUninitialized(t *testing.T) {
	m := setupTestMCPStateMachine(t)
	ctx := context.Background()

	// Get into Initialized state.
	_ = m.Transition(ctx, EventInitializeRequest, nil)
	_ = m.Transition(ctx, EventClientInitialized, nil) // Go directly from Initializing -> Initialized
	require.Equal(t, StateInitialized, m.CurrentState())

	// Reset.
	err := m.Reset() // Use embedded FSM Reset.
	require.NoError(t, err)

	assert.Equal(t, StateUninitialized, m.CurrentState(), "State should be reset to Uninitialized.")
	assert.NoError(t, m.ValidateMethod("initialize"), "Initialize should be allowed after reset.")
	require.Error(t, m.ValidateMethod("tools/list"), "tools/list should be rejected after reset.")
}

// assertErrorCode checks if the error can be asserted as the target type *mcperrors.BaseError and has the expected code.
// nolint: unparam // Keep nolint directive to suppress warning for this specific test file's usage
func assertErrorCode(t *testing.T, expectedCode mcperrors.ErrorCode, err error) {
	t.Helper()
	require.Error(t, err, "Expected an error but got nil.")

	// Check for the specific expected error types based on the test case.
	var protocolErr *mcperrors.ProtocolError
	if errors.As(err, &protocolErr) {
		assert.Equal(t, expectedCode, protocolErr.Code, "MCP error code mismatch for ProtocolError.")
		return // Found the expected type and checked the code.
	}

	// Fallback if no specific type matched (though ideally tests should expect specific types).
	var baseErr *mcperrors.BaseError
	isMCPError := errors.As(err, &baseErr)
	require.True(t, isMCPError, "Error should be an MCP error. Got: %T", err)
	assert.Equal(t, expectedCode, baseErr.Code, "MCP error code mismatch (checked base type as fallback).")
}
