// Package mcp implements the Model Context Protocol server logic, including handlers and types.
package mcp

// file: internal/mcp/handler.go.

import (
	// Keep only imports needed for Handler struct and NewHandler.
	"time"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"

	// Import schema package for the interface.
	"github.com/dkoosis/cowgnition/internal/schema"
	// Import RTM client package here when created.
	// "github.com/dkoosis/cowgnition/internal/rtm".
)

// Handler holds dependencies for MCP method handlers.
type Handler struct {
	logger logging.Logger
	config *config.Config
	// Change field to use the interface type.
	// Corrected: Use ValidatorInterface.
	validator       schema.ValidatorInterface
	startTime       time.Time
	connectionState *ConnectionState
	// rtmClient *rtm.Client // Add RTM client when needed.
}

// NewHandler creates a new Handler instance with necessary dependencies.
// Change parameter to accept the interface type.
// It initializes the MCP handler with configuration, schema validator, server start time,
// connection state tracking, and a logger.
// Corrected: Use ValidatorInterface.
func NewHandler(cfg *config.Config, validator schema.ValidatorInterface, startTime time.Time, connState *ConnectionState, logger logging.Logger) *Handler {
	// TODO: Initialize RTM client here when implemented.
	return &Handler{
		logger:          logger.WithField("component", "mcp_handler"),
		config:          cfg,
		validator:       validator, // Store the interface.
		startTime:       startTime, // Store startTime.
		connectionState: connState, // Store connectionState.
		// rtmClient: rtm.NewClient(...).
	}
}

// --- All other handler methods (handleInitialize, handlePing, etc.) are in separate files (handlers_*.go) ---.
// --- They are methods on the *Handler struct and don't need direct changes in *this* file. ---.
