// file: internal/mcp/mcp_server_metrics.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/metrics"
)

// Global metrics collector instance
var globalMetricsCollector *metrics.MetricsCollector

// InitializeMetricsCollector sets up the global metrics collector
func InitializeMetricsCollector() {
	if globalMetricsCollector == nil {
		globalMetricsCollector = metrics.NewMetricsCollector(20) // Keep last 20 errors
	}
}

// GetMetricsCollector returns the global metrics collector instance
func GetMetricsCollector() *metrics.MetricsCollector {
	if globalMetricsCollector == nil {
		InitializeMetricsCollector()
	}
	return globalMetricsCollector
}

// ReadServerHealthMetrics retrieves the current server health metrics
func (s *Server) ReadServerHealthMetrics(ctx context.Context) ([]interface{}, error) {
	collector := GetMetricsCollector()
	currentMetrics := collector.GetCurrentMetrics()

	// Enrich with RTM-specific information if available
	if s.rtmService != nil {
		method, path, available := s.rtmService.GetTokenStorageInfo()
		collector.UpdateRTMAuthStatus(
			s.rtmService.IsAuthenticated(),
			s.rtmService.GetUsername(),
			method,
			path,
		)
	}

	// Marshal the metrics to JSON and return as resource
	jsonData, err := json.MarshalIndent(currentMetrics, "", "  ")
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal server health metrics")
	}

	return []interface{}{
		TextResourceContents{
			ResourceContents: ResourceContents{
				URI:      "cowgnition://server/health",
				MimeType: "application/json",
			},
			Text: string(jsonData),
		},
	}, nil
}

// RecordRequestMetrics records metrics for an MCP request
func (s *Server) RecordRequestMetrics(method string, startTime time.Time, err error) {
	latencyMs := int(time.Since(startTime).Milliseconds())
	GetMetricsCollector().RecordRequest(method, latencyMs, err == nil)

	if err != nil {
		stack := string(debug.Stack())
		errorMsg := fmt.Sprintf("%v", err)
		GetMetricsCollector().RecordError("mcp_server", errorMsg, stack)
	}
}

// RecordConnection records a new connection
func (s *Server) RecordConnection(connectionID string, connected bool) {
	GetMetricsCollector().RecordConnection(connectionID, connected)
}

// RecordConnectionFailure records a failed connection attempt
func (s *Server) RecordConnectionFailure() {
	GetMetricsCollector().RecordConnectionFailure()
}
