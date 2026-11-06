// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"net/url"
	"strconv"
)

// ServerFirewall mirrors the ServerFirewall schema.
type ServerFirewall struct {
	CopiedPolicies      []FirewallPolicy `json:"copiedPolicies"`
	UserPolicies        []FirewallPolicy `json:"userPolicies"`
	IngressImplicitRule string           `json:"ingressImplicitRule"`
	EgressImplicitRule  string           `json:"egressImplicitRule"`
	Consistent          *bool            `json:"consistent"`
	Active              bool             `json:"active"`
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

// CheckServerFirewall is GetServerFirewall with consistencyCheck, which makes
// netcup set Consistent.
func (c *Client) CheckServerFirewall(ctx context.Context, serverID int, mac string) (*ServerFirewall, error) {
	var out ServerFirewall
	if err := c.do(ctx, "GET", sfwPath(serverID, mac)+"?consistencyCheck=true", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetServerFirewall assigns the given user-policy ids to a server interface and
// waits for the async task. active controls whether the firewall is enabled.
// The body replaces the copied policies, so they are removed unless keepCopied
// is set.
func (c *Client) SetServerFirewall(ctx context.Context, serverID int, mac string, userPolicyIDs []int, active, keepCopied bool) error {
	copied := []identifier{}
	if keepCopied {
		cur, err := c.GetServerFirewall(ctx, serverID, mac)
		if err != nil {
			return err
		}
		for _, p := range cur.CopiedPolicies {
			copied = append(copied, identifier{ID: p.ID})
		}
	}
	body := serverFirewallSave{
		CopiedPolicies: copied,
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
