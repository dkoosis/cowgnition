// internal/mcp/resource.go
package mcp

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"
	cgerr "github.com/dkoosis/cowgnition/internal/mcp/errors"
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

	return nil, cgerr.NewResourceError(
		fmt.Sprintf("resource '%s' not found", name),
		nil,
		map[string]interface{}{"resource_name": name},
	)
}

// ReadResource reads a resource across all providers.
func (rm *ResourceManager) ReadResource(ctx context.Context, name string, args map[string]string) (string, string, error) {
	provider, err := rm.FindResourceProvider(name)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to find resource provider")
	}

	content, mimeType, err := provider.ReadResource(ctx, name, args)
	if err != nil {
		return "", "", cgerr.NewResourceError(
			fmt.Sprintf("failed to read resource '%s'", name),
			err,
			map[string]interface{}{
				"resource_name": name,
				"args":          args,
			},
		)
	}

	return content, mimeType, nil
}
