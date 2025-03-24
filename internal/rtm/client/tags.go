// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import (
	"context"
)

// GetTags gets all tags from the RTM API.
// This returns the raw XML response, which needs to be parsed by the caller.
func (c *Client) GetTags() ([]byte, error) {
	ctx := context.Background()
	return c.callMethod(ctx, "rtm.tags.getList", nil)
}
