// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
// file: internal/rtm/rtm_client_metrics.go.
package rtm

import (
	"context"
	"encoding/json"
	"time"
)

// MetricsCollector defines the interface for collecting RTM API call metrics.
// FIX: Updated comment format.
type MetricsCollector interface {
	RecordRTMAPICall(latencyMs int, err error)
}

// Global variable to store the metrics collector.
// FIX: Added period.
var metricsCollector MetricsCollector

// SetMetricsCollector allows setting the metrics collector from outside.
// FIX: Added period.
func SetMetricsCollector(collector MetricsCollector) {
	metricsCollector = collector
}

// CallMethodWithMetrics provides a metrics-wrapped version of the CallMethod function.
// FIX: Added period.
func (c *Client) CallMethodWithMetrics(ctx context.Context, method string, params map[string]string) (json.RawMessage, error) {
	startTime := time.Now()

	// Call the original method.
	result, err := c.CallMethod(ctx, method, params)

	// Record metrics if the collector is available.
	if metricsCollector != nil {
		latencyMs := int(time.Since(startTime).Milliseconds())
		metricsCollector.RecordRTMAPICall(latencyMs, err)
	}

	return result, err
}

// wrapCallMethodWithMetrics wraps the internal callMethod function to record metrics.
// FIX: Added period.
// nolint:unused // Keeping function as requested, suppressing unused warning.
func (c *Client) wrapCallMethodWithMetrics(ctx context.Context, method string, params map[string]string) (json.RawMessage, error) {
	startTime := time.Now()

	// Call the internal method.
	result, err := c.callMethod(ctx, method, params)

	// Record metrics if the collector is available.
	if metricsCollector != nil {
		latencyMs := int(time.Since(startTime).Milliseconds())
		metricsCollector.RecordRTMAPICall(latencyMs, err)
	}

	return result, err
}
