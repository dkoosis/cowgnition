// file: internal/jsonrpc/utils.go
package jsonrpc

import (
	"encoding/json"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/logging"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
	"github.com/sourcegraph/jsonrpc2"
)

// Initialize the logger at the package level.
var utilsLogger = logging.GetLogger("jsonrpc_utils")

// ParseParams unmarshals the params of a jsonrpc2.Request into the specified struct
// with consistent error handling that matches our project's error pattern.
func ParseParams(req *jsonrpc2.Request, dst interface{}) error {
	if req.Params == nil {
		return nil
	}

	if err := json.Unmarshal(*req.Params, dst); err != nil {
		return cgerr.ErrorWithDetails(
			errors.Wrap(err, "failed to unmarshal params"),
			cgerr.CategoryRPC,
			cgerr.CodeInvalidParams,
			map[string]interface{}{
				"method":      req.Method,
				"target_type": fmt.Sprintf("%T", dst),
				"params_size": len(*req.Params),
			},
		)
	}
	return nil
}

// FormatRequestID safely formats a request ID as a string for logging purposes.
// This avoids potential issues with different ID types (numbers, strings, null).
func FormatRequestID(id interface{}) string {
	if id == nil {
		return "null"
	}
	return fmt.Sprintf("%v", id)
}

// IsNotification checks if a request is a notification (has no ID).
func IsNotification(req *jsonrpc2.Request) bool {
	return req.ID == nil
}

// CreateErrorResponse creates a JSON-RPC 2.0 error response using our cgerr error format.
// This is a convenience function for generating error responses in handler code.
func CreateErrorResponse(id interface{}, err error) (*jsonrpc2.Response, error) {
	rpcErr := cgerr.ToJSONRPCError(err)

	// Create a JSON-RPC 2.0 response
	resp := &jsonrpc2.Response{
		ID:    id,
		Error: rpcErr,
	}

	return resp, nil
}
