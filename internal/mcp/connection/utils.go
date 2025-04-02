// internal/mcp/connection/utils.go
package connection

import (
	"fmt"
	"time"
)

// Constants for log levels
const (
	LogLevelDebug = "DEBUG"
	LogLevelInfo  = "INFO"
	LogLevelWarn  = "WARN"
	LogLevelError = "ERROR"
)

// generateConnectionID creates a unique ID for the connection.
func generateConnectionID() string {
	return fmt.Sprintf("conn_%d", time.Now().UnixNano())
}

// logf logs a message with the given level and format.
func (m *ConnectionManager) logf(level string, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	m.logger.Printf("[%s] [%s] [%s] %s", level, m.connectionID, m.state, message)
}
