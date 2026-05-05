// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"strings"
	"testing"
)

func TestInfoDNSZone(t *testing.T) {
	c := newTestClient(t, func(action string, _ []byte) string {
		switch action {
		case "login":
			return loginOK
		case "infoDnsZone":
			return `{"status":"success","statuscode":2000,"responsedata":{"name":"example.com","ttl":"86400","serial":"2024010101","refresh":"28800","retry":"7200","expire":"1209600","dnssecstatus":false}}`
		}
		t.Fatalf("unexpected action %q", action)
		return ""
	})
	z, err := c.InfoDNSZone(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("InfoDNSZone: %v", err)
	}
	if z.TTL != "86400" || z.Refresh != "28800" || z.Expire != "1209600" {
		t.Fatalf("unexpected zone: %+v", z)
	}
}

func TestUpdateDNSZoneSendsZone(t *testing.T) {
	var body string
	c := newTestClient(t, func(action string, b []byte) string {
		if action == "login" {
			return loginOK
		}
		body = string(b)
		return `{"status":"success","statuscode":2000,"responsedata":{"name":"example.com","ttl":"3600","serial":"2024010102","refresh":"28800","retry":"7200","expire":"1209600","dnssecstatus":false}}`
	})
	z, err := c.UpdateDNSZone(context.Background(), "example.com", DNSZone{
		Name: "example.com", TTL: "3600", Refresh: "28800", Retry: "7200", Expire: "1209600",
	})
	if err != nil {
		t.Fatalf("UpdateDNSZone: %v", err)
	}
	if z.TTL != "3600" {
		t.Fatalf("ttl = %q, want 3600", z.TTL)
	}
	if !strings.Contains(body, `"dnszone"`) {
		t.Fatalf("request did not contain dnszone: %s", body)
	}
}
