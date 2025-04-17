// Package rtm implements the client and service logic for interacting with the Remember The Milk API.
package rtm

// file: internal/rtm/mcp_resources.go

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url" // Ensure url is imported
	"strings" // Ensure strings is imported

	"github.com/cockroachdb/errors"
	"github.com/dkoosis/cowgnition/internal/mcp"
	mcperrors "github.com/dkoosis/cowgnition/internal/mcp/mcp_errors"
)

// GetResources returns the MCP resources provided by this service.
func (s *Service) GetResources() []mcp.Resource {
	return []mcp.Resource{
		{
			Name:        "RTM Authentication Status",
			URI:         "rtm://auth",
			Description: "Provides the current authentication status with Remember The Milk (RTM).",
			MimeType:    "application/json",
		},
		{
			Name:        "RTM Lists",
			URI:         "rtm://lists",
			Description: "Lists available in your Remember The Milk account.",
			MimeType:    "application/json",
		},
		{
			Name:        "RTM Tags",
			URI:         "rtm://tags",
			Description: "Tags used in your Remember The Milk account.",
			MimeType:    "application/json",
		},
		{
			Name:        "RTM Tasks (Default Filter)",
			URI:         "rtm://tasks",
			Description: "Tasks in your Remember The Milk account (default view). Use rtm://tasks?filter=... for specific filters.",
			MimeType:    "application/json",
		},
		// Consider adding a ResourceTemplate if complex filtering is common
	}
}

// ReadResource handles MCP resource read requests for this service.
func (s *Service) ReadResource(ctx context.Context, uri string) ([]interface{}, error) {
	if !s.initialized {
		// Return internal error, not MCP error result, as this indicates a server state issue
		return nil, errors.New("RTM service is not initialized")
	}

	// Route based on URI
	switch {
	case uri == "rtm://auth":
		return s.readAuthResource(ctx)
	case uri == "rtm://lists":
		return s.readListsResource(ctx)
	case uri == "rtm://tags":
		return s.readTagsResource(ctx)
	case uri == "rtm://tasks":
		return s.readTasksResourceWithFilter(ctx, "") // No filter
	case strings.HasPrefix(uri, "rtm://tasks?"):
		filter, err := extractFilterFromURI(uri)
		if err != nil {
			// Return internal parsing error
			return nil, errors.Wrapf(err, "failed to parse filter from tasks URI: %s", uri)
		}
		return s.readTasksResourceWithFilter(ctx, filter)
	default:
		// Return MCP resource not found error
		return nil, mcperrors.NewResourceError(
			fmt.Sprintf("Unknown RTM resource URI: %s", uri),
			nil,
			map[string]interface{}{"uri": uri})
	}
}

// --- Resource Readers ---

func (s *Service) readAuthResource(ctx context.Context) ([]interface{}, error) {
	authState, err := s.GetAuthState(ctx) // Use service method
	if err != nil {
		// Return internal error from GetAuthState
		return nil, errors.Wrap(err, "failed to get auth state for resource")
	}
	return s.createJSONResourceContent("rtm://auth", authState) // Use helper
}

func (s *Service) readListsResource(ctx context.Context) ([]interface{}, error) {
	if !s.IsAuthenticated() {
		return s.notAuthenticatedResourceContent("rtm://lists"), nil // Return specific content, not error
	}
	lists, err := s.client.GetLists(ctx) // Assumes client.GetLists is correct
	if err != nil {
		// Return internal error from GetLists
		return nil, errors.Wrap(err, "failed to get lists for resource")
	}
	return s.createJSONResourceContent("rtm://lists", lists) // Use helper
}

func (s *Service) readTagsResource(ctx context.Context) ([]interface{}, error) {
	if !s.IsAuthenticated() {
		return s.notAuthenticatedResourceContent("rtm://tags"), nil // Return specific content, not error
	}
	tags, err := s.client.GetTags(ctx) // Assumes client.GetTags is correct
	if err != nil {
		// Return internal error from GetTags
		return nil, errors.Wrap(err, "failed to get tags for resource")
	}
	return s.createJSONResourceContent("rtm://tags", tags) // Use helper
}

func (s *Service) readTasksResourceWithFilter(ctx context.Context, filter string) ([]interface{}, error) {
	resourceURI := "rtm://tasks"
	if filter != "" {
		resourceURI = fmt.Sprintf("rtm://tasks?filter=%s", url.QueryEscape(filter))
	}

	if !s.IsAuthenticated() {
		return s.notAuthenticatedResourceContent(resourceURI), nil // Return specific content, not error
	}
	tasks, err := s.client.GetTasks(ctx, filter) // Assumes client.GetTasks is correct
	if err != nil {
		// Return internal error from GetTasks
		return nil, errors.Wrapf(err, "failed to get tasks for resource (filter: '%s')", filter)
	}
	return s.createJSONResourceContent(resourceURI, tasks) // Use helper
}

// --- Resource Helpers ---

// extractFilterFromURI parses the 'filter' query parameter.
func extractFilterFromURI(uriString string) (string, error) {
	parsedURL, err := url.Parse(uriString)
	if err != nil {
		return "", errors.Wrapf(err, "invalid URI format: %s", uriString)
	}
	// Return URL-decoded filter value
	return parsedURL.Query().Get("filter"), nil
}

// createJSONResourceContent marshals data and wraps it in TextResourceContents.
func (s *Service) createJSONResourceContent(uri string, data interface{}) ([]interface{}, error) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		// Return internal marshalling error
		return nil, errors.Wrapf(err, "failed to marshal resource data for URI: %s", uri)
	}
	return []interface{}{
		mcp.TextResourceContents{
			ResourceContents: mcp.ResourceContents{
				URI:      uri,
				MimeType: "application/json",
			},
			Text: string(jsonData),
		},
	}, nil
}
