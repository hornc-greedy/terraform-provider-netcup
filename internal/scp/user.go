// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// UserID returns the internal SCP user id (not the customer number), resolved
// once via /userinfo and cached. Many endpoints are scoped to /users/{userId}.
func (c *Client) UserID(ctx context.Context) (int, error) {
	c.mu.Lock()
	if c.userID != 0 {
		id := c.userID
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	// netcup returns id as a JSON string; json.Number accepts it.
	var parsed struct {
		ID json.Number `json:"id"`
	}
	if err := c.sendTo(ctx, http.MethodGet, c.UserinfoURL, "", nil, &parsed); err != nil {
		return 0, fmt.Errorf("userinfo: %w", err)
	}
	id, err := parsed.ID.Int64()
	if err != nil {
		return 0, fmt.Errorf("userinfo id %q is not an integer: %w", parsed.ID.String(), err)
	}

	c.mu.Lock()
	c.userID = int(id)
	c.mu.Unlock()
	return int(id), nil
}
