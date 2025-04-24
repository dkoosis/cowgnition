// Package services defines common interfaces for different service integrations
// within CowGnition, allowing the core MCP server to interact with them generically.
// file: internal/services/service.go
package services

import (
	"context"
	"encoding/json"

	// NOTE: "fmt" import removed as it's no longer used in this file after removing GetPrompt.

	mcptypes "github.com/dkoosis/cowgnition/internal/mcp_types" // Use mcptypes alias.
	// NOTE: mcperrors import is NOT needed here, as it was only used by the misplaced GetPrompt.
)

// Service defines the standard interface for all backend service integrations (e.g., RTM, Calendar).
// Each service implementation provides its specific capabilities (tools, resources)
// and handles requests routed to it by the main MCP server.
// Configuration specific to a service should be handled during its instantiation (constructor).
type Service interface {
	// GetName returns the unique identifier for the service (e.g., "rtm").
	// This name is used for routing requests and should be lowercase.
	GetName() string

	// GetTools returns a list of MCP Tool definitions provided by this service.
	// Tool names should follow the convention "serviceName_toolName" (e.g., "rtm_getTasks").
	GetTools() []mcptypes.Tool

	// GetResources returns a list of MCP Resource definitions provided by this service.
	// Resource URIs should ideally use a scheme related to the service name (e.g., "rtm://lists").
	GetResources() []mcptypes.Resource

	// ReadResource handles requests to read data from a resource provided by this service.
	// The uri is the specific resource URI being requested (e.g., "rtm://lists").
	// Returns the resource content (typically as []interface{} containing structs like
	// mcptypes.TextResourceContents or mcptypes.BlobResourceContents) or an error if reading fails.
	ReadResource(ctx context.Context, uri string) ([]interface{}, error)

	// CallTool handles requests to execute a tool provided by this service.
	// The name is the specific tool name (e.g., "rtm_getTasks"), and args contains
	// the parameters provided by the client as raw JSON.
	// Returns the result of the tool execution (mcptypes.CallToolResult) or an error
	// only if the *handling* of the call fails (e.g., internal server error before
	// or after tool execution). Errors *within* the tool's logic (e.g., RTM API error)
	// should be reported within the returned mcptypes.CallToolResult by setting IsError=true.
	CallTool(ctx context.Context, name string, args json.RawMessage) (*mcptypes.CallToolResult, error)

	// GetPrompt handles requests to retrieve a specific prompt template.
	// The name is the prompt name (e.g., "rtm_generateReport"), and args contains
	// the parameters provided by the client.
	// Returns the prompt messages and structure (mcptypes.GetPromptResult) or an error.
	GetPrompt(ctx context.Context, name string, args map[string]string) (*mcptypes.GetPromptResult, error)

	// Initialize performs any necessary setup for the service after instantiation,
	// such as loading initial state or verifying credentials.
	// It should be called once before the service is used by the MCP server.
	Initialize(ctx context.Context) error

	// Shutdown performs cleanup tasks for the service, like closing connections
	// or saving state before the application exits.
	Shutdown() error

	// IsAuthenticated returns true if the service currently has valid authentication
	// credentials or state needed to perform its operations. Returns false otherwise.
	// For services not requiring authentication, this may always return true.
	IsAuthenticated() bool
}

// NO METHOD DEFINITIONS FOR THE INTERFACE ITSELF SHOULD BE HERE.
