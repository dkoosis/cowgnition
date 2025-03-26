// internal/mcp/resource.go
package mcp

import (
	"context"
)

// ResourceProvider defines an interface for components that provide MCP resources.
type ResourceProvider interface {
	// GetResourceDefinitions returns the list of resources this provider handles.
	GetResourceDefinitions() []ResourceDefinition

	// ReadResource attempts to read the content of a resource with the given name and arguments.
	// Returns the resource content, MIME type, and any error encountered.
	ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error)
}

// ResourceManager manages all registered resource providers.
type ResourceManager struct {
	providers []ResourceProvider
}

// NewResourceManager creates a new resource manager.
func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		providers: []ResourceProvider{},
	}
}

// RegisterProvider registers a ResourceProvider.
func (rm *ResourceManager) RegisterProvider(provider ResourceProvider) {
	rm.providers = append(rm.providers, provider)
}

// GetAllResourceDefinitions returns all resource definitions from all providers.
func (rm *ResourceManager) GetAllResourceDefinitions() []ResourceDefinition {
	var allResources []ResourceDefinition
	for _, provider := range rm.providers {
		allResources = append(allResources, provider.GetResourceDefinitions()...)
	}
	return allResources
}

// FindResourceProvider finds the provider for a specific resource name.
func (rm *ResourceManager) FindResourceProvider(name string) (ResourceProvider, error) {
	for _, provider := range rm.providers {
		for _, res := range provider.GetResourceDefinitions() {
			if res.Name == name {
				return provider, nil
			}
		}
	}
	return nil, ErrResourceNotFound
}

// ReadResource reads a resource across all providers.
func (rm *ResourceManager) ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) {
	provider, err := rm.FindResourceProvider(name)
	if err != nil {
		return "", "", err
	}
	return provider.ReadResource(ctx, name, args)
}
