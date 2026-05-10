// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"strconv"
)

// ListServers returns all servers of the account.
func (c *Client) ListServers(ctx context.Context) ([]Server, error) {
	var out []Server
	if err := c.do(ctx, "GET", "/servers", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetServer returns one server by id, including live info fields.
func (c *Client) GetServer(ctx context.Context, id int) (*Server, error) {
	var out Server
	if err := c.do(ctx, "GET", "/servers/"+strconv.Itoa(id)+"?loadServerLiveInfo=true", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetInterfaces returns all network interfaces of a server, including IPs and
// rDNS entries.
func (c *Client) GetInterfaces(ctx context.Context, serverID int) ([]Interface, error) {
	var out []Interface
	if err := c.do(ctx, "GET", "/servers/"+strconv.Itoa(serverID)+"/interfaces?loadRdns=true", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
