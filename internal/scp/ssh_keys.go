// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"strconv"
	"time"
)

// normalizeTimestamp canonicalises an RFC3339 timestamp to microsecond
// precision. The create endpoint returns nanoseconds while the list endpoint
// rounds to microseconds; normalising both keeps create, read and import in
// sync. Unparseable input is returned unchanged.
func normalizeTimestamp(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.Round(time.Microsecond).Format(time.RFC3339Nano)
}

// SSHKey is an SSH public key stored in the SCP account.
type SSHKey struct {
	ID        int    `json:"id,omitempty"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// ListSSHKeys returns all SSH keys of the authenticated user.
func (c *Client) ListSSHKeys(ctx context.Context) ([]SSHKey, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	var out []SSHKey
	if err := c.do(ctx, "GET", "/users/"+strconv.Itoa(uid)+"/ssh-keys", nil, &out); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].CreatedAt = normalizeTimestamp(out[i].CreatedAt)
	}
	return out, nil
}

// GetSSHKey returns one SSH key by id, or nil if it no longer exists.
func (c *Client) GetSSHKey(ctx context.Context, id int) (*SSHKey, error) {
	keys, err := c.ListSSHKeys(ctx)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if keys[i].ID == id {
			return &keys[i], nil
		}
	}
	return nil, nil
}

// CreateSSHKey creates an SSH key and returns it with its server-assigned id.
func (c *Client) CreateSSHKey(ctx context.Context, name, key string) (*SSHKey, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	var out SSHKey
	if err := c.do(ctx, "POST", "/users/"+strconv.Itoa(uid)+"/ssh-keys", SSHKey{Name: name, Key: key}, &out); err != nil {
		return nil, err
	}
	out.CreatedAt = normalizeTimestamp(out.CreatedAt)
	return &out, nil
}

// DeleteSSHKey removes an SSH key by id.
func (c *Client) DeleteSSHKey(ctx context.Context, id int) error {
	uid, err := c.UserID(ctx)
	if err != nil {
		return err
	}
	return c.do(ctx, "DELETE", "/users/"+strconv.Itoa(uid)+"/ssh-keys/"+strconv.Itoa(id), nil, nil)
}
