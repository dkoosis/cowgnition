// file: internal/rtm/factory.go
package rtm

import (
	"context"

	"github.com/dkoosis/cowgnition/internal/config"
	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/mcp"
)

// ServiceFactory creates RTM services.
type ServiceFactory struct {
	config *config.Config
	logger logging.Logger
}

// NewServiceFactory creates a new RTM service factory.
func NewServiceFactory(cfg *config.Config, logger logging
