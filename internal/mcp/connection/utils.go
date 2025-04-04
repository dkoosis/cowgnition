// file: internal/mcp/connection/utils.go
package connection

import (
	"fmt"

	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
)

// logFormatter formats a log message with connection ID and state.
//
//nolint:unused
func logFormatter(level definitions.LogLevel, connectionID string, state State, format string, args ...interface{}) string {
	// Format: [LEVEL] [ConnectionID] [State] Message
	return fmt.Sprintf("[%s] [%s] [%s] %s", level, connectionID, state, fmt.Sprintf(format, args...))
}
