// Package mcp implements the Model Context Protocol server logic, including handlers and types.
// File: internal/mcp/mcp_server_metrics.go.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/cockroachdb/errors"
	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Import the shared types package.
	"github.com/dkoosis/cowgnition/internal/metrics"
	"github.com/dkoosis/cowgnition/internal/rtm" // Import rtm package to check type.
	// Needed for Service interface.
)

// Global metrics collector instance.
var globalMetricsCollector *metrics.Collector // FIX: Use renamed type 'Collector'.

// InitializeMetricsCollector sets up the global metrics collector.
func InitializeMetricsCollector() {
	if globalMetricsCollector == nil {
		globalMetricsCollector = metrics.NewMetricsCollector(20) // Keep last 20 errors. FIX: Use renamed type 'Collector'.
	}
}

// GetMetricsCollector returns the global metrics collector instance.
func GetMetricsCollector() *metrics.Collector { // FIX: Use renamed type 'Collector'.
	if globalMetricsCollector == nil {
		InitializeMetricsCollector()
	}
	return globalMetricsCollector
}

// ReadServerHealthMetrics retrieves the current server health metrics.
func (s *Server) ReadServerHealthMetrics(_ context.Context) ([]interface{}, error) { // Removed ctx.
	collector := GetMetricsCollector()
	// Ensure metrics are up-to-date before adding service-specific info.
	currentMetrics := collector.GetCurrentMetrics() // Get potentially stale metrics first.

	// Enrich with RTM-specific information if available.
	rtmSvcRaw, found := s.GetService("rtm") // Use registry lookup.
	if found {
		// Attempt to type-assert to the concrete RTM service type.
		rtmSvc, ok := rtmSvcRaw.(*rtm.Service) // Use concrete type from rtm package.
		if ok && rtmSvc != nil {
			method, path, _ := rtmSvc.GetTokenStorageInfo()
			// Update the collector directly; GetCurrentMetrics called later will reflect this.
			collector.UpdateRTMAuthStatus(
				rtmSvc.IsAuthenticated(),
				rtmSvc.GetUsername(), // Get username from the service.
				method,
				path,
			)
			s.logger.Debug("Enriched metrics collector with RTM auth status.")
			// Refresh metrics AFTER updating collector state.
			currentMetrics = collector.GetCurrentMetrics()
		} else {
			s.logger.Warn("Found 'rtm' service in registry, but it has an unexpected type.")
		}
	} else {
		s.logger.Debug("RTM service not found in registry, skipping RTM auth status in metrics.")
	}

	// Marshal the potentially updated metrics to JSON and return as resource.
	jsonData, err := json.MarshalIndent(currentMetrics, "", "  ")
	if err != nil {
		s.logger.Error("Failed to marshal server health metrics.", "error", err)
		return nil, errors.Wrap(err, "failed to marshal server health metrics")
	}

	return []interface{}{
		mcptypes.TextResourceContents{
			ResourceContents: mcptypes.ResourceContents{
				URI:      "cowgnition://health", // Use consistent health URI.
				MimeType: "application/json",
			},
			Text: string(jsonData),
		},
	}, nil
}

// RecordRequestMetrics records metrics for an MCP request.
func (s *Server) RecordRequestMetrics(method string, startTime time.Time, err error) {
	latencyMs := int(time.Since(startTime).Milliseconds())
	GetMetricsCollector().RecordRequest(method, latencyMs, err == nil)

	if err != nil {
		stack := string(debug.Stack())
		errorMsg := fmt.Sprintf("%v", err)
		GetMetricsCollector().RecordError("mcp_server", errorMsg, stack)
	}
}

// RecordConnection records a new connection.
func (s *Server) RecordConnection(connectionID string, connected bool) {
	GetMetricsCollector().RecordConnection(connectionID, connected)
}

// RecordConnectionFailure records a failed connection attempt.
func (s *Server) RecordConnectionFailure() {
	GetMetricsCollector().RecordConnectionFailure()
}
