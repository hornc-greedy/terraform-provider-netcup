// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"net/url"
	"strconv"
)

// ServerFirewall mirrors the firewall state of one server interface (subset).
type ServerFirewall struct {
	CopiedPolicies []FirewallPolicy `json:"copiedPolicies"`
	UserPolicies   []FirewallPolicy `json:"userPolicies"`
	Active         bool             `json:"active"`
}

// identifier is the IdentifierInt wrapper the save body expects.
type identifier struct {
	ID int `json:"id"`
}

// serverFirewallSave is the body of PUT .../firewall.
type serverFirewallSave struct {
	CopiedPolicies []identifier `json:"copiedPolicies"`
	UserPolicies   []identifier `json:"userPolicies"`
	Active         bool         `json:"active"`
}

func sfwPath(serverID int, mac string) string {
	return "/servers/" + strconv.Itoa(serverID) + "/interfaces/" + url.PathEscape(mac) + "/firewall"
}

// GetServerFirewall returns the firewall assignment of a server interface.
func (c *Client) GetServerFirewall(ctx context.Context, serverID int, mac string) (*ServerFirewall, error) {
	var out ServerFirewall
	if err := c.do(ctx, "GET", sfwPath(serverID, mac), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetServerFirewall assigns the given user-policy ids to a server interface and
// waits for the async task. active controls whether the firewall is enabled.
func (c *Client) SetServerFirewall(ctx context.Context, serverID int, mac string, userPolicyIDs []int, active bool) error {
	body := serverFirewallSave{
		CopiedPolicies: []identifier{},
		UserPolicies:   make([]identifier, 0, len(userPolicyIDs)),
		Active:         active,
	}
	for _, id := range userPolicyIDs {
		body.UserPolicies = append(body.UserPolicies, identifier{ID: id})
	}
	var task TaskInfo
	if err := c.do(ctx, "PUT", sfwPath(serverID, mac), body, &task); err != nil {
		return err
	}
	return c.waitForTask(ctx, task.UUID)
}
