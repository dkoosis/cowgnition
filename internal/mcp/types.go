// Package mcp defines core types, constants, and interfaces for the
// Machine Control Protocol (MCP).
package mcp

import (
	"context"
	"time"
	// Import definitions for use in contracts
	// IMPORTANT: Replace with the correct import path for your module
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
)

// State represents the different states a connection managed by the MCP can be in.
type State string // Renamed from ConnectionState

const (
	StateUnconnected  State = "unconnected"
	StateInitializing State = "initializing"
	StateConnected    State = "connected"
	StateTerminating  State = "terminating"
	StateError        State = "error"
)

func (s State) String() string { return string(s) }

// Trigger represents events that can cause state transitions within the MCP state machine.
type Trigger string

const (
	TriggerInitialize       Trigger = "Initialize"
	TriggerInitSuccess      Trigger = "InitSuccess"
	TriggerInitFailure      Trigger = "InitFailure"
	TriggerListResources    Trigger = "ListResources"
	TriggerReadResource     Trigger = "ReadResource"
	TriggerListTools        Trigger = "ListTools"
	TriggerCallTool         Trigger = "CallTool"
	TriggerShutdown         Trigger = "Shutdown"
	TriggerShutdownComplete Trigger = "ShutdownComplete"
	TriggerErrorOccurred    Trigger = "ErrorOccurred"
	TriggerDisconnect       Trigger = "Disconnect"
	TriggerPing             Trigger = "Ping"
	TriggerSubscribe        Trigger = "Subscribe"
)

func (t Trigger) String() string { return string(t) }

// ResourceManagerContract defines the interface expected by the connection manager
// for resource management operations. Implementations adapt specific resource sources.
type ResourceManagerContract interface {
	// GetAllResourceDefinitions returns metadata for all available resources.
	GetAllResourceDefinitions() []definitions.ResourceDefinition // Use specific type

	// ReadResource reads a resource with the given name and arguments.
	// Returns the resource content as a string, its MIME type, and any error.
	ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) // Use specific type
}

// ToolManagerContract defines the interface expected by the connection manager
// for tool management operations. Implementations adapt specific tool execution backends.
type ToolManagerContract interface {
	// GetAllToolDefinitions returns metadata for all available tools.
	GetAllToolDefinitions() []definitions.ToolDefinition // Use specific type

	// CallTool attempts to execute a tool with the given name and arguments.
	// Returns the result of the tool execution as a string and any error.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) // Use specific type
}

// IsCompatibleProtocolVersion checks if the client's protocol version is compatible.
func IsCompatibleProtocolVersion(clientVersion string) bool {
	supportedVersions := map[string]bool{"2.0": true, "2024-11-05": true}
	return supportedVersions[clientVersion]
}

// --- Base Server Interfaces/Types ---
// Assuming these base interfaces/types are defined elsewhere in package mcp,
// potentially in internal/mcp/server.go or similar.
// These are needed by the connection.ConnectionServer.

type Server struct {
	// ... fields for the base server (config, transport, base managers) ...
	config          Config // Example config type
	version         string
	requestTimeout  time.Duration
	shutdownTimeout time.Duration
	resourceManager ResourceManager // Base resource manager interface/type
	toolManager     ToolManager     // Base tool manager interface/type
	transport       string
}

// Example base config interface needed by ConnectionServer setup
type Config interface {
	GetServerName() string
	// ... other config methods ...
}

// Example base ResourceManager interface needed by adapter
type ResourceManager interface {
	GetAllResourceDefinitions() []definitions.ResourceDefinition
	ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error)
}

// Example base ToolManager interface needed by adapter
type ToolManager interface {
	GetAllToolDefinitions() []definitions.ToolDefinition
	CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error)
}

// Add methods for Server if needed, e.g., startHTTP
func (s *Server) startHTTP() error { /* ... implementation ... */ return nil }
