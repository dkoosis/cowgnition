// Package metrics provides structures and functions for collecting and managing server health and performance metrics.
// file: internal/metrics/server_metrics.go.
package metrics

import (
	"runtime"
	"sync"
	"time"
)

// ServerMetrics holds various metrics about the server's health and performance.
type ServerMetrics struct {
	// Server uptime and basic info.
	StartTime     time.Time     `json:"startTime"`
	Uptime        time.Duration `json:"uptime"`
	GoVersion     string        `json:"goVersion"`
	NumGoroutines int           `json:"numGoroutines"`

	// Memory stats.
	MemoryAllocated   uint64 `json:"memoryAllocated"`   // Currently allocated memory in bytes.
	MemoryTotalAlloc  uint64 `json:"memoryTotalAlloc"`  // Total allocated memory since start.
	MemorySystemTotal uint64 `json:"memorySystemTotal"` // Total memory obtained from system.
	MemoryGCCount     uint32 `json:"memoryGCCount"`     // Number of completed GC cycles.

	// Connection stats.
	ActiveConnections int `json:"activeConnections"`
	TotalConnections  int `json:"totalConnections"`
	FailedConnections int `json:"failedConnections"`

	// Request stats.
	TotalRequests    int            `json:"totalRequests"`
	FailedRequests   int            `json:"failedRequests"`
	RequestLatencies map[string]int `json:"requestLatencies"` // Method to average ms.

	// RTM API stats.
	RTMAPICallCount    int `json:"rtmApiCallCount"`
	RTMAPIErrorCount   int `json:"rtmApiErrorCount"`
	RTMAPIAvgLatencyMs int `json:"rtmApiAvgLatencyMs"`

	// Authentication status.
	RTMAuthenticated   bool   `json:"rtmAuthenticated"`
	RTMUsername        string `json:"rtmUsername,omitempty"`
	TokenStorageMethod string `json:"tokenStorageMethod"`
	TokenStoragePath   string `json:"tokenStoragePath,omitempty"`

	// Last errors.
	LastErrors []ErrorInfo `json:"lastErrors,omitempty"`
}

// ErrorInfo contains details about an error that occurred.
type ErrorInfo struct {
	Timestamp time.Time `json:"timestamp"`
	Component string    `json:"component"`
	Message   string    `json:"message"`
	Stack     string    `json:"stack,omitempty"`
}

// Collector manages server metrics collection and reporting.
// FIX: Renamed from MetricsCollector to avoid stutter (metrics.Collector).
type Collector struct {
	metrics     ServerMetrics
	startTime   time.Time
	errorBuffer []ErrorInfo
	bufferSize  int
	mu          sync.RWMutex

	// Connection tracking.
	activeConnections map[string]bool // Map of connection IDs to status.
}

// NewMetricsCollector creates a new metrics collector instance.
func NewMetricsCollector(errorBufferSize int) *Collector { // FIX: Returns *Collector
	startTime := time.Now()

	// FIX: Returns &Collector
	return &Collector{
		metrics: ServerMetrics{
			StartTime:        startTime,
			GoVersion:        runtime.Version(),
			RequestLatencies: make(map[string]int),
		},
		startTime:         startTime,
		errorBuffer:       make([]ErrorInfo, 0, errorBufferSize),
		bufferSize:        errorBufferSize,
		activeConnections: make(map[string]bool),
	}
}

// GetCurrentMetrics returns a copy of the current server metrics.
func (c *Collector) GetCurrentMetrics() ServerMetrics { // FIX: Receiver is *Collector
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Update real-time metrics.
	c.metrics.Uptime = time.Since(c.startTime)
	c.metrics.NumGoroutines = runtime.NumGoroutine()

	// Update memory stats.
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	c.metrics.MemoryAllocated = memStats.Alloc
	c.metrics.MemoryTotalAlloc = memStats.TotalAlloc
	c.metrics.MemorySystemTotal = memStats.Sys
	c.metrics.MemoryGCCount = memStats.NumGC

	// Copy the metrics to avoid race conditions.
	metricsCopy := c.metrics

	// Create a copy of the error buffer.
	if len(c.errorBuffer) > 0 {
		metricsCopy.LastErrors = make([]ErrorInfo, len(c.errorBuffer))
		copy(metricsCopy.LastErrors, c.errorBuffer)
	}

	return metricsCopy
}

// RecordRequest records statistics about a request.
func (c *Collector) RecordRequest(method string, latencyMs int, success bool) { // FIX: Receiver is *Collector
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metrics.TotalRequests++
	if !success {
		c.metrics.FailedRequests++
	}

	// Update average latency for this method.
	if existing, ok := c.metrics.RequestLatencies[method]; ok {
		// Simple moving average.
		c.metrics.RequestLatencies[method] = (existing + latencyMs) / 2
	} else {
		c.metrics.RequestLatencies[method] = latencyMs
	}
}

// RecordRTMAPICall records statistics about RTM API calls.
func (c *Collector) RecordRTMAPICall(latencyMs int, err error) { // FIX: Receiver is *Collector
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metrics.RTMAPICallCount++
	if err != nil {
		c.metrics.RTMAPIErrorCount++
	}

	// Update average latency.
	if c.metrics.RTMAPICallCount > 1 {
		// Corrected calculation to prevent integer division potentially yielding 0.
		// Convert to float64 for calculation, then back to int.
		c.metrics.RTMAPIAvgLatencyMs = int((float64(c.metrics.RTMAPIAvgLatencyMs*(c.metrics.RTMAPICallCount-1)) + float64(latencyMs)) / float64(c.metrics.RTMAPICallCount))
	} else {
		c.metrics.RTMAPIAvgLatencyMs = latencyMs
	}
}

// RecordConnection tracks connection statistics.
func (c *Collector) RecordConnection(connectionID string, active bool) { // FIX: Receiver is *Collector
	c.mu.Lock()
	defer c.mu.Unlock()

	if active {
		// New active connection.
		if !c.activeConnections[connectionID] { // Only increment TotalConnections if it's a *new* active connection
			c.activeConnections[connectionID] = true
			c.metrics.TotalConnections++
		}
	} else {
		// Connection closed.
		delete(c.activeConnections, connectionID)
	}

	// Update active count.
	c.metrics.ActiveConnections = len(c.activeConnections)
}

// RecordConnectionFailure increments the failed connections counter.
func (c *Collector) RecordConnectionFailure() { // FIX: Receiver is *Collector
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metrics.FailedConnections++
}

// RecordError adds an error to the error buffer.
func (c *Collector) RecordError(component, message, stack string) { // FIX: Receiver is *Collector
	c.mu.Lock()
	defer c.mu.Unlock()

	errorInfo := ErrorInfo{
		Timestamp: time.Now(),
		Component: component,
		Message:   message,
		Stack:     stack,
	}

	// Add to the circular buffer.
	if len(c.errorBuffer) >= c.bufferSize {
		// Remove oldest error.
		c.errorBuffer = c.errorBuffer[1:]
	}

	c.errorBuffer = append(c.errorBuffer, errorInfo)
}

// UpdateRTMAuthStatus updates the RTM authentication status metrics.
func (c *Collector) UpdateRTMAuthStatus(authenticated bool, username string, storageMethod, storagePath string) { // FIX: Receiver is *Collector
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metrics.RTMAuthenticated = authenticated
	c.metrics.RTMUsername = username
	c.metrics.TokenStorageMethod = storageMethod
	c.metrics.TokenStoragePath = storagePath
}
