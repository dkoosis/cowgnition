// file: internal/mcp/resource.go
// Package mcp defines interfaces and structures for managing resources within the Model Context Protocol (MCP).
// This file implements the ResourceManager interface.
// Terminate all comments with a period.
package mcp

import (
	"context"
	"fmt"
	"time"

	// Using cockroachdb/errors for better context.
	"github.com/dkoosis/cowgnition/internal/logging"
	// Use the corrected definitions package.
	"github.com/dkoosis/cowgnition/internal/mcp/definitions"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
)

// Initialize the logger at the package level.
var resourceLogger = logging.GetLogger("mcp_resource")

// ResourceManagerImpl implements the ResourceManager interface.
type ResourceManagerImpl struct {
	providers []ResourceProvider // providers holds the registered ResourceProviders.
	// TODO: Consider adding an index (e.g., map[string]ResourceProvider) for faster provider lookup by URI.
}

// NewResourceManager creates a new resource manager.
// It initializes the ResourceManagerImpl with an empty list of providers.
func NewResourceManager() ResourceManager {
	resourceLogger.Debug("Initializing new resource manager.") // Added period.
	return &ResourceManagerImpl{
		providers: []ResourceProvider{}, // Initialize with no providers.
	}
}

// RegisterProvider registers a ResourceProvider.
// This adds a provider to the list of available providers,
// allowing the ResourceManager to access its resources.
func (rm *ResourceManagerImpl) RegisterProvider(provider ResourceProvider) {
	providerType := fmt.Sprintf("%T", provider)
	resourceLogger.Info("Registering resource provider.", "provider_type", providerType) // Added period.
	rm.providers = append(rm.providers, provider)                                        // Add the provider to the list.
}

// GetAllResourceDefinitions returns all resource definitions from all providers.
// This aggregates the definitions from each provider into a single list.
// Signature updated to return []definitions.Resource.
func (rm *ResourceManagerImpl) GetAllResourceDefinitions() []definitions.Resource {
	resourceLogger.Debug("Getting all resource definitions.") // Added period.
	// Corrected: Use the new definitions.Resource type.
	var allResources []definitions.Resource
	for _, provider := range rm.providers {
		// Assuming provider.GetResourceDefinitions signature matches updated ResourceProvider interface.
		defs := provider.GetResourceDefinitions()
		resourceLogger.Debug("Fetched definitions from provider.", "provider_type", fmt.Sprintf("%T", provider), "count", len(defs)) // Added period.
		allResources = append(allResources, defs...)                                                                                 // Collect definitions from each provider.
	}
	resourceLogger.Debug("Total resource definitions fetched.", "count", len(allResources)) // Added period.
	return allResources
}

// FindResourceProvider finds the provider for a specific resource URI.
// It iterates through the registered providers and their definitions
// to locate the one whose resource definition URI matches the requested URI.
// NOTE: This currently only supports exact URI matches. A more advanced implementation
// would be needed to handle URI templates (RFC 6570).
// Signature updated to accept uri string.
func (rm *ResourceManagerImpl) FindResourceProvider(uri string) (ResourceProvider, error) {
	resourceLogger.Debug("Finding resource provider.", "resource_uri", uri) // Changed log field.
	for _, provider := range rm.providers {
		providerType := fmt.Sprintf("%T", provider)
		// Assuming provider.GetResourceDefinitions signature matches updated ResourceProvider interface.
		for _, res := range provider.GetResourceDefinitions() {
			// Match based on the URI field of the resource definition.
			if res.URI == uri {
				resourceLogger.Debug("Found provider for resource.", "resource_uri", uri, "provider_type", providerType) // Changed log field.
				return provider, nil                                                                                     // Return the provider if found.
			}
			// TODO: Add logic here to handle matching against ResourceTemplates (res.URITemplate) if needed.
		}
	}

	// Get all available resource URIs for better error context.
	var availableResources []string
	for _, provider := range rm.providers {
		// Assuming provider.GetResourceDefinitions signature matches updated ResourceProvider interface.
		for _, res := range provider.GetResourceDefinitions() {
			availableResources = append(availableResources, res.URI) // Collect resource URIs.
		}
		// TODO: Add logic to list available ResourceTemplates as well.
	}
	resourceLogger.Warn("Resource provider not found.", "resource_uri", uri, "available_count", len(availableResources)) // Changed log field.

	// Return a specific error indicating the resource URI was not found.
	return nil, cgerr.NewResourceError(
		fmt.Sprintf("resource URI '%s' not found.", uri), // Updated message.
		nil, // No underlying error to wrap.
		map[string]interface{}{
			"resource_uri":        uri, // Updated property name.
			"available_resources": availableResources,
		},
	)
}

// ReadResource reads a resource across all providers using its URI.
// It first finds the appropriate provider for the given resource URI
// and then calls the provider's ReadResource method.
// It handles context timeouts and wraps errors.
// Signature updated to accept uri string and return definitions.ReadResourceResult.
func (rm *ResourceManagerImpl) ReadResource(ctx context.Context, uri string) (definitions.ReadResourceResult, error) {
	emptyResult := definitions.ReadResourceResult{}               // Helper for error returns.
	resourceLogger.Info("Reading resource.", "resource_uri", uri) // Changed log field.

	// Find the provider based on the resource URI.
	provider, err := rm.FindResourceProvider(uri)
	if err != nil {
		// Log the detailed error from FindResourceProvider.
		resourceLogger.Error("Failed to find resource provider.",
			"resource_uri", uri, // Changed log field.
			"error", fmt.Sprintf("%+v", err),
		)
		// Return the original detailed error.
		return emptyResult, err
	}

	// Capture the start time for timing information.
	startTime := time.Now()
	providerType := fmt.Sprintf("%T", provider)
	resourceLogger.Debug("Calling ReadResource on provider.", "resource_uri", uri, "provider_type", providerType) // Changed log field.

	// Check for context cancellation or deadline *before* calling the provider.
	if ctxErr := ctx.Err(); ctxErr != nil {
		timeoutErr := cgerr.NewTimeoutError(
			fmt.Sprintf("ResourceManagerImpl.ReadResource: context ended before reading resource URI '%s'.", uri), // Updated message.
			map[string]interface{}{
				"resource_uri":  uri, // Updated property name.
				"context_error": ctxErr.Error(),
			},
		)
		resourceLogger.Error("Context error before reading resource.",
			"resource_uri", uri, // Changed log field.
			"error", fmt.Sprintf("%+v", timeoutErr),
		)
		return emptyResult, timeoutErr
	}

	// Call the provider's ReadResource method using the updated signature.
	// It now returns ReadResourceResult, error.
	// Assuming provider.ReadResource signature matches updated ResourceProvider interface.
	result, err := provider.ReadResource(ctx, uri)
	readDuration := time.Since(startTime)

	if err != nil {
		// Log the error from provider.ReadResource.
		resourceLogger.Error("Provider failed to read resource.",
			"resource_uri", uri, // Changed log field.
			"provider_type", providerType,
			"duration_ms", readDuration.Milliseconds(),
			"error", fmt.Sprintf("%+v", err),
		)

		// Wrap the provider error with context. The handler will convert this to JSON-RPC error.
		return emptyResult, cgerr.NewResourceError(
			fmt.Sprintf("ResourceManagerImpl.ReadResource: failed to read resource URI '%s' from provider %s.", uri, providerType), // Updated message.
			err, // Keep the original wrapped error.
			map[string]interface{}{
				"resource_uri":   uri, // Updated property name.
				"provider_type":  providerType,
				"operation_time": readDuration.String(),
			},
		)
	}

	// Log success. The result struct contains content details.
	contentDesc := fmt.Sprintf("%d content block(s)", len(result.Contents))
	if len(result.Contents) == 1 {
		if result.Contents[0].Text != nil {
			contentDesc = fmt.Sprintf("1 text block (len:%d)", len(*result.Contents[0].Text))
		} else if result.Contents[0].Blob != nil {
			contentDesc = fmt.Sprintf("1 blob block (len:%d)", len(*result.Contents[0].Blob))
		}
	}

	resourceLogger.Info("Successfully read resource.",
		"resource_uri", uri, // Changed log field.
		"provider_type", providerType,
		"content_summary", contentDesc, // Provide summary instead of full content.
		"duration_ms", readDuration.Milliseconds(),
	)
	// Return the successful result structure.
	return result, nil
}
