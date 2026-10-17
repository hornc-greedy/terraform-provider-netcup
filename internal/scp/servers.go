// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"fmt"
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

// GetServerLive is GetServer for callers that need LiveInfo. It fails when
// netcup cannot report the live info.
func (c *Client) GetServerLive(ctx context.Context, id int) (*Server, error) {
	s, err := c.GetServer(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.LiveInfo == nil {
		return nil, fmt.Errorf("netcup reports no live info for server %d", id)
	}
	return s, nil
}

// PatchServer applies one server attribute, e.g. {"nickname": "web"}, and waits
// for the task when netcup runs the change asynchronously.
func (c *Client) PatchServer(ctx context.Context, id int, patch any) error {
	var task TaskInfo
	if err := c.send(ctx, "PATCH", "/servers/"+strconv.Itoa(id), "application/merge-patch+json", patch, &task); err != nil {
		return err
	}
	return c.waitForTask(ctx, task.UUID)
}
