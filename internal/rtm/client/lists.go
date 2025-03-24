// Package rtm provides client functionality for the Remember The Milk API.
package rtm

import (
	"context"
)

// GetLists gets all lists from the RTM API.
// This returns the raw XML response, which needs to be parsed by the caller.
func (c *Client) GetLists() ([]byte, error) {
	ctx := context.Background()
	return c.callMethod(ctx, "rtm.lists.getList", nil)
}
