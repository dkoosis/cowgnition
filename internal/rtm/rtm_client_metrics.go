// file: internal/rtm/rtm_client_metrics.go
package rtm

import (
	"context"
	"encoding/json"
	"time"
)

// Function type for a metrics collector to avoid direct dependency on mcp
type MetricsCollector interface {
	RecordRTMAPICall(latencyMs int, err error)
}

// Global variable to store the metrics collector
var metricsCollector MetricsCollector

// SetMetricsCollector allows setting the metrics collector from outside
func SetMetricsCollector(collector MetricsCollector) {
	metricsCollector = collector
}

// CallMethodWithMetrics provides a metrics-wrapped version of the CallMethod function
func (c *Client) CallMethodWithMetrics(ctx context.Context, method string, params map[string]string) (json.RawMessage, error) {
	startTime := time.Now()

	// Call the original method
	result, err := c.CallMethod(ctx, method, params)

	// Record metrics if the collector is available
	if metricsCollector != nil {
		latencyMs := int(time.Since(startTime).Milliseconds())
		metricsCollector.RecordRTMAPICall(latencyMs, err)
	}

	return result, err
}

// wrapCallMethodWithMetrics wraps the internal callMethod function to record metrics
func (c *Client) wrapCallMethodWithMetrics(ctx context.Context, method string, params map[string]string) (json.RawMessage, error) {
	startTime := time.Now()

	// Call the internal method
	result, err := c.callMethod(ctx, method, params)

	// Record metrics if the collector is available
	if metricsCollector != nil {
		latencyMs := int(time.Since(startTime).Milliseconds())
		metricsCollector.RecordRTMAPICall(latencyMs, err)
	}

	return result, err
}
