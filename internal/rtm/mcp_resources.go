// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
// file: internal/rtm/mcp_resources.go
// This file previously defined the GetResources list and ReadResource handler for MCP.
// This logic has now been moved into internal/rtm/service.go as part of the.
// services.Service interface implementation.
package rtm

// NOTE: GetResources function has been moved to internal/rtm/service.go.
// NOTE: ReadResource function has been moved to internal/rtm/service.go.
// NOTE: Resource reading helpers (readAuthResource, etc.) moved into service.go.
// NOTE: Resource content helpers (createJSONResourceContent, etc.) moved into service.go.
