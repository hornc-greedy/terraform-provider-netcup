// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// DefaultUserinfoURL is the OIDC userinfo endpoint of the scp realm. It lives on
// the realm, not under the API base, so it is requested separately.
const DefaultUserinfoURL = "https://www.servercontrolpanel.de/realms/scp/protocol/openid-connect/userinfo"

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

	token, err := c.ensureToken(ctx)
	if err != nil {
		return 0, err
	}
	url := c.UserinfoURL
	if url == "" {
		url = DefaultUserinfoURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("userinfo failed (http %d)", res.StatusCode)
	}
	// netcup returns id as a JSON string; accept string or number defensively.
	var parsed struct {
		ID json.Number `json:"id"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, fmt.Errorf("decode userinfo: %w", err)
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
