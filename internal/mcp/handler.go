// file: internal/mcp/handler.go

package mcp

import (
	// Keep only imports needed for Handler struct and NewHandler.
	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	// Import RTM client package here when created.
	// "github.com/dkoosis/cowgnition/internal/rtm".
)

// Handler holds dependencies for MCP method handlers.
type Handler struct {
	logger logging.Logger
	config *config.Config
	// Add RTM client instance here when available:
	// rtmClient *rtm.Client.
}

// NewHandler creates a new Handler.
func NewHandler(cfg *config.Config, logger logging.Logger) *Handler {
	// TODO: Initialize RTM client here when implemented, passing cfg.RTM.APIKey etc.
	return &Handler{
		logger: logger.WithField("component", "mcp_handler"),
		config: cfg,
		// rtmClient: rtm.NewClient(cfg.RTM.APIKey, cfg.RTM.SharedSecret, logger), // Example.
	}
}
