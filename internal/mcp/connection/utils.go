// internal/mcp/connection/utils.go
package connection

import (
	"fmt"
)

// logFormatter formats a log message with connection ID and state.
func logFormatter(level LogLevel, connectionID string, state ConnectionState, format string, args ...interface{}) string {
	// Format: [LEVEL] [ConnectionID] [State] Message
	return fmt.Sprintf("[%s] [%s] [%s] %s", level, connectionID, state, fmt.Sprintf(format, args...))
}
