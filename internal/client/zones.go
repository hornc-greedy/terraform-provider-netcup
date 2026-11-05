// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
)

// DNSZone mirrors the zone-level (SOA) settings of a netcup zone. netcup
// returns the numeric fields as strings.
type DNSZone struct {
	Name         string `json:"name"`
	TTL          string `json:"ttl"`
	Serial       string `json:"serial"`
	Refresh      string `json:"refresh"`
	Retry        string `json:"retry"`
	Expire       string `json:"expire"`
	DNSSECStatus bool   `json:"dnssecstatus"`
}

// InfoDNSZone returns the zone settings. Reads need no zone lock.
func (c *Client) InfoDNSZone(ctx context.Context, domain string) (*DNSZone, error) {
	raw, err := c.sessionCall(ctx, "infoDnsZone", map[string]any{"domainname": domain})
	if err != nil {
		return nil, err
	}
	var z DNSZone
	if err := json.Unmarshal(raw, &z); err != nil {
		return nil, fmt.Errorf("decode dns zone: %w", err)
	}
	return &z, nil
}

// UpdateDNSZone writes the zone settings under the per-zone lock and returns the
// updated zone.
func (c *Client) UpdateDNSZone(ctx context.Context, domain string, zone DNSZone) (*DNSZone, error) {
	unlock := c.zoneLock(domain)
	defer unlock()
	raw, err := c.sessionCall(ctx, "updateDnsZone", map[string]any{"domainname": domain, "dnszone": zone})
	if err != nil {
		return nil, err
	}
	var z DNSZone
	if err := json.Unmarshal(raw, &z); err != nil {
		return nil, fmt.Errorf("decode dns zone: %w", err)
	}
	return &z, nil
}
