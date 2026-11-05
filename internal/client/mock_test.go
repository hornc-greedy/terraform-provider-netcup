// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/mockapi"
)

func mockClient(t *testing.T) (*Client, *mockapi.Server) {
	t.Helper()
	m := mockapi.New(t)
	c := New(mockapi.CCPCustomer, mockapi.CCPKey, mockapi.CCPPassword)
	c.Endpoint = m.URL + mockapi.CCPPath + "?JSON"
	return c, m
}

func TestMockLogout(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	if err := c.Logout(ctx); err != nil || m.CCPLogouts() != 0 {
		t.Fatalf("Logout without a session: %v, %d logouts", err, m.CCPLogouts())
	}
	if _, err := c.InfoDNSZone(ctx, mockapi.CCPDomain); err != nil {
		t.Fatal(err)
	}
	if err := c.Logout(ctx); err != nil || m.CCPLogouts() != 1 {
		t.Fatalf("Logout: %v, %d logouts", err, m.CCPLogouts())
	}
}

func TestMockRecordsKeepOthersOnUpdate(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	first, err := c.CreateDNSRecord(ctx, mockapi.CCPDomain, DNSRecord{Hostname: "a", Type: "A", Destination: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.CreateDNSRecord(ctx, mockapi.CCPDomain, DNSRecord{Hostname: "b", Type: "A", Destination: "192.0.2.2"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || m.CCPRecordCount() != 2 {
		t.Fatalf("ids %s, %s, %d records", first.ID, second.ID, m.CCPRecordCount())
	}
	if err := c.DeleteDNSRecord(ctx, mockapi.CCPDomain, *first); err != nil {
		t.Fatal(err)
	}
	records, err := c.InfoDNSRecords(ctx, mockapi.CCPDomain)
	if err != nil || len(records) != 1 || records[0].ID != second.ID {
		t.Fatalf("records = %+v, %v", records, err)
	}
}

func TestMockUnknownDomainFails(t *testing.T) {
	c, _ := mockClient(t)
	_, err := c.InfoDNSRecords(context.Background(), "unknown.example")
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != 5029 {
		t.Fatalf("err = %v", err)
	}
}

func TestMockEmptyZoneHasNoRecords(t *testing.T) {
	c, _ := mockClient(t)
	records, err := c.InfoDNSRecords(context.Background(), mockapi.CCPDomain)
	if err != nil || len(records) != 0 {
		t.Fatalf("records = %+v, %v", records, err)
	}
}

func TestMockLogsInAgainAfterEndedSession(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	if _, err := c.InfoDNSZone(ctx, mockapi.CCPDomain); err != nil {
		t.Fatal(err)
	}
	m.EndCCPSession()
	if _, err := c.InfoDNSZone(ctx, mockapi.CCPDomain); err != nil {
		t.Fatalf("InfoDNSZone after the session ended: %v", err)
	}
}

func TestMockSessionEndsBeforeAWrite(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	if _, err := c.InfoDNSZone(ctx, mockapi.CCPDomain); err != nil {
		t.Fatal(err)
	}
	m.EndCCPSession()
	if _, err := c.CreateDNSRecord(ctx, mockapi.CCPDomain, DNSRecord{Hostname: "a", Type: "A", Destination: "192.0.2.1"}); err != nil {
		t.Fatal(err)
	}
	if got := m.CCPRecordCount(); got != 1 {
		t.Fatalf("records = %d, want 1", got)
	}
}

func TestMockSessionEndsForConcurrentCalls(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	if _, err := c.InfoDNSZone(ctx, mockapi.CCPDomain); err != nil {
		t.Fatal(err)
	}
	m.EndCCPSession()
	var calls sync.WaitGroup
	errs := make(chan error, 10)
	for range cap(errs) {
		calls.Go(func() {
			_, err := c.InfoDNSZone(ctx, mockapi.CCPDomain)
			errs <- err
		})
	}
	calls.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestMockCreateDoesNotTakeAnotherRecord(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	m.AddCCPRecordOnNextWrite("other", "A", "192.0.2.9")
	record, err := c.CreateDNSRecord(ctx, mockapi.CCPDomain, DNSRecord{Hostname: "a", Type: "A", Destination: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	if record.Hostname != "a" {
		t.Fatalf("record = %+v", record)
	}
}
