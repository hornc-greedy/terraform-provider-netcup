// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
)

// InfoDNSRecords returns all records of a zone. Reads need no zone lock.
func (c *Client) InfoDNSRecords(ctx context.Context, domain string) ([]DNSRecord, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	raw, err := c.do(ctx, "infoDnsRecords", map[string]any{
		"domainname":     domain,
		"customernumber": c.CustomerNumber,
		"apikey":         c.APIKey,
		"apisessionid":   c.session(),
	})
	if err != nil {
		return nil, err
	}
	var set dnsRecordSet
	if err := json.Unmarshal(raw, &set); err != nil {
		return nil, fmt.Errorf("decode dns records: %w", err)
	}
	return set.DNSRecords, nil
}

// updateDNSRecordsRaw sends a delta to a zone and returns the full updated set.
// It does NOT take the zone lock; callers that need atomic read-modify-write
// (e.g. CreateDNSRecord) hold the lock around several calls themselves.
func (c *Client) updateDNSRecordsRaw(ctx context.Context, domain string, recs []DNSRecord) ([]DNSRecord, error) {
	if err := c.ensureSession(ctx); err != nil {
		return nil, err
	}
	raw, err := c.do(ctx, "updateDnsRecords", map[string]any{
		"domainname":     domain,
		"customernumber": c.CustomerNumber,
		"apikey":         c.APIKey,
		"apisessionid":   c.session(),
		"dnsrecordset":   dnsRecordSet{DNSRecords: recs},
	})
	if err != nil {
		return nil, err
	}
	var set dnsRecordSet
	if err := json.Unmarshal(raw, &set); err != nil {
		return nil, fmt.Errorf("decode dns records: %w", err)
	}
	return set.DNSRecords, nil
}

// UpdateDNSRecords sends a delta under the per-zone lock and returns the full
// updated set.
func (c *Client) UpdateDNSRecords(ctx context.Context, domain string, recs []DNSRecord) ([]DNSRecord, error) {
	unlock := c.zoneLock(domain)
	defer unlock()
	return c.updateDNSRecordsRaw(ctx, domain, recs)
}

// CreateDNSRecord adds a single record and resolves its server-assigned id.
// netcup's response returns the whole zone, so the new id is determined by
// snapshotting the ids before the write — the per-zone lock makes that atomic.
func (c *Client) CreateDNSRecord(ctx context.Context, domain string, rec DNSRecord) (*DNSRecord, error) {
	rec.ID = ""
	unlock := c.zoneLock(domain)
	defer unlock()

	before, err := c.InfoDNSRecords(ctx, domain)
	if err != nil {
		return nil, err
	}
	had := make(map[string]bool, len(before))
	for _, r := range before {
		had[r.ID] = true
	}

	after, err := c.updateDNSRecordsRaw(ctx, domain, []DNSRecord{rec})
	if err != nil {
		return nil, err
	}
	for i := range after {
		if !had[after[i].ID] {
			return &after[i], nil
		}
	}
	// Fallback: identical record already existed; match by attributes.
	for i := len(after) - 1; i >= 0; i-- {
		r := after[i]
		if r.Hostname == rec.Hostname && r.Type == rec.Type && r.Destination == rec.Destination {
			return &after[i], nil
		}
	}
	return nil, fmt.Errorf("created record %s/%s not found in update response", rec.Hostname, rec.Type)
}

// UpdateDNSRecord updates one existing record (matched by id) and returns it.
func (c *Client) UpdateDNSRecord(ctx context.Context, domain string, rec DNSRecord) (*DNSRecord, error) {
	after, err := c.UpdateDNSRecords(ctx, domain, []DNSRecord{rec})
	if err != nil {
		return nil, err
	}
	for i := range after {
		if after[i].ID == rec.ID {
			return &after[i], nil
		}
	}
	return nil, fmt.Errorf("record %s not found after update", rec.ID)
}

// DeleteDNSRecord removes one record. The full record is sent with
// deleterecord=true so netcup can match it unambiguously.
func (c *Client) DeleteDNSRecord(ctx context.Context, domain string, rec DNSRecord) error {
	rec.DeleteRecord = true
	_, err := c.UpdateDNSRecords(ctx, domain, []DNSRecord{rec})
	return err
}
