// file: internal/rtm/client_metrics.go
package rtm

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dkoosis/cowgnition/internal/mcp"
)

// wrapCallMethodWithMetrics wraps the callMethod function to record metrics
func (c *Client) wrapCallMethodWithMetrics(ctx context.Context, method string, params map[string]string) (json.RawMessage, error) {
	startTime := time.Now()

	// Call the original method
	result, err := c.callMethod(ctx, method, params)

	// Record metrics if the metrics collector is available
	if collector := mcp.GetMetricsCollector(); collector != nil {
		latencyMs := int(time.Since(startTime).Milliseconds())
		collector.RecordRTMAPICall(latencyMs, err)
	}

	return result, err
}

// UpdateCallMethodToRecordMetrics updates the Client.CallMethod to record metrics
// This is a bit of a hack - in a real implementation, we'd modify the client.go
// file directly to include metrics in CallMethod, but for this example, we're
// demonstrating how to patch it in
func (c *Client) UpdateCallMethodToRecordMetrics() {
	// The original client.CallMethod function
	originalCallMethod := c.CallMethod

	// Replace it with a version that records metrics
	c.CallMethod = func(ctx context.Context, method string, params map[string]string) (json.RawMessage, error) {
		startTime := time.Now()

		// Call the original method
		result, err := originalCallMethod(ctx, method, params)

		// Record metrics if the metrics collector is available
		if collector := mcp.GetMetricsCollector(); collector != nil {
			latencyMs := int(time.Since(startTime).Milliseconds())
			collector.RecordRTMAPICall(latencyMs, err)
		}

		return result, err
	}
}
