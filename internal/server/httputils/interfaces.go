// internal/server/httputils/interfaces.go
package httputils

import (
	"os"
)

// RTMServiceInterface defines the interface for interacting with RTM service
type RTMServiceInterface interface {
	IsAuthenticated() bool
	StartAuthFlow() (string, string, error)
	CompleteAuthFlow(frob string) error
	ClearAuthentication() error
	GetActiveAuthFlows() int
}

// TokenManagerInterface defines the interface for token management
type TokenManagerInterface interface {
	HasToken() bool
	GetTokenFileInfo() (os.FileInfo, error)
}

// ServerInterface defines the public interface needed by handlers
type ServerInterface interface {
	GetRTMService() RTMServiceInterface
	GetTokenManager() TokenManagerInterface
}
