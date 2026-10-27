// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/mockapi"
)

func mockClient(t *testing.T) (*Client, *mockapi.Server) {
	t.Helper()
	m := mockapi.New(t)
	c := NewForServer(m.URL, mockapi.RefreshToken)
	c.RetryDelay = time.Millisecond
	return c, m
}

func TestMockServers(t *testing.T) {
	c, _ := mockClient(t)
	ctx := context.Background()
	servers, err := c.ListServers(ctx)
	if err != nil || len(servers) != 1 {
		t.Fatalf("ListServers: %v %+v", err, servers)
	}
	live, err := c.GetServerLive(ctx, mockapi.DefaultServerID)
	if err != nil {
		t.Fatalf("GetServerLive: %v", err)
	}
	if live.LiveInfo.State != "RUNNING" {
		t.Fatalf("state = %q", live.LiveInfo.State)
	}
	if err := c.PatchServer(ctx, mockapi.DefaultServerID, map[string]string{"nickname": "web"}); err != nil {
		t.Fatalf("PatchServer: %v", err)
	}
	updated, err := c.GetServer(ctx, mockapi.DefaultServerID)
	if err != nil || updated.Nickname != "web" || updated.ExportCount != mockapi.DefaultExportCount {
		t.Fatalf("GetServer: %v %+v", err, updated)
	}
	if _, err := c.GetServer(ctx, 1); !IsNotFound(err) {
		t.Fatalf("missing server: %v", err)
	}
}

func TestMockInterfaces(t *testing.T) {
	c, _ := mockClient(t)
	interfaces, err := c.GetInterfaces(context.Background(), mockapi.DefaultServerID)
	if err != nil {
		t.Fatalf("GetInterfaces: %v", err)
	}
	if interfaces[0].Mac != mockapi.DefaultMAC || len(interfaces[0].IPv6Addresses[0].RDNS) != 0 {
		t.Fatalf("unexpected interfaces: %+v", interfaces)
	}
}

func TestMockReverseDNS(t *testing.T) {
	c, _ := mockClient(t)
	ctx := context.Background()
	for _, ip := range []string{mockapi.DefaultIPv4, "2001:db8::1"} {
		if err := c.SetReverseDNS(ctx, ip, "mail.example.com"); err != nil {
			t.Fatalf("SetReverseDNS(%s): %v", ip, err)
		}
		if got, err := c.GetReverseDNS(ctx, ip); err != nil || got != "mail.example.com" {
			t.Fatalf("GetReverseDNS(%s): %q %v", ip, got, err)
		}
		if err := c.DeleteReverseDNS(ctx, ip); err != nil {
			t.Fatalf("DeleteReverseDNS(%s): %v", ip, err)
		}
		if got, err := c.GetReverseDNS(ctx, ip); err != nil || got != "" {
			t.Fatalf("GetReverseDNS after delete (%s): %q %v", ip, got, err)
		}
	}
}

func TestMockSSHKeys(t *testing.T) {
	c, _ := mockClient(t)
	ctx := context.Background()
	created, err := c.CreateSSHKey(ctx, "k", "ssh-ed25519 AAAA")
	if err != nil {
		t.Fatalf("CreateSSHKey: %v", err)
	}
	got, err := c.GetSSHKey(ctx, created.ID)
	if err != nil || got == nil || got.CreatedAt != created.CreatedAt {
		t.Fatalf("GetSSHKey: %v, created %+v, got %+v", err, created, got)
	}
	if err := c.DeleteSSHKey(ctx, created.ID); err != nil {
		t.Fatalf("DeleteSSHKey: %v", err)
	}
	if got, err := c.GetSSHKey(ctx, created.ID); err != nil || got != nil {
		t.Fatalf("GetSSHKey after delete: %v %+v", err, got)
	}
}

func TestMockFirewallPolicies(t *testing.T) {
	c, _ := mockClient(t)
	ctx := context.Background()
	created, err := c.CreateFirewallPolicy(ctx, FirewallPolicy{
		Name: "p",
		Rules: []FirewallRule{{
			Direction: "INGRESS", Protocol: "TCP", Action: "ACCEPT", Sources: []string{"192.0.2.1", "192.0.2.2"},
		}},
	})
	if err != nil {
		t.Fatalf("CreateFirewallPolicy: %v", err)
	}
	updated, err := c.UpdateFirewallPolicy(ctx, created.ID, FirewallPolicy{Name: "q", Rules: []FirewallRule{}})
	if err != nil || updated.Name != "q" || len(updated.Rules) != 0 {
		t.Fatalf("UpdateFirewallPolicy: %v %+v", err, updated)
	}
	policies, err := c.ListFirewallPolicies(ctx)
	if err != nil || len(policies) != 1 {
		t.Fatalf("ListFirewallPolicies: %v %+v", err, policies)
	}
	if err := c.DeleteFirewallPolicy(ctx, created.ID); err != nil {
		t.Fatalf("DeleteFirewallPolicy: %v", err)
	}
	if _, err := c.GetFirewallPolicy(ctx, created.ID); !IsNotFound(err) {
		t.Fatalf("policy after delete: %v", err)
	}
}

func TestMockServerFirewall(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	policy, err := c.CreateFirewallPolicy(ctx, FirewallPolicy{Name: "p", Rules: []FirewallRule{}})
	if err != nil {
		t.Fatalf("CreateFirewallPolicy: %v", err)
	}
	if err := c.SetServerFirewall(ctx, mockapi.DefaultServerID, mockapi.DefaultMAC, []int{policy.ID}, true, true); err != nil {
		t.Fatalf("SetServerFirewall: %v", err)
	}
	copied, user, active := m.FirewallOf(mockapi.DefaultMAC)
	if len(copied) != 1 || copied[0] != mockapi.CopiedPolicyID || len(user) != 1 || !active {
		t.Fatalf("keepCopied: copied %v, user %v, active %v", copied, user, active)
	}
	if err := c.SetServerFirewall(ctx, mockapi.DefaultServerID, mockapi.DefaultMAC, nil, false, false); err != nil {
		t.Fatalf("SetServerFirewall: %v", err)
	}
	copied, user, active = m.FirewallOf(mockapi.DefaultMAC)
	if len(copied) != 0 || len(user) != 0 || active {
		t.Fatalf("cleared: copied %v, user %v, active %v", copied, user, active)
	}
	if _, err := c.GetServerFirewall(ctx, mockapi.DefaultServerID, "aa:bb:cc:dd:ee:02"); !IsNotFound(err) {
		t.Fatalf("missing interface: %v", err)
	}
}

func TestMockSnapshots(t *testing.T) {
	c, _ := mockClient(t)
	ctx := context.Background()
	if _, err := c.CreateSnapshot(ctx, mockapi.DefaultServerID, "s", "", "", false); err == nil {
		t.Fatal("offline snapshot without disk must fail")
	}
	snap, err := c.CreateSnapshot(ctx, mockapi.DefaultServerID, "s", "d", "vda", false)
	if err != nil || snap == nil || snap.Disks[0] != "vda" {
		t.Fatalf("CreateSnapshot: %v %+v", err, snap)
	}
	if err := c.DeleteSnapshot(ctx, mockapi.DefaultServerID, "s"); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
	if got, err := c.GetSnapshot(ctx, mockapi.DefaultServerID, "s"); err != nil || got != nil {
		t.Fatalf("GetSnapshot after delete: %v %+v", err, got)
	}
}

func TestMockFailover(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	found, server, err := c.FailoverRoute(ctx, mockapi.FailoverV6ID, true)
	if err != nil || !found || server != nil {
		t.Fatalf("unrouted: %v %v %+v", err, found, server)
	}
	if err := c.RouteFailover(ctx, mockapi.FailoverV6ID, mockapi.DefaultServerID, true); err != nil {
		t.Fatalf("RouteFailover: %v", err)
	}
	found, server, err = c.FailoverRoute(ctx, mockapi.FailoverV6ID, true)
	if err != nil || !found || server == nil || server.ID != mockapi.DefaultServerID {
		t.Fatalf("routed: %v %v %+v", err, found, server)
	}
	m.RemoveFailover(mockapi.FailoverV4ID)
	if found, _, err := c.FailoverRoute(ctx, mockapi.FailoverV4ID, false); err != nil || found {
		t.Fatalf("removed: %v %v", err, found)
	}
}

func TestMockMaintenance(t *testing.T) {
	c, _ := mockClient(t)
	if _, err := c.GetMaintenance(context.Background()); err != nil {
		t.Fatalf("GetMaintenance: %v", err)
	}
}

func TestMockFailedTaskIsReported(t *testing.T) {
	c, m := mockClient(t)
	m.FailTasks("disk full")
	err := c.PatchServer(context.Background(), mockapi.DefaultServerID, map[string]string{"nickname": "web"})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v", err)
	}
}

func TestMockRetriesInjectedFailures(t *testing.T) {
	c, m := mockClient(t)
	m.FailNext(http.MethodGet, "/api/v1/servers", http.StatusServiceUnavailable, 2)
	if _, err := c.ListServers(context.Background()); err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	// Two failures, the first page and the empty page that ends the list.
	if got := len(m.Requests()); got != 4 {
		t.Fatalf("requests = %d, want 4", got)
	}
}
