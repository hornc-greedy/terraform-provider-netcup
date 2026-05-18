// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"strconv"
)

// FirewallRule mirrors one rule of a firewall policy. The API uses plural field
// names (sources/destinations, sourcePorts/destinationPorts) even for a single
// value. Valid enum values:
//   - action:    ACCEPT | DROP
//   - protocol:  TCP | UDP | ICMP | ICMPv6
//   - direction: INGRESS | EGRESS
type FirewallRule struct {
	Description      string   `json:"description,omitempty"`
	Direction        string   `json:"direction"`
	Protocol         string   `json:"protocol"`
	Action           string   `json:"action"`
	Sources          []string `json:"sources,omitempty"`
	SourcePorts      string   `json:"sourcePorts,omitempty"`
	Destinations     []string `json:"destinations,omitempty"`
	DestinationPorts string   `json:"destinationPorts,omitempty"`
}

// FirewallPolicy mirrors the FirewallPolicy schema. On create the API consumes
// the FirewallPolicySave shape (name, description, rules); id is omitted then
// and populated on read.
type FirewallPolicy struct {
	ID          int            `json:"id,omitempty"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Rules       []FirewallRule `json:"rules,omitempty"`
}

func (c *Client) fwBase(ctx context.Context) (string, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return "", err
	}
	return "/users/" + strconv.Itoa(uid) + "/firewall-policies", nil
}

// ListFirewallPolicies returns all firewall policies of the user.
func (c *Client) ListFirewallPolicies(ctx context.Context) ([]FirewallPolicy, error) {
	base, err := c.fwBase(ctx)
	if err != nil {
		return nil, err
	}
	var out []FirewallPolicy
	if err := c.do(ctx, "GET", base, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetFirewallPolicy returns one policy by id.
func (c *Client) GetFirewallPolicy(ctx context.Context, id int) (*FirewallPolicy, error) {
	base, err := c.fwBase(ctx)
	if err != nil {
		return nil, err
	}
	var out FirewallPolicy
	if err := c.do(ctx, "GET", base+"/"+strconv.Itoa(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateFirewallPolicy creates a policy and returns it with its id.
func (c *Client) CreateFirewallPolicy(ctx context.Context, p FirewallPolicy) (*FirewallPolicy, error) {
	base, err := c.fwBase(ctx)
	if err != nil {
		return nil, err
	}
	var out FirewallPolicy
	if err := c.do(ctx, "POST", base, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateFirewallPolicy updates a policy in place and returns the fresh state.
func (c *Client) UpdateFirewallPolicy(ctx context.Context, id int, p FirewallPolicy) (*FirewallPolicy, error) {
	base, err := c.fwBase(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.do(ctx, "PUT", base+"/"+strconv.Itoa(id), p, nil); err != nil {
		return nil, err
	}
	return c.GetFirewallPolicy(ctx, id)
}

// DeleteFirewallPolicy removes a policy by id.
func (c *Client) DeleteFirewallPolicy(ctx context.Context, id int) error {
	base, err := c.fwBase(ctx)
	if err != nil {
		return err
	}
	return c.do(ctx, "DELETE", base+"/"+strconv.Itoa(id), nil, nil)
}
