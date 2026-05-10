// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"net/url"
	"strings"
)

// IsIPv6 reports whether ip is an IPv6 address (used to pick the v4/v6 path).
func IsIPv6(ip string) bool {
	return strings.Contains(ip, ":")
}

func rdnsPath(ip string) string {
	if IsIPv6(ip) {
		return "/rdns/ipv6/" + url.PathEscape(ip)
	}
	return "/rdns/ipv4/" + url.PathEscape(ip)
}

func rdnsCollection(ip string) string {
	if IsIPv6(ip) {
		return "/rdns/ipv6"
	}
	return "/rdns/ipv4"
}

// GetReverseDNS returns the rDNS hostname set for ip, or "" if none is set.
func (c *Client) GetReverseDNS(ctx context.Context, ip string) (string, error) {
	var out rdnsResponse
	if err := c.do(ctx, "GET", rdnsPath(ip), nil, &out); err != nil {
		return "", err
	}
	return out.RDNS, nil
}

// SetReverseDNS sets the rDNS hostname for ip. The endpoint is synchronous.
func (c *Client) SetReverseDNS(ctx context.Context, ip, hostname string) error {
	return c.do(ctx, "POST", rdnsCollection(ip), setRdns{IP: ip, RDNS: hostname}, nil)
}

// DeleteReverseDNS removes the rDNS entry for ip.
func (c *Client) DeleteReverseDNS(ctx context.Context, ip string) error {
	return c.do(ctx, "DELETE", rdnsPath(ip), nil, nil)
}
