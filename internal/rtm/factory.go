// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/factory.go

import (
	"context"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
)

// ServiceFactory creates RTM services.
type ServiceFactory struct {
	config *config.Config
	logger logging.Logger
}

// NewServiceFactory creates a new RTM service factory.
func NewServiceFactory(cfg *config.Config, logger logging.Logger) *ServiceFactory {
	// Use no-op logger if not provided
	if logger == nil {
		logger = logging.GetNoopLogger()
	}

	return &ServiceFactory{
		config: cfg,
		logger: logger.WithField("component", "rtm_factory"),
	}
}

// CreateService creates a new RTM service instance.
func (f *ServiceFactory) CreateService(ctx context.Context) (*Service, error) {
	service := NewService(f.config, f.logger)

	// Initialize the service
	if err := service.Initialize(ctx); err != nil {
		f.logger.Error("Failed to initialize RTM service", "error", err)
		return nil, err
	}

	f.logger.Info("Created RTM service",
		"authenticated", service.IsAuthenticated(),
		"username", service.GetUsername())

	return service, nil
}

// CreateClient creates a new RTM client without creating a full service.
// This is useful for components that need to interact with RTM directly
// but don't need the MCP integration.
func (f *ServiceFactory) CreateClient() *Client {
	rtmConfig := Config{
		APIKey:       f.config.RTM.APIKey,
		SharedSecret: f.config.RTM.SharedSecret,
	}

	return NewClient(rtmConfig, f.logger)
}
