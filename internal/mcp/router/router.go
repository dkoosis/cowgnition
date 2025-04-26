// Package router provides a routing mechanism for dispatching MCP method calls.
// file: internal/mcp/router/router.go
package router

import (
	"context"
	"encoding/json"
	"fmt"
	"sync" // Added for map protection.

	"github.com/dkoosis/cowgnition/internal/logging"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors" // For MethodNotFound error.
)

// Handler defines the function signature for handling MCP requests that expect a response.
// It receives the context and raw parameters, returning raw result bytes or an error.
type Handler func(ctx context.Context, params json.RawMessage) (json.RawMessage, error)

// NotificationHandler defines the function signature for handling MCP notifications (no response expected).
// It receives the context and raw parameters, returning only an error if processing fails.
type NotificationHandler func(ctx context.Context, params json.RawMessage) error

// Route defines the mapping between an MCP method name and its handler function(s).
type Route struct {
	Method              string              // The MCP method name (e.g., "initialize", "tools/list").
	Handler             Handler             // Handler for requests expecting a response.
	NotificationHandler NotificationHandler // Handler for notifications.
	// IsNotification field removed, determined dynamically by the caller of Route method.
}

// Router defines the interface for an MCP method router.
type Router interface {
	// AddRoute registers a handler for a specific MCP method.
	AddRoute(route Route) error
	// Route dispatches an incoming message to the appropriate registered handler.
	Route(ctx context.Context, method string, params json.RawMessage, isNotification bool) (json.RawMessage, error)
	// GetRoutes returns a list of registered method names (for debugging/introspection).
	GetRoutes() []string
}

// router implements the Router interface.
type router struct {
	routes map[string]Route // Map method name to its Route definition.
	mu     sync.RWMutex     // Protects the routes map.
	logger logging.Logger
}

// NewRouter creates a new Router instance.
func NewRouter(logger logging.Logger) Router {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	return &router{
		routes: make(map[string]Route),
		logger: logger.WithField("component", "mcp_router"),
	}
}

// AddRoute registers a new route. Returns error if the method is already registered.
func (r *router) AddRoute(route Route) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if route.Method == "" {
		return fmt.Errorf("cannot register route with empty method name")
	}
	if route.Handler == nil && route.NotificationHandler == nil {
		return fmt.Errorf("route for method '%s' must have at least one handler (Handler or NotificationHandler)", route.Method)
	}

	if _, exists := r.routes[route.Method]; exists {
		r.logger.Warn("Attempted to register duplicate route.", "method", route.Method)
		return fmt.Errorf("route for method '%s' already registered", route.Method)
	}

	r.routes[route.Method] = route
	r.logger.Debug("Registered route.", "method", route.Method)
	return nil
}

// Route looks up the handler for the given method and executes it.
// It distinguishes between requests (isNotification=false) and notifications (isNotification=true).
func (r *router) Route(ctx context.Context, method string, params json.RawMessage, isNotification bool) (json.RawMessage, error) {
	r.mu.RLock()
	route, exists := r.routes[method]
	r.mu.RUnlock()

	if !exists {
		r.logger.Warn("Method not found in router.", "method", method)
		// Return a specific MCP error for method not found.
		return nil, mcperrors.NewMethodNotFoundError(
			fmt.Sprintf("Method '%s' not found", method),
			nil,
			map[string]interface{}{"method": method},
		)
	}

	// Execute the appropriate handler based on whether it's a notification.
	if isNotification {
		if route.NotificationHandler != nil {
			r.logger.Debug("Routing to notification handler.", "method", method)
			err := route.NotificationHandler(ctx, params)
			// For notifications, we return nil bytes and only propagate errors from the handler.
			return nil, err
		}
		// If it's a notification but only a request Handler is registered, log a warning?.
		// MCP spec allows sending notifications to methods that might normally expect responses,.
		// the server just doesn't send a response back. Let's log this case.
		if route.Handler != nil {
			r.logger.Warn("Received notification for method with only a request handler registered, executing handler but discarding result.", "method", method)
			// Execute the request handler but ignore the result bytes.
			_, err := route.Handler(ctx, params)
			return nil, err // Propagate error, but no result bytes.
		}
		// If neither handler exists (should have been caught by AddRoute, but check defensively).
		r.logger.Error("Internal Router Error: No suitable handler found for notification.", "method", method)
		return nil, mcperrors.NewInternalError( // Should not happen if AddRoute validation works.
			fmt.Sprintf("internal router error: no handler configured for notification method '%s'", method),
			nil,
			map[string]interface{}{"method": method},
		)

	}
	// It's a request (expects a response).
	if route.Handler != nil {
		r.logger.Debug("Routing to request handler.", "method", method)
		resultBytes, err := route.Handler(ctx, params)
		// Return both result bytes and error from the handler.
		return resultBytes, err
	}
	// If it's a request but only a NotificationHandler is registered.
	r.logger.Error("Invalid configuration: Received request for method with only a notification handler.", "method", method)
	return nil, mcperrors.NewMethodNotFoundError( // Treat as MethodNotFound as it can't produce a response.
		fmt.Sprintf("Method '%s' is notification-only and cannot produce a response", method),
		nil,
		map[string]interface{}{"method": method},
	)

}

// GetRoutes returns a slice of registered method names.
func (r *router) GetRoutes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	methods := make([]string, 0, len(r.routes))
	for method := range r.routes {
		methods = append(methods, method)
	}
	// Potentially sort the methods if consistent ordering is desired.
	// sort.Strings(methods).
	return methods
}
