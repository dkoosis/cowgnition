// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// This file contains handlers for core MCP methods like initialize, shutdown, ping.
package mcp

// file: internal/mcp/handlers_core.go

import (
	"context"
	"encoding/json"
	"fmt" // Required for logging format inside handleNotificationsInitialized
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/config"             // Keep config import.
	"github.com/dkoosis/cowgnition/internal/logging"            // Keep logging import.
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Import mcp_types for shared types.
	"github.com/dkoosis/cowgnition/internal/schema"             // Need validator interface for Handler struct field.
	// Need connection state for Handler struct field.
	// Need time for Handler struct field.
)

// Handler manages the state and logic for core MCP methods.
// This struct might need adjustment based on actual dependencies.
// It now includes the ConnectionState.
type Handler struct {
	config          *config.Config
	validator       schema.ValidatorInterface
	startTime       time.Time
	connectionState *ConnectionState // Added connection state
	logger          logging.Logger
}

// NewHandler creates a new core MCP handler.
// Now requires ConnectionState.
func NewHandler(cfg *config.Config, validator schema.ValidatorInterface, startTime time.Time, state *ConnectionState, logger logging.Logger) *Handler {
	if logger == nil {
		logger = logging.GetNoopLogger()
	}
	// Add nil check for state? Or assume valid state passed in.
	if state == nil {
		logger.Error("NewHandler called with nil connectionState!")
		// Handle appropriately - maybe panic or return error if possible?
	}
	return &Handler{
		config:          cfg,
		validator:       validator,
		startTime:       startTime,
		connectionState: state,
		logger:          logger.WithField("component", "mcp_core_handler"),
	}
}

// handlePing handles the "ping" request.
// Returns an empty object as result, signifying success.
//
//nolint:unparam
func (h *Handler) handlePing(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	h.logger.Debug("Handling ping request.")
	return json.RawMessage(`{}`), nil
}

// handleInitialize handles the "initialize" request.
// Performs capability negotiation and **now also updates the connection state**.
func (h *Handler) handleInitialize(_ context.Context, params json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Handling initialize request.")

	// Use mcptypes.InitializeRequest.
	var req mcptypes.InitializeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		// Don't set state if request is invalid
		return nil, errors.Wrap(err, "failed to unmarshal initialize request parameters")
	}

	// Validate state before processing
	if h.connectionState == nil {
		h.logger.Error("Internal error: connectionState is nil in handleInitialize.")
		return nil, errors.New("internal server error: connection state missing")
	}
	// Ensure we are in the Uninitialized state
	if err := h.connectionState.ValidateMethodSequence("initialize"); err != nil {
		h.logger.Warn("Initialize called in invalid state.", "state", h.connectionState.CurrentState())
		// Return error that will be converted to JSON-RPC error
		return nil, err
	}

	// --- Processing ---
	h.connectionState.SetClientInfo(req.ClientInfo)
	h.connectionState.SetClientCapabilities(req.Capabilities)

	h.logger.Info("Client capabilities received.",
		"clientInfo", req.ClientInfo,
		"capabilities", req.Capabilities)

	if req.ProtocolVersion != "" {
		h.logger.Info("Client requested protocol version.", "version", req.ProtocolVersion)
	} else {
		h.logger.Warn("Client did not specify a protocol version in initialize request.")
	}

	// Interim Fix: Force protocol version
	serverProtocolVersion := "2024-11-05" // TODO: Make this dynamic or config driven
	h.logger.Warn("Forcing server protocol version.",
		"serverVersion", serverProtocolVersion,
		"reason", "Compatibility fix for clients like Claude Desktop lacking standard version field")

	// Define server capabilities (TODO: Make dynamic based on registered services)
	caps := mcptypes.ServerCapabilities{
		Tools:     &mcptypes.ToolsCapability{ListChanged: false},
		Resources: &mcptypes.ResourcesCapability{ListChanged: false, Subscribe: false},
		Prompts:   &mcptypes.PromptsCapability{ListChanged: false},
	}

	appVersion := GetAppVersion(h.config)
	serverInfo := mcptypes.Implementation{Name: h.config.Server.Name, Version: appVersion}

	res := mcptypes.InitializeResult{
		ProtocolVersion: serverProtocolVersion,
		ServerInfo:      &serverInfo,
		Capabilities:    caps,
	}

	resBytes, err := json.Marshal(res)
	if err != nil {
		// Don't change state if response marshalling fails
		return nil, errors.Wrap(err, "failed to marshal initialize result")
	}

	// --- State Transition on Success ---
	// Transition state *after* successfully preparing the response
	h.connectionState.SetInitializing() // Or SetInitialized directly? Check MCP spec flow. Let's assume Initializing.
	h.logger.Info("Initialize successful, returning server capabilities. State -> Initializing.")

	return resBytes, nil
}

// handleShutdown handles the "shutdown" request.
// Acknowledges the request and sets the state.
//
//nolint:unparam
func (h *Handler) handleShutdown(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
	h.logger.Info("Handling shutdown request.")
	if h.connectionState == nil {
		h.logger.Error("Internal error: connectionState is nil in handleShutdown.")
		return nil, errors.New("internal server error: connection state missing")
	}
	// Validate sequence
	if err := h.connectionState.ValidateMethodSequence("shutdown"); err != nil {
		h.logger.Warn("Shutdown called in invalid state.", "state", h.connectionState.CurrentState())
		return nil, err
	}

	h.connectionState.SetShutdown() // Mark state
	h.logger.Info("Server state set to ShuttingDown.")
	// Actual server shutdown (closing transport, etc.) is handled by the Server.Shutdown method.
	return json.RawMessage(`null`), nil // Shutdown response result is null.
}

// handleExit handles the "exit" notification.
// Logs the event and signals the main application to terminate.
// Note: This is a notification handler, returns error only.
//
//nolint:unparam
func (h *Handler) handleExit(ctx context.Context, _ json.RawMessage) error {
	h.logger.Info("Handling exit notification.")
	if h.connectionState == nil {
		h.logger.Error("Internal error: connectionState is nil in handleExit.")
		// Allow exit even if state is nil? Or return error? Let's allow for now.
	}

	h.logger.Warn("Exit notification received. Server should terminate process if running standalone.")

	// Find the cancel function passed via context if available.
	type cancelCtxKey struct{}
	cancelFunc, ok := ctx.Value(cancelCtxKey{}).(context.CancelFunc)
	if ok && cancelFunc != nil {
		h.logger.Info("Calling context cancel function due to exit notification.")
		cancelFunc()
	} else {
		h.logger.Warn("No cancel function found in context for exit notification.")
		// Consider if os.Exit is appropriate here for stdio transport
	}

	return nil // Notifications don't have responses / return nil error on success.
}

// handleCancelRequest handles the "$/cancelRequest" notification.
// Logs the cancellation attempt. Returns error only.
//
//nolint:unparam // Notification handlers correctly return nil error.
func (h *Handler) handleCancelRequest(_ context.Context, params json.RawMessage) error {
	var reqParams struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(params, &reqParams); err != nil {
		h.logger.Warn("Failed to unmarshal $/cancelRequest params.", "error", err)
		return nil // Don't propagate parse errors for notifications.
	}
	// Validate state? Cancellation might only be valid in Initialized state?
	// Assume valid for now.
	h.logger.Info("Received cancellation request notification.", "requestId", string(reqParams.ID))
	// TODO: Implement actual request cancellation logic.
	return nil
}

// --- NEWLY ADDED ---
// handleNotificationsInitialized handles the notifications/initialized notification.
// Official definition: This notification is sent from the client to the server after
// initialization has finished. It signals that the client has successfully processed
// the server's initialization response and is ready for further communication.
// Returns error only as it's a notification handler.
//
//nolint:unparam // Notification handlers correctly return nil error.
func (h *Handler) handleNotificationsInitialized(_ context.Context, params json.RawMessage) error {
	h.logger.Info("Handling 'notifications/initialized' from client.")

	// Validate state: Should only occur after sending initialize response (state Initializing)
	if h.connectionState == nil {
		h.logger.Error("Internal error: connectionState is nil in handleNotificationsInitialized.")
		// This is bad, but can't send error response for notification. Log and proceed?
	} else {
		// Ensure we are in the Initializing state
		// NOTE: The original logic in handleInitialize set state to Initialized immediately.
		// The MCP spec flow might be: Client->Init Request -> Server sends Init Response (State -> Initializing) -> Client sends Init Notification -> Server processes (State -> Initialized)
		// Adjusting state logic based on this assumption.
		if h.connectionState.CurrentState() != StateInitializing {
			h.logger.Warn("Received notifications/initialized in unexpected state.", "state", h.connectionState.CurrentState())
			// Cannot send error response. Log and potentially ignore?
			return nil // Or return an internal error if needed for server logic? Let's return nil for now.
		}
		// Transition to fully initialized state
		h.connectionState.SetInitialized()
		h.logger.Info("Connection state set to Initialized following notifications/initialized.")
	}

	// Optional: Parse params if they contain useful info (like final capabilities)
	var notifParams struct {
		// Define fields if the spec includes params for this notification
	}
	if err := json.Unmarshal(params, &notifParams); err != nil {
		h.logger.Debug("Could not parse notifications/initialized params (might be empty or invalid).", "error", err)
	} else {
		// Process parsed params if needed
		h.logger.Debug("Parsed notifications/initialized params.", "params", fmt.Sprintf("%+v", notifParams))
	}

	// This is a notification (no response needed). Return nil error on success.
	return nil
}

// --- Helper ---

// GetAppVersion retrieves the application version (placeholder).
func GetAppVersion(_ *config.Config) string {
	// TODO: Read version from embedded build flags (e.g., ldflags using runtime/debug.ReadBuildInfo).
	// Remove reliance on config field for now.
	// if cfg != nil && cfg.Server.Version != "" { // <- Remove or comment out this block
	//     return cfg.Server.Version
	// }
	return "0.1.0-dev" // Default fallback.
}
