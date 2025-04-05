// Package mcp defines interfaces and structures for managing resources within the Model Context Protocol (MCP).
// file: internal/mcp/resource.go
package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/dkoosis/cowgnition/internal/logging"
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// Initialize the logger at the package level.
var resourceLogger = logging.GetLogger("mcp_resource")

// ResourceManagerImpl implements the ResourceManager interface.
type ResourceManagerImpl struct {
	providers []ResourceProvider // providers holds the registered ResourceProviders.
}

// NewResourceManager creates a new resource manager.
// It initializes the ResourceManagerImpl with an empty list of providers.
func NewResourceManager() ResourceManager {
	resourceLogger.Debug("Initializing new resource manager")
	return &ResourceManagerImpl{
		providers: []ResourceProvider{}, // Initialize with no providers.
	}
}

// RegisterProvider registers a ResourceProvider.
// This adds a provider to the list of available providers,
// allowing the ResourceManager to access its resources.
func (rm *ResourceManagerImpl) RegisterProvider(provider ResourceProvider) {
	providerType := fmt.Sprintf("%T", provider)
	resourceLogger.Info("Registering resource provider", "provider_type", providerType)
	rm.providers = append(rm.providers, provider) // Add the provider to the list.
}

// GetAllResourceDefinitions returns all resource definitions from all providers.
// This aggregates the definitions from each provider into a single list,
// providing a comprehensive view of available resources.
func (rm *ResourceManagerImpl) GetAllResourceDefinitions() []definitions.ResourceDefinition {
	resourceLogger.Debug("Getting all resource definitions")
	var allResources []definitions.ResourceDefinition
	for _, provider := range rm.providers {
		defs := provider.GetResourceDefinitions()
		resourceLogger.Debug("Fetched definitions from provider", "provider_type", fmt.Sprintf("%T", provider), "count", len(defs))
		allResources = append(allResources, defs...) // Collect definitions from each provider.
	}
	resourceLogger.Debug("Total resource definitions fetched", "count", len(allResources))
	return allResources
}

// FindResourceProvider finds the provider for a specific resource name.
// It iterates through the registered providers to locate the one
// that handles the resource with the given name.
// If no provider is found, it returns an error with a list of available resources
// to aid in debugging.
func (rm *ResourceManagerImpl) FindResourceProvider(name string) (ResourceProvider, error) {
	resourceLogger.Debug("Finding resource provider", "resource_name", name)
	for _, provider := range rm.providers {
		providerType := fmt.Sprintf("%T", provider)
		for _, res := range provider.GetResourceDefinitions() {
			if res.Name == name {
				resourceLogger.Debug("Found provider for resource", "resource_name", name, "provider_type", providerType)
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
	resourceLogger.Warn("Resource provider not found", "resource_name", name, "available_count", len(availableResources))

	// This already returns a detailed cgerr type, which is good.
	return nil, cgerr.NewResourceError(
		fmt.Sprintf("resource '%s' not found", name),
		nil, // No underlying error to wrap
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
func (rm *ResourceManagerImpl) ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) {
	resourceLogger.Info("Reading resource", "resource_name", name, "args", args)
	provider, err := rm.FindResourceProvider(name) // Find the provider for the resource.
	if err != nil {
		// Log the error from FindResourceProvider before returning it (as per assessment intent for L67)
		// The error from FindResourceProvider is already a detailed cgerr.NewResourceError.
		resourceLogger.Error("Failed to find resource provider",
			"resource_name", name,
			"args", args,
			"error", fmt.Sprintf("%+v", err), // Log with full details
		)
		// Return the original detailed error from FindResourceProvider.
		// No need to Wrapf here as the original error is already informative.
		return "", "", err
	}

	// Capture the start time for timing information
	startTime := time.Now()
	providerType := fmt.Sprintf("%T", provider)
	resourceLogger.Debug("Calling ReadResource on provider", "resource_name", name, "provider_type", providerType)

	// Check for context cancellation or deadline *before* calling the provider
	if ctxErr := ctx.Err(); ctxErr != nil {
		timeoutErr := cgerr.NewTimeoutError(
			fmt.Sprintf("ResourceManagerImpl.ReadResource: context ended before reading resource '%s'", name),
			map[string]interface{}{
				"resource_name": name,
				"args":          args,
				"context_error": ctxErr.Error(), // Include context error in the timeout error.
			},
		)
		resourceLogger.Error("Context error before reading resource",
			"resource_name", name,
			"args", args,
			"error", fmt.Sprintf("%+v", timeoutErr),
		)
		return "", "", timeoutErr
	}

	content, mimeType, err := provider.ReadResource(ctx, name, args) // Read the resource from the provider.
	readDuration := time.Since(startTime)
	if err != nil {
		// Log the error from provider.ReadResource before returning (as per assessment intent for L117)
		resourceLogger.Error("Provider failed to read resource",
			"resource_name", name,
			"provider_type", providerType,
			"args", args,
			"duration_ms", readDuration.Milliseconds(),
			"error", fmt.Sprintf("%+v", err), // Log with full details
		)

		// The existing cgerr usage is good, just ensure message includes function context.
		return "", "", cgerr.NewResourceError(
			fmt.Sprintf("ResourceManagerImpl.ReadResource: failed to read resource '%s' from provider %s", name, providerType), // Added function context
			err, // Keep the original wrapped error
			map[string]interface{}{
				"resource_name":  name,
				"provider_type":  providerType,
				"args":           args,
				"operation_time": readDuration.String(), // Include operation time in the error context.
			},
		)
	}

	resourceLogger.Info("Successfully read resource",
		"resource_name", name,
		"provider_type", providerType,
		"mime_type", mimeType,
		"content_length", len(content),
		"duration_ms", readDuration.Milliseconds(),
	)
	// Return the resource content
	return content, mimeType, nil
}
