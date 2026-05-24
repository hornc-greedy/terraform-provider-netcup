// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"strconv"
)

// FailoverIPv4 mirrors the FailoverIPv4 schema (subset).
type FailoverIPv4 struct {
	ID         int    `json:"id"`
	IP         string `json:"ip"`
	CIDRSuffix int    `json:"cidrSuffix"`
	Editable   bool   `json:"editable"`
	Server     *struct {
		ID int `json:"id"`
	} `json:"server"`
}

// FailoverIPv6 mirrors the FailoverIPv6 schema (subset).
type FailoverIPv6 struct {
	ID                  int    `json:"id"`
	NetworkPrefix       string `json:"networkPrefix"`
	NetworkPrefixLength int    `json:"networkPrefixLength"`
	Editable            bool   `json:"editable"`
	Server              *struct {
		ID int `json:"id"`
	} `json:"server"`
}

func foBase(userID int, v6 bool) string {
	v := "v4"
	if v6 {
		v = "v6"
	}
	return "/users/" + strconv.Itoa(userID) + "/failoverips/" + v
}

// ListFailoverIPv4 returns all failover IPv4s of the user.
func (c *Client) ListFailoverIPv4(ctx context.Context) ([]FailoverIPv4, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	var out []FailoverIPv4
	if err := c.do(ctx, "GET", foBase(uid, false), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListFailoverIPv6 returns all failover IPv6s of the user.
func (c *Client) ListFailoverIPv6(ctx context.Context) ([]FailoverIPv6, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return nil, err
	}
	var out []FailoverIPv6
	if err := c.do(ctx, "GET", foBase(uid, true), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RouteFailover routes a failover IP (by its id) to a server and waits for the
// async task. Set v6=true for an IPv6 failover.
func (c *Client) RouteFailover(ctx context.Context, id, serverID int, v6 bool) error {
	uid, err := c.UserID(ctx)
	if err != nil {
		return err
	}
	body := map[string]int{"serverId": serverID}
	var task TaskInfo
	if err := c.do(ctx, "PATCH", foBase(uid, v6)+"/"+strconv.Itoa(id), body, &task); err != nil {
		return err
	}
	return c.waitForTask(ctx, task.UUID)
}
