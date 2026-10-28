// SPDX-License-Identifier: MPL-2.0

package clouddns_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/clouddns"
	"github.com/hornc-greedy/terraform-provider-netcup/internal/mockapi"
)

func mockClient(t *testing.T) (*clouddns.Client, *mockapi.Server) {
	t.Helper()
	m := mockapi.New(t)
	c := clouddns.New(m.URL, mockapi.APIKey)
	c.RetryDelay = time.Millisecond
	return c, m
}

const challengeValue = "abcdefghijklmnopqrstuvwxyz0123456789_-ABCDE"

func TestRecordTypes(t *testing.T) {
	c, _ := mockClient(t)
	types, err := c.RecordTypes(context.Background())
	if err != nil || len(types) == 0 {
		t.Fatalf("RecordTypes: %v %v", types, err)
	}
}

func TestDomains(t *testing.T) {
	c, _ := mockClient(t)
	ctx := context.Background()
	domains, err := c.Domains(ctx)
	if err != nil || len(domains) != 2 {
		t.Fatalf("Domains: %v %v", domains, err)
	}
	domain, err := c.DomainByFQDN(ctx, mockapi.CloudDNSDomain)
	if err != nil || domain == nil || domain.ID != mockapi.CloudDNSDomainID || !domain.IsDNSManaged {
		t.Fatalf("DomainByFQDN: %+v %v", domain, err)
	}
	missing, err := c.DomainByFQDN(ctx, "missing.example")
	if err != nil || missing != nil {
		t.Fatalf("missing domain: %+v %v", missing, err)
	}
}

func TestChangeRecordsCreatesRevision(t *testing.T) {
	c, _ := mockClient(t)
	ctx := context.Background()
	before, err := c.ActiveRevision(ctx, mockapi.CloudDNSDomainID)
	if err != nil {
		t.Fatalf("ActiveRevision: %v", err)
	}
	www := clouddns.RecordInput{Name: "www", Type: "A", RData: "192.0.2.1", TTL: 300}
	if err := c.ChangeRecords(ctx, mockapi.CloudDNSDomainID, []clouddns.RecordInput{www}, nil); err != nil {
		t.Fatalf("ChangeRecords: %v", err)
	}
	after, err := c.ActiveRevision(ctx, mockapi.CloudDNSDomainID)
	if err != nil || after.Identifier == before.Identifier {
		t.Fatalf("revision unchanged: %+v %v", after, err)
	}
	records, err := c.Records(ctx, mockapi.CloudDNSDomainID, after.Identifier)
	if err != nil || len(records) != 2 {
		t.Fatalf("Records: %+v %v", records, err)
	}
	record, err := c.RecordByIdentifier(ctx, mockapi.CloudDNSDomainID, after.Identifier, records[1].Identifier)
	if err != nil || record.Name != "www" || *record.TTL != 300 {
		t.Fatalf("RecordByIdentifier: %+v %v", record, err)
	}
	revisions, err := c.Revisions(ctx, mockapi.CloudDNSDomainID)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("Revisions: %+v %v", revisions, err)
	}
	if _, err := c.Revision(ctx, mockapi.CloudDNSDomainID, before.Identifier); err != nil {
		t.Fatalf("Revision: %v", err)
	}
}

func TestChangeRecordsIsStrict(t *testing.T) {
	c, _ := mockClient(t)
	ctx := context.Background()
	missing := clouddns.RecordInput{Name: "gone", Type: "A", RData: "192.0.2.1", TTL: 300}
	err := c.ChangeRecords(ctx, mockapi.CloudDNSDomainID, nil, []clouddns.RecordInput{missing})
	var apiErr *clouddns.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnprocessableEntity {
		t.Fatalf("err = %v, want 422", err)
	}
}

func TestLegacyDomainIsReported(t *testing.T) {
	c, _ := mockClient(t)
	_, err := c.ActiveRevision(context.Background(), mockapi.LegacyDomainID)
	if err == nil || !strings.Contains(err.Error(), "does not run on CloudDNS") || !strings.Contains(err.Error(), "No result was found.") {
		t.Fatalf("err = %v", err)
	}
}

func TestChallenges(t *testing.T) {
	c, _ := mockClient(t)
	ctx := context.Background()
	created, err := c.CreateChallenge(ctx, mockapi.CloudDNSDomainID, "www", challengeValue)
	if err != nil || created.FQDN == "" {
		t.Fatalf("CreateChallenge: %+v %v", created, err)
	}
	got, err := c.Challenge(ctx, mockapi.CloudDNSDomainID, "www", challengeValue)
	if err != nil || got == nil {
		t.Fatalf("Challenge: %+v %v", got, err)
	}
	all, err := c.Challenges(ctx, mockapi.CloudDNSDomainID)
	if err != nil || len(all) != 1 {
		t.Fatalf("Challenges: %+v %v", all, err)
	}
	if err := c.DeleteChallenge(ctx, mockapi.CloudDNSDomainID, "www", challengeValue); err != nil {
		t.Fatalf("DeleteChallenge: %v", err)
	}
	gone, err := c.Challenge(ctx, mockapi.CloudDNSDomainID, "www", challengeValue)
	if err != nil || gone != nil {
		t.Fatalf("Challenge after delete: %+v %v", gone, err)
	}
}

func TestListStopsAtLastPageDespiteNextPage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"messages":[],"meta":{"pagination":{"page":1,"lastPage":1,"nextPage":1}},"result":[{"id":1}]}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := clouddns.New(srv.URL, "k").Domains(context.Background()); err != nil {
		t.Fatalf("Domains: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestListReadsEveryPage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("page") == "" {
			_, _ = w.Write([]byte(`{"meta":{"pagination":{"page":1,"lastPage":2}},"result":[{"id":1}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"meta":{"pagination":{"page":2,"lastPage":2}},"result":[{"id":2}]}`))
	}))
	t.Cleanup(srv.Close)
	domains, err := clouddns.New(srv.URL, "k").Domains(context.Background())
	if err != nil || len(domains) != 2 || calls != 2 {
		t.Fatalf("domains %+v, calls %d, err %v", domains, calls, err)
	}
}

func TestChangeRecordsRunOneAtATimePerDomain(t *testing.T) {
	var mu sync.Mutex
	inFlight, peak := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		inFlight++
		peak = max(peak, inFlight)
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		_, _ = w.Write([]byte(`{"result":{"identifier":"r"}}`))
	}))
	t.Cleanup(srv.Close)
	c := clouddns.New(srv.URL, "k")
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			if err := c.ChangeRecords(context.Background(), 1, nil, nil); err != nil {
				t.Errorf("ChangeRecords: %v", err)
			}
		})
	}
	wg.Wait()
	if peak != 1 {
		t.Fatalf("peak = %d, want 1", peak)
	}
}

func TestOtherErrorsHaveNoCloudDNSHint(t *testing.T) {
	m := mockapi.New(t)
	_, err := clouddns.New(m.URL, "wrong").ActiveRevision(context.Background(), mockapi.CloudDNSDomainID)
	if err == nil || strings.Contains(err.Error(), "does not run on CloudDNS") {
		t.Fatalf("err = %v", err)
	}
}

func TestRecordsSendNoPageParameter(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	revision, err := c.ActiveRevision(ctx, mockapi.CloudDNSDomainID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Records(ctx, mockapi.CloudDNSDomainID, revision.Identifier); err != nil {
		t.Fatal(err)
	}
	requests := m.Requests()
	if last := requests[len(requests)-1]; last.Query.Has("page") {
		t.Fatalf("query = %v", last.Query)
	}
}

func TestWrongKeyIsReported(t *testing.T) {
	m := mockapi.New(t)
	_, err := clouddns.New(m.URL, "wrong").Domains(context.Background())
	if err == nil || !strings.Contains(err.Error(), "authenticationError") {
		t.Fatalf("err = %v", err)
	}
}

func TestRetriesGetOnServerError(t *testing.T) {
	c, m := mockClient(t)
	m.FailNext(http.MethodGet, "/v1/domain", http.StatusServiceUnavailable, 2)
	if _, err := c.Domains(context.Background()); err != nil {
		t.Fatalf("Domains: %v", err)
	}
}

func TestChangesetIsNotRetried(t *testing.T) {
	c, m := mockClient(t)
	m.FailNext(http.MethodPost, "/v1/domain/2001/changeset", http.StatusServiceUnavailable, 1)
	www := clouddns.RecordInput{Name: "www", Type: "A", RData: "192.0.2.1", TTL: 300}
	if err := c.ChangeRecords(context.Background(), mockapi.CloudDNSDomainID, []clouddns.RecordInput{www}, nil); err == nil {
		t.Fatal("expected error")
	}
	if got := len(m.Requests()); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}
