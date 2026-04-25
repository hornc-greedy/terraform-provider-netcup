// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, handler func(action string, body []byte) string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Action string `json:"action"`
		}
		_ = json.Unmarshal(body, &req)
		_, _ = io.WriteString(w, handler(req.Action, body))
	}))
	t.Cleanup(srv.Close)

	c := New("123456", "key", "pass")
	c.Endpoint = srv.URL
	return c
}

const loginOK = `{"status":"success","statuscode":2000,"responsedata":{"apisessionid":"SID"}}`

func TestLogin(t *testing.T) {
	c := newTestClient(t, func(action string, _ []byte) string {
		if action != "login" {
			t.Fatalf("unexpected action %q", action)
		}
		return loginOK
	})
	if err := c.ensureSession(context.Background()); err != nil {
		t.Fatalf("ensureSession: %v", err)
	}
	if got := c.session(); got != "SID" {
		t.Fatalf("session = %q, want SID", got)
	}
}

func TestInfoDNSRecords(t *testing.T) {
	c := newTestClient(t, func(action string, _ []byte) string {
		switch action {
		case "login":
			return loginOK
		case "infoDnsRecords":
			return `{"status":"success","statuscode":2000,"responsedata":{"dnsrecords":[
				{"id":"1","hostname":"@","type":"A","destination":"1.2.3.4","priority":"0"},
				{"id":"2","hostname":"@","type":"MX","destination":"mail.example.com","priority":"10"}
			]}}`
		}
		t.Fatalf("unexpected action %q", action)
		return ""
	})
	recs, err := c.InfoDNSRecords(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("InfoDNSRecords: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
	if recs[1].Type != "MX" || recs[1].Priority != "10" {
		t.Fatalf("unexpected record: %+v", recs[1])
	}
}

// TestErrorResponseIsString is the regression test for the crash that affects
// other netcup providers: on error, responsedata is a string, not an object.
func TestErrorResponseIsString(t *testing.T) {
	c := newTestClient(t, func(action string, _ []byte) string {
		if action == "login" {
			return loginOK
		}
		return `{"status":"error","statuscode":5029,"shortmessage":"DNS records not found","longmessage":"Domain not found.","responsedata":"Domain not found in your account."}`
	})
	_, err := c.InfoDNSRecords(context.Background(), "missing.example")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.StatusCode != 5029 {
		t.Fatalf("StatusCode = %d, want 5029", apiErr.StatusCode)
	}
}

func TestCreateDNSRecordResolvesID(t *testing.T) {
	c := newTestClient(t, func(action string, _ []byte) string {
		switch action {
		case "login":
			return loginOK
		case "infoDnsRecords":
			return `{"status":"success","statuscode":2000,"responsedata":{"dnsrecords":[
				{"id":"1","hostname":"@","type":"A","destination":"1.2.3.4","priority":"0"}
			]}}`
		case "updateDnsRecords":
			return `{"status":"success","statuscode":2000,"responsedata":{"dnsrecords":[
				{"id":"1","hostname":"@","type":"A","destination":"1.2.3.4","priority":"0"},
				{"id":"2","hostname":"www","type":"A","destination":"5.6.7.8","priority":"0"}
			]}}`
		}
		t.Fatalf("unexpected action %q", action)
		return ""
	})
	rec, err := c.CreateDNSRecord(context.Background(), "example.com", DNSRecord{
		Hostname: "www", Type: "A", Destination: "5.6.7.8", Priority: "0",
	})
	if err != nil {
		t.Fatalf("CreateDNSRecord: %v", err)
	}
	if rec.ID != "2" {
		t.Fatalf("new record id = %q, want 2", rec.ID)
	}
}

func TestUpdateDNSRecordsSendsRecordSet(t *testing.T) {
	var captured string
	c := newTestClient(t, func(action string, body []byte) string {
		if action == "login" {
			return loginOK
		}
		captured = string(body)
		return `{"status":"success","statuscode":2000,"responsedata":{"dnsrecords":[
			{"id":"9","hostname":"@","type":"TXT","destination":"v=spf1 -all","priority":"0"}
		]}}`
	})
	_, err := c.UpdateDNSRecords(context.Background(), "example.com", []DNSRecord{
		{ID: "9", Hostname: "@", Type: "TXT", Destination: "v=spf1 -all"},
	})
	if err != nil {
		t.Fatalf("UpdateDNSRecords: %v", err)
	}
	if !strings.Contains(captured, `"dnsrecordset"`) || !strings.Contains(captured, `"dnsrecords"`) {
		t.Fatalf("request did not contain dnsrecordset: %s", captured)
	}
}
