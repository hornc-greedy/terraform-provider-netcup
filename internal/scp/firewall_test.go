// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"strings"
	"testing"
)

func TestFirewallPolicyCRUD(t *testing.T) {
	c := sshTestClient(t, func(method, path string, body []byte) (int, string) {
		switch {
		case method == "POST" && path == "/users/78513/firewall-policies":
			// Verify the correct (plural) field names are sent.
			b := string(body)
			if !strings.Contains(b, `"sources"`) || !strings.Contains(b, `"destinationPorts"`) {
				t.Fatalf("rule field names wrong in body: %s", b)
			}
			if strings.Contains(b, `"enabled"`) || strings.Contains(b, `"comment"`) {
				t.Fatalf("body contains non-existent fields: %s", b)
			}
			return 201, `{"id":7,"name":"p","description":"d","rules":[{"action":"ACCEPT","protocol":"TCP","direction":"INGRESS","destinationPorts":"22","sources":["0.0.0.0/0"],"description":"ssh"}]}`
		case method == "GET" && path == "/users/78513/firewall-policies/7":
			return 200, `{"id":7,"name":"p","description":"d","rules":[{"action":"ACCEPT","protocol":"TCP","direction":"INGRESS","destinationPorts":"22","sources":["0.0.0.0/0"],"description":"ssh"}]}`
		case method == "PUT" && path == "/users/78513/firewall-policies/7":
			return 200, ``
		case method == "DELETE" && path == "/users/78513/firewall-policies/7":
			return 204, ``
		}
		t.Fatalf("unexpected %s %s", method, path)
		return 500, ""
	})
	ctx := context.Background()

	created, err := c.CreateFirewallPolicy(ctx, FirewallPolicy{
		Name: "p", Description: "d",
		Rules: []FirewallRule{{
			Action: "ACCEPT", Protocol: "TCP", Direction: "INGRESS",
			DestinationPorts: "22", Sources: []string{"0.0.0.0/0"}, Description: "ssh",
		}},
	})
	if err != nil {
		t.Fatalf("CreateFirewallPolicy: %v", err)
	}
	if created.ID != 7 || len(created.Rules) != 1 {
		t.Fatalf("unexpected policy: %+v", created)
	}
	if created.Rules[0].Sources[0] != "0.0.0.0/0" {
		t.Fatalf("sources not decoded: %+v", created.Rules[0])
	}
	if _, err := c.UpdateFirewallPolicy(ctx, 7, *created); err != nil {
		t.Fatalf("UpdateFirewallPolicy: %v", err)
	}
	if err := c.DeleteFirewallPolicy(ctx, 7); err != nil {
		t.Fatalf("DeleteFirewallPolicy: %v", err)
	}
}

func TestMaintenanceDecode(t *testing.T) {
	c := newTestClient(t, func(method, path string, _ []byte) (int, string) {
		return 200, `{"startAt":null,"finishAt":null}`
	})
	m, err := c.GetMaintenance(context.Background())
	if err != nil {
		t.Fatalf("GetMaintenance: %v", err)
	}
	if m.StartAt != "" || m.FinishAt != "" {
		t.Fatalf("expected empty timestamps, got %+v", m)
	}
}
