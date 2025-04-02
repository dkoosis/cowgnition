// Package mcp defines interfaces and structures for managing resources within the Model Context Protocol (MCP).
// file: internal/mcp/resource.go
package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// ResourceProvider defines an interface for components that provide MCP resources.
// This interface abstracts the underlying resource access mechanism.
type ResourceProvider interface {
	// GetResourceDefinitions returns the list of resources this provider handles.
	// This allows the ResourceManager to discover available resources.
	GetResourceDefinitions() []ResourceDefinition

	// ReadResource attempts to read the content of a resource with the given name and arguments.
	// Returns the resource content, MIME type, and any error encountered.
	// The context is used to manage timeouts and cancellations.
	ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error)
}

// ResourceManager manages all registered resource providers.
// It acts as a central registry and access point for resources.
type ResourceManager struct {
	providers []ResourceProvider // providers holds the registered ResourceProviders.
}

// NewResourceManager creates a new resource manager.
// It initializes the ResourceManager with an empty list of providers.
func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		providers: []ResourceProvider{}, // Initialize with no providers.
	}
}

// RegisterProvider registers a ResourceProvider.
// This adds a provider to the list of available providers,
// allowing the ResourceManager to access its resources.
func (rm *ResourceManager) RegisterProvider(provider ResourceProvider) {
	rm.providers = append(rm.providers, provider) // Add the provider to the list.
}

// GetAllResourceDefinitions returns all resource definitions from all providers.
// This aggregates the definitions from each provider into a single list,
// providing a comprehensive view of available resources.
func (rm *ResourceManager) GetAllResourceDefinitions() []ResourceDefinition {
	var allResources []ResourceDefinition
	for _, provider := range rm.providers {
		allResources = append(allResources, provider.GetResourceDefinitions()...) // Collect definitions from each provider.
	}
	return allResources
}

// FindResourceProvider finds the provider for a specific resource name.
// It iterates through the registered providers to locate the one
// that handles the resource with the given name.
// If no provider is found, it returns an error with a list of available resources
// to aid in debugging.
func (rm *ResourceManager) FindResourceProvider(name string) (ResourceProvider, error) {
	for _, provider := range rm.providers {
		for _, res := range provider.GetResourceDefinitions() {
			if res.Name == name {
				return provider, nil // Return the provider if found.
			}
		}
	}

	// Get all available resource names for better error context
	var availableResources []string
	for _, provider := range rm.providers {
		for _, res := range provider.GetResourceDefinitions() {
			availableResources = append(availableResources, res.Name) // Collect all resource names.
		}
	}

	return nil, cgerr.NewResourceError(
		fmt.Sprintf("resource '%s' not found", name),
		nil,
		map[string]interface{}{
			"resource_name":       name,
			"available_resources": availableResources, // Include available resources in the error context.
		},
	)
}

// ReadResource reads a resource across all providers.
// It first finds the appropriate provider for the given resource name
// and then calls the provider's ReadResource method to retrieve the content.
// It also handles context timeouts and wraps errors with additional context.
func (rm *ResourceManager) ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) {
	provider, err := rm.FindResourceProvider(name) // Find the provider for the resource.
	if err != nil {
		return "", "", errors.Wrap(err, "failed to find resource provider") // Wrap error with context.
	}

	// Capture the start time for timing information
	startTime := time.Now()

	// Check for context cancellation or deadline
	if ctx.Err() != nil {
		return "", "", cgerr.NewTimeoutError(
			fmt.Sprintf("context ended before reading resource '%s'", name),
			map[string]interface{}{
				"resource_name": name,
				"context_error": ctx.Err().Error(), // Include context error in the timeout error.
			},
		)
	}

	content, mimeType, err := provider.ReadResource(ctx, name, args) // Read the resource from the provider.
	if err != nil {
		// Add more context to the error
		return "", "", cgerr.NewResourceError(
			fmt.Sprintf("failed to read resource '%s'", name),
			err,
			map[string]interface{}{
				"resource_name":  name,
				"args":           args,
				"operation_time": time.Since(startTime).String(), // Include operation time in the error context.
			},
		)
	}

	// Return the resource content
	return content, mimeType, nil
}
