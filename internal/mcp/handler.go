// file: internal/mcp/handler.go

package mcp

import (
	// Keep only imports needed for Handler struct and NewHandler.
	"time"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/schema"
	// Import RTM client package here when created.
	// "github.com/dkoosis/cowgnition/internal/rtm".
)

// Handler holds dependencies for MCP method handlers.
type Handler struct {
	logger          logging.Logger
	config          *config.Config
	validator       *schema.SchemaValidator
	startTime       time.Time
	connectionState *ConnectionState
	// rtmClient *rtm.Client
}

func NewHandler(cfg *config.Config, validator *schema.SchemaValidator, startTime time.Time, connState *ConnectionState, logger logging.Logger) *Handler {
	// TODO: Initialize RTM client here when implemented
	return &Handler{
		logger:          logger.WithField("component", "mcp_handler"),
		config:          cfg,
		validator:       validator, // Store validator
		startTime:       startTime, // Store startTime
		connectionState: connState, // Store connectionState
		// rtmClient: rtm.NewClient(...)
	}
}
