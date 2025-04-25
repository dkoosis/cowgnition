// Package router_test tests the MCP method router implementation.
// file: internal/mcp/router/router_test.go
package router

import (
	"context"
	"encoding/json"
	"errors" // Standard errors package.
	"fmt"
	"sort" // For predictable GetRoutes output.
	"sync/atomic"
	"testing"

	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // Import MCP errors.
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks & Helpers ---

var errMockHandler = errors.New("mock handler error")

// mockRequestHandler simulates a handler for requests expecting a response.
func mockRequestHandler(method string, shouldError bool) Handler {
	return func(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
		if shouldError {
			return nil, errMockHandler
		}
		// Simple echo response for testing.
		resp := map[string]interface{}{
			"receivedMethod": method,
			"receivedParams": string(params),
		}
		resBytes, _ := json.Marshal(resp)
		return resBytes, nil
	}
}

// mockNotificationHandler simulates a handler for notifications.
func mockNotificationHandler(method string, shouldError bool, executionCounter *atomic.Int32) NotificationHandler {
	return func(_ context.Context, params json.RawMessage) error {
		if executionCounter != nil {
			executionCounter.Add(1)
		}
		if shouldError {
			return errMockHandler
		}
		fmt.Printf("Mock notification handler executed for %s with params: %s\n", method, string(params)) // Simulate action.
		return nil
	}
}

// Helper to assert specific MCP error codes.
func assertMCPErrorCode(t *testing.T, expectedCode mcperrors.ErrorCode, err error) {
	t.Helper()
	require.Error(t, err, "Expected an error but got nil.")
	var mcpErr *mcperrors.BaseError
	isMCPError := errors.As(err, &mcpErr)
	require.True(t, isMCPError, "Error should be an MCP error (BaseError or specific type). Got: %T", err)
	assert.Equal(t, expectedCode, mcpErr.Code, "MCP error code mismatch.")
}

// --- Test Cases ---

// TestRouter_NewRouter_Succeeds tests the router constructor.
func TestRouter_NewRouter_Succeeds(t *testing.T) {
	logger := logging.GetNoopLogger()
	r := NewRouter(logger)
	require.NotNil(t, r, "NewRouter should return a non-nil instance.")
	// Check if internal fields are initialized (requires type assertion).
	concreteRouter, ok := r.(*router)
	require.True(t, ok, "NewRouter should return a concrete *router instance.")
	assert.NotNil(t, concreteRouter.routes, "Internal routes map should be initialized.")
	assert.NotNil(t, concreteRouter.logger, "Internal logger should be initialized.")
}

// TestRouter_AddRoute_Succeeds tests adding valid request and notification routes.
func TestRouter_AddRoute_Succeeds(t *testing.T) {
	r := NewRouter(logging.GetNoopLogger())

	// Add request route.
	err := r.AddRoute(Route{
		Method:  "test/request",
		Handler: mockRequestHandler("test/request", false),
	})
	assert.NoError(t, err, "Should succeed adding a request route.")

	// Add notification route.
	err = r.AddRoute(Route{
		Method:              "test/notification",
		NotificationHandler: mockNotificationHandler("test/notification", false, nil),
	})
	assert.NoError(t, err, "Should succeed adding a notification route.")

	// Add route with both handlers.
	err = r.AddRoute(Route{
		Method:              "test/both",
		Handler:             mockRequestHandler("test/both", false),
		NotificationHandler: mockNotificationHandler("test/both", false, nil),
	})
	assert.NoError(t, err, "Should succeed adding a route with both handlers.")

	// Verify routes were added (internal check).
	concreteRouter := r.(*router)
	assert.Len(t, concreteRouter.routes, 3, "Should have 3 routes registered.")
	_, reqExists := concreteRouter.routes["test/request"]
	_, notifExists := concreteRouter.routes["test/notification"]
	_, bothExists := concreteRouter.routes["test/both"]
	assert.True(t, reqExists, "test/request route should exist.")
	assert.True(t, notifExists, "test/notification route should exist.")
	assert.True(t, bothExists, "test/both route should exist.")
}

// TestRouter_AddRoute_Fails_When_DuplicateMethod tests adding the same method twice.
func TestRouter_AddRoute_Fails_When_DuplicateMethod(t *testing.T) {
	r := NewRouter(logging.GetNoopLogger())
	err := r.AddRoute(Route{Method: "duplicate", Handler: mockRequestHandler("duplicate", false)})
	require.NoError(t, err)
	err = r.AddRoute(Route{Method: "duplicate", NotificationHandler: mockNotificationHandler("duplicate", false, nil)})
	require.Error(t, err, "Should fail adding a duplicate method name.")
	assert.Contains(t, err.Error(), "already registered", "Error message should indicate duplicate registration.")
}

// TestRouter_AddRoute_Fails_When_NoHandler tests adding a route without any handler.
func TestRouter_AddRoute_Fails_When_NoHandler(t *testing.T) {
	r := NewRouter(logging.GetNoopLogger())
	err := r.AddRoute(Route{Method: "nohandler"})
	require.Error(t, err, "Should fail adding a route with no handlers.")
	assert.Contains(t, err.Error(), "must have at least one handler", "Error message should indicate missing handler.")
}

// TestRouter_AddRoute_Fails_When_EmptyMethod tests adding a route with an empty method name.
func TestRouter_AddRoute_Fails_When_EmptyMethod(t *testing.T) {
	r := NewRouter(logging.GetNoopLogger())
	err := r.AddRoute(Route{Method: "", Handler: mockRequestHandler("", false)})
	require.Error(t, err, "Should fail adding a route with empty method name.")
	assert.Contains(t, err.Error(), "empty method name", "Error message should indicate empty method name.")
}

// TestRouter_Route_Succeeds_When_RequestMethodExists tests routing a request.
func TestRouter_Route_Succeeds_When_RequestMethodExists(t *testing.T) {
	r := NewRouter(logging.GetNoopLogger())
	method := "test/doSomething"
	params := json.RawMessage(`{"arg": 1}`)
	expectedResult := `{"receivedMethod":"test/doSomething","receivedParams":"{\"arg\": 1}"}`
	err := r.AddRoute(Route{Method: method, Handler: mockRequestHandler(method, false)})
	require.NoError(t, err)

	resBytes, routeErr := r.Route(context.Background(), method, params, false) // isNotification = false.

	require.NoError(t, routeErr, "Routing a valid request should not return an error.")
	assert.JSONEq(t, expectedResult, string(resBytes), "Response bytes should match expected.")
}

// TestRouter_Route_Succeeds_When_NotificationMethodExists tests routing a notification.
func TestRouter_Route_Succeeds_When_NotificationMethodExists(t *testing.T) {
	r := NewRouter(logging.GetNoopLogger())
	method := "test/notify"
	params := json.RawMessage(`{"info": "data"}`)
	var counter atomic.Int32
	err := r.AddRoute(Route{Method: method, NotificationHandler: mockNotificationHandler(method, false, &counter)})
	require.NoError(t, err)

	resBytes, routeErr := r.Route(context.Background(), method, params, true) // isNotification = true.

	require.NoError(t, routeErr, "Routing a valid notification should not return an error.")
	assert.Nil(t, resBytes, "Response bytes should be nil for notifications.")
	assert.Equal(t, int32(1), counter.Load(), "Notification handler should have been executed once.")
}

// TestRouter_Route_Succeeds_When_NotificationSentToRequestHandler tests sending a notification to a request-only handler.
func TestRouter_Route_Succeeds_When_NotificationSentToRequestHandler(t *testing.T) {
	r := NewRouter(logging.GetNoopLogger())
	method := "test/requestOnly"
	params := json.RawMessage(`{"data": "for_request"}`)
	err := r.AddRoute(Route{Method: method, Handler: mockRequestHandler(method, false)}) // Only request handler.
	require.NoError(t, err)

	resBytes, routeErr := r.Route(context.Background(), method, params, true) // isNotification = true.

	// Expect no error, handler runs, but no response bytes are returned.
	require.NoError(t, routeErr, "Routing notification to request-only handler should succeed.")
	assert.Nil(t, resBytes, "Response bytes should be nil when notification sent to request handler.")
}

// TestRouter_Route_Fails_When_RequestMethodNotFound tests routing an unknown method.
func TestRouter_Route_Fails_When_RequestMethodNotFound(t *testing.T) {
	r := NewRouter(logging.GetNoopLogger())
	params := json.RawMessage(`{}`)

	resBytes, routeErr := r.Route(context.Background(), "unknown/method", params, false) // isNotification = false.

	require.Error(t, routeErr, "Routing an unknown method should return an error.")
	assert.Nil(t, resBytes, "Response bytes should be nil on routing error.")
	assertMCPErrorCode(t, mcperrors.ErrMethodNotFound, routeErr)
}

// TestRouter_Route_Fails_When_RequestSentToNotificationHandler tests sending a request to a notification-only handler.
func TestRouter_Route_Fails_When_RequestSentToNotificationHandler(t *testing.T) {
	r := NewRouter(logging.GetNoopLogger())
	method := "test/notificationOnly"
	params := json.RawMessage(`{"data": "some_data"}`)
	err := r.AddRoute(Route{Method: method, NotificationHandler: mockNotificationHandler(method, false, nil)}) // Only notification handler.
	require.NoError(t, err)

	resBytes, routeErr := r.Route(context.Background(), method, params, false) // isNotification = false.

	require.Error(t, routeErr, "Routing request to notification-only handler should return an error.")
	assert.Nil(t, resBytes, "Response bytes should be nil.")
	assertMCPErrorCode(t, mcperrors.ErrMethodNotFound, routeErr) // Expect MethodNotFound as it cannot produce a response.
}

// TestRouter_Route_Propagates_HandlerError tests if errors from handlers are returned.
func TestRouter_Route_Propagates_HandlerError(t *testing.T) {
	r := NewRouter(logging.GetNoopLogger())
	methodReq := "test/requestError"
	methodNotif := "test/notificationError"
	params := json.RawMessage(`{}`)

	// Add routes with handlers that return errors.
	err := r.AddRoute(Route{Method: methodReq, Handler: mockRequestHandler(methodReq, true)})
	require.NoError(t, err)
	err = r.AddRoute(Route{Method: methodNotif, NotificationHandler: mockNotificationHandler(methodNotif, true, nil)})
	require.NoError(t, err)

	// Test request handler error.
	resBytesReq, routeErrReq := r.Route(context.Background(), methodReq, params, false)
	require.Error(t, routeErrReq, "Error from request handler should be propagated.")
	assert.Nil(t, resBytesReq, "Response bytes should be nil when handler errors.")
	assert.ErrorIs(t, routeErrReq, errMockHandler, "Propagated error should match the mock handler error.")

	// Test notification handler error.
	resBytesNotif, routeErrNotif := r.Route(context.Background(), methodNotif, params, true)
	require.Error(t, routeErrNotif, "Error from notification handler should be propagated.")
	assert.Nil(t, resBytesNotif, "Response bytes should be nil for notification handler error.")
	assert.ErrorIs(t, routeErrNotif, errMockHandler, "Propagated error should match the mock handler error.")
}

// TestRouter_GetRoutes_ReturnsRegisteredMethods tests listing registered routes.
func TestRouter_GetRoutes_ReturnsRegisteredMethods(t *testing.T) {
	r := NewRouter(logging.GetNoopLogger())
	methods := []string{"route/a", "route/b", "route/c"}

	for _, m := range methods {
		err := r.AddRoute(Route{Method: m, Handler: mockRequestHandler(m, false)})
		require.NoError(t, err)
	}

	registered := r.GetRoutes()
	sort.Strings(registered) // Sort for deterministic comparison.
	sort.Strings(methods)

	assert.Equal(t, methods, registered, "GetRoutes should return all registered method names.")

	// Test with no routes.
	rEmpty := NewRouter(logging.GetNoopLogger())
	assert.Empty(t, rEmpty.GetRoutes(), "GetRoutes should return empty slice when no routes are registered.")
}
