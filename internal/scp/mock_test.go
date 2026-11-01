// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
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

func TestMockRejectsBodylessWriteWithoutContentType(t *testing.T) {
	c, _ := mockClient(t)
	token, err := c.ensureToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for method, path := range map[string]string{
		http.MethodPost: fmt.Sprintf("/servers/%d/rescuesystem", mockapi.DefaultServerID),
	} {
		req, err := http.NewRequestWithContext(context.Background(), method, c.BaseURL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s %s: status = %d, want 400", method, path, res.StatusCode)
		}
	}
}

func TestMockServerInfo(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	const id = mockapi.DefaultServerID
	disks, err := c.ListDisks(ctx, id)
	if err != nil || len(disks) != 2 {
		t.Fatalf("ListDisks: %v %+v", err, disks)
	}
	if disk, err := c.GetDisk(ctx, id, disks[0].Name); err != nil || disk.StorageDriver != "VIRTIO" {
		t.Fatalf("GetDisk: %v %+v", err, disk)
	}
	if drivers, err := c.ListSupportedDrivers(ctx, id); err != nil || len(drivers) != 2 {
		t.Fatalf("ListSupportedDrivers: %v %v", err, drivers)
	}
	if _, err := c.GetGPUDriver(ctx, id); err == nil {
		t.Fatal("GPU driver of a server without one must fail")
	}
	m.EnableGPU()
	if info, err := c.GetGPUDriver(ctx, id); err != nil || info.PresignedURL == "" || len(info.Headers) != 1 {
		t.Fatalf("GetGPUDriver: %v %+v", err, info)
	}
	if agent, err := c.GetGuestAgent(ctx, id); err != nil || !*agent.GuestAgentAvailable || agent.GuestAgentData == nil {
		t.Fatalf("GetGuestAgent: %v %+v", err, agent)
	}
	m.RemoveGuestAgent()
	if agent, err := c.GetGuestAgent(ctx, id); err != nil || *agent.GuestAgentAvailable || agent.GuestAgentData != nil {
		t.Fatalf("GetGuestAgent without data: %v %+v", err, agent)
	}
	if available, err := c.GetGuestAgentStatus(ctx, id); err != nil || *available {
		t.Fatalf("GetGuestAgentStatus after removal: %v %v", err, available)
	}
	if flavours, err := c.ListImageFlavours(ctx, id); err != nil || flavours[0].Image == nil || flavours[0].Image.Name == "" {
		t.Fatalf("ListImageFlavours: %v %+v", err, flavours)
	}
	if images, err := c.ListIsoImages(ctx, id); err != nil || images[0].Filename == "" || images[0].Description != nil {
		t.Fatalf("ListIsoImages: %v %+v", err, images)
	}
}

func TestMockServerLogsPaging(t *testing.T) {
	c, _ := mockClient(t)
	ctx := context.Background()
	all, err := c.ListServerLogs(ctx, mockapi.DefaultServerID, Page{})
	if err != nil || len(all) != 2 {
		t.Fatalf("all pages: %v %+v", err, all)
	}
	one, err := c.ListServerLogs(ctx, mockapi.DefaultServerID, Page{Limit: 1, Offset: 1})
	if err != nil || len(one) != 1 || one[0].LogKey != all[1].LogKey {
		t.Fatalf("second entry: %v %+v", err, one)
	}
}

func TestMockMetrics(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	hours := int64(2)
	for metric, want := range map[Metric]string{MetricCPU: "cpu0", MetricDisk: "vda", MetricNetwork: "eth0", MetricNetworkPacket: "eth0packets"} {
		data, err := c.GetMetrics(ctx, mockapi.DefaultServerID, metric, &hours)
		if err != nil || !strings.Contains(string(data), `"`+want+`"`) {
			t.Fatalf("%s: %v %s", metric, err, data)
		}
		requests := m.Requests()
		if got := requests[len(requests)-1].Query.Get("hours"); got != "2" {
			t.Fatalf("%s: hours = %q", metric, got)
		}
	}
	if _, err := c.GetMetrics(ctx, mockapi.DefaultServerID, MetricCPU, nil); err != nil {
		t.Fatal(err)
	}
	requests := m.Requests()
	if requests[len(requests)-1].Query.Has("hours") {
		t.Fatal("hours sent although unset")
	}
}

func TestMockSnapshotList(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	if _, err := c.CreateSnapshot(ctx, mockapi.DefaultServerID, "s", "", "vda", false); err != nil {
		t.Fatal(err)
	}
	snapshots, err := c.ListSnapshots(ctx, mockapi.DefaultServerID)
	if err != nil || len(snapshots) != 1 || *snapshots[0].Exported || snapshots[0].ExportedSizeInKiB != nil {
		t.Fatalf("ListSnapshots: %v %+v", err, snapshots)
	}
	m.MarkSnapshotExported("s", 2048)
	snapshots, err = c.ListSnapshots(ctx, mockapi.DefaultServerID)
	if err != nil || !*snapshots[0].Exported || snapshots[0].ExportedSizeInKiB == nil || *snapshots[0].ExportedSizeInKiB != 2048 {
		t.Fatalf("exported: %v %+v", err, snapshots)
	}
}

func TestMockSnapshotDryRun(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	const id = mockapi.DefaultServerID
	cases := []struct {
		disk   string
		online bool
		code   string
		body   string
	}{
		{"vda", false, "", `{"diskName":"vda","onlineSnapshot":false}`},
		{"", true, "", `{"onlineSnapshot":true}`},
		{"", false, "server.snapshot.create.error.disk", `{"onlineSnapshot":false}`},
		{"vda", true, "server.snapshot.create.error.online.diskselected", `{"diskName":"vda","onlineSnapshot":true}`},
	}
	for _, tc := range cases {
		found, err := c.DryRunSnapshot(ctx, id, tc.disk, tc.online)
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		if tc.code == "" && len(found) != 0 || tc.code != "" && (len(found) != 1 || found[0].Code != tc.code) {
			t.Fatalf("%+v: errors %+v", tc, found)
		}
		requests := m.Requests()
		if got := requests[len(requests)-1].Body; got != tc.body {
			t.Fatalf("body = %s, want %s", got, tc.body)
		}
	}
	if _, err := c.DryRunSnapshot(ctx, 1, "vda", false); !IsNotFound(err) {
		t.Fatalf("missing server: %v", err)
	}
}

func TestDryRunRenewsTokenAfterUnauthorized(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	m.FailNext(http.MethodPost, fmt.Sprintf("/api/v1/servers/%d/snapshots:dryrun", mockapi.DefaultServerID), http.StatusUnauthorized, 1)
	if _, err := c.DryRunSnapshot(ctx, mockapi.DefaultServerID, "vda", false); err != nil {
		t.Fatal(err)
	}
}

func TestDryRunKeepsMessageOfObjectError(t *testing.T) {
	c := newTestClient(t, func(_, _ string, _ []byte) (int, string) {
		return 400, `{"code":"c","message":"boom"}`
	})
	_, err := c.DryRunSnapshot(context.Background(), 5, "vda", false)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestMockTasks(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	if err := c.PatchServer(ctx, mockapi.DefaultServerID, map[string]string{"nickname": "web"}); err != nil {
		t.Fatal(err)
	}
	serverID := int64(mockapi.DefaultServerID)
	finished, name := "FINISHED", "ServerPatch"
	tasks, err := c.ListTasks(ctx, TaskFilter{Q: &name, ServerID: &serverID, State: &finished}, Page{})
	if err != nil || len(tasks) != 1 {
		t.Fatalf("ListTasks: %v %+v", err, tasks)
	}
	if tasks[0].FinishedAt == "" || *tasks[0].TaskProgress.ProgressInPercent != 100 || tasks[0].ExecutingUser.Username != "mock" {
		t.Fatalf("task fields: %+v", tasks[0])
	}
	requests := m.Requests()
	query := requests[len(requests)-2].Query
	if query.Get("state") != "FINISHED" || query.Get("q") != "ServerPatch" || query.Get("serverId") != fmt.Sprint(serverID) || query.Get("limit") == "" {
		t.Fatalf("query = %v", query)
	}
	other := int64(1)
	if tasks, err := c.ListTasks(ctx, TaskFilter{ServerID: &other}, Page{}); err != nil || len(tasks) != 0 {
		t.Fatalf("tasks of another server: %v %+v", err, tasks)
	}
	zero := int64(0)
	if _, err := c.ListTasks(ctx, TaskFilter{ServerID: &zero}, Page{Limit: 5, Offset: 2}); err != nil {
		t.Fatal(err)
	}
	requests = m.Requests()
	query = requests[len(requests)-1].Query
	if query.Get("serverId") != "0" || query.Get("limit") != "5" || query.Get("offset") != "2" {
		t.Fatalf("query = %v", query)
	}
}

func TestMockOffsetWithoutLimit(t *testing.T) {
	c, _ := mockClient(t)
	logs, err := c.ListServerLogs(context.Background(), mockapi.DefaultServerID, Page{Offset: 1})
	if err != nil || len(logs) != 1 || logs[0].LogKey != "server.rebooted" {
		t.Fatalf("logs: %v %+v", err, logs)
	}
}

func TestDryRunRejectsOtherOKBody(t *testing.T) {
	c := newTestClient(t, func(_, _ string, _ []byte) (int, string) {
		return 200, `{}`
	})
	if _, err := c.DryRunSnapshot(context.Background(), 5, "vda", false); err == nil {
		t.Fatal("a body that is no list must fail")
	}
}

func TestMockInterfaceLifecycle(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	const id = mockapi.DefaultServerID
	mac, err := c.CreateInterface(ctx, id, 100, "VIRTIO")
	if err != nil || mac == "" {
		t.Fatalf("CreateInterface: %q %v", mac, err)
	}
	if err := c.UpdateInterface(ctx, id, mac, "E1000"); err != nil {
		t.Fatalf("UpdateInterface: %v", err)
	}
	if got := m.InterfaceDriver(mac); got != "E1000" {
		t.Fatalf("driver = %q", got)
	}
	if got := m.InterfaceVlan(mac); got != 100 {
		t.Fatalf("vlan = %d", got)
	}
	if err := c.UpdateInterface(ctx, id, mac, "E1000"); err != nil {
		t.Fatalf("UpdateInterface without change: %v", err)
	}
	got, err := c.GetInterface(ctx, id, mac)
	if err != nil || got.Mac != mac || got.Driver != "e1000" {
		t.Fatalf("GetInterface: %v %+v", err, got)
	}
	server, err := c.GetServer(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if vlan, known := server.VlanOf(mac); !known || *vlan != 100 {
		t.Fatalf("VlanOf: %v %v", vlan, known)
	}
	if vlan, known := server.VlanOf(mockapi.DefaultMAC); !known || vlan != nil {
		t.Fatalf("VlanOf of the public interface: %v %v", vlan, known)
	}
	if err := c.DeleteInterface(ctx, id, mac); err != nil {
		t.Fatalf("DeleteInterface: %v", err)
	}
	if _, err := c.GetInterface(ctx, id, mac); !IsNotFound(err) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestMockInterfacesCreatedInParallelGetTheirOwnMAC(t *testing.T) {
	c, _ := mockClient(t)
	ctx := context.Background()
	macs := make([]string, 5)
	var wg sync.WaitGroup
	for i := range macs {
		wg.Go(func() {
			mac, err := c.CreateInterface(ctx, mockapi.DefaultServerID, 100, "VIRTIO")
			if err != nil {
				t.Errorf("CreateInterface: %v", err)
			}
			macs[i] = mac
		})
	}
	wg.Wait()
	slices.Sort(macs)
	if got := slices.Compact(macs); len(got) != 5 {
		t.Fatalf("macs = %v", macs)
	}
}

func TestMockInterfaceLimitIsReported(t *testing.T) {
	c, _ := mockClient(t)
	ctx := context.Background()
	for range 9 {
		if _, err := c.CreateInterface(ctx, mockapi.DefaultServerID, 100, "VIRTIO"); err != nil {
			t.Fatal(err)
		}
	}
	_, err := c.CreateInterface(ctx, mockapi.DefaultServerID, 100, "VIRTIO")
	if err == nil || !strings.Contains(err.Error(), "Maximum amount") {
		t.Fatalf("err = %v", err)
	}
}

func TestMockInterfaceTaskFailureStillReturnsMAC(t *testing.T) {
	c, m := mockClient(t)
	m.FailTasks("disk full")
	mac, err := c.CreateInterface(context.Background(), mockapi.DefaultServerID, 100, "VIRTIO")
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v", err)
	}
	if mac == "" {
		t.Fatal("MAC of the created interface is missing")
	}
}

func TestLockServerEndsWithContext(t *testing.T) {
	c, _ := mockClient(t)
	unlock, err := c.lockServer(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.lockServer(ctx, 1); err == nil {
		t.Fatal("waiting for the lock must end with the context")
	}
}

func TestMockVlans(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	id := int64(mockapi.VlanID)
	if v, err := c.GetVlan(ctx, id); err != nil || v.Name != "default" || *v.BandwidthClass.SpeedInMBit != 1000 {
		t.Fatalf("GetVlan: %v %+v", err, v)
	}
	if v, err := c.GetUserVlan(ctx, id); err != nil || v.Site.City == "" || v.User.Username != "mock" {
		t.Fatalf("GetUserVlan: %v %+v", err, v)
	}
	if _, err := c.GetUserVlan(ctx, 1); !IsNotFound(err) {
		t.Fatalf("missing VLAN: %v", err)
	}
	if v, err := c.GetVlan(ctx, mockapi.ForeignVlanID); err != nil || v.Name != "foreign" {
		t.Fatalf("GetVlan of another user: %v %+v", err, v)
	}
	if _, err := c.GetUserVlan(ctx, mockapi.ForeignVlanID); !IsNotFound(err) {
		t.Fatalf("GetUserVlan of another user: %v", err)
	}
	all, err := c.ListUserVlans(ctx, nil)
	if err != nil || len(all) != 1 {
		t.Fatalf("ListUserVlans: %v %+v", err, all)
	}
	other := int64(1)
	if none, err := c.ListUserVlans(ctx, &other); err != nil || len(none) != 0 {
		t.Fatalf("VLANs of another server: %v %+v", err, none)
	}
	requests := m.Requests()
	if got := requests[len(requests)-1].Query.Get("serverId"); got != "1" {
		t.Fatalf("serverId = %q", got)
	}
}

func TestMockVlanRenameWaitsForTheName(t *testing.T) {
	c, m := mockClient(t)
	m.DelayVlanRename(1)
	v, err := c.UpdateUserVlan(context.Background(), mockapi.VlanID, "private")
	if err != nil || v.Name != "private" {
		t.Fatalf("UpdateUserVlan: %v %+v", err, v)
	}
}

func TestVlanRenameEndsWithContext(t *testing.T) {
	c, m := mockClient(t)
	m.DelayVlanRename(1000)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.UpdateUserVlan(ctx, mockapi.VlanID, "private"); err == nil {
		t.Fatal("waiting must end with the context")
	}
}

func TestMockIso(t *testing.T) {
	c, _ := mockClient(t)
	ctx := context.Background()
	const id = mockapi.DefaultServerID
	iso, err := c.GetIso(ctx, id)
	if err != nil || iso.IsoAttached || iso.Iso != "" {
		t.Fatalf("no ISO: %v %+v", err, iso)
	}
	name := "uploaded.iso"
	if started, err := c.AttachIso(ctx, id, nil, &name, nil); err != nil || !started {
		t.Fatalf("AttachIso: %v %v", started, err)
	}
	iso, err = c.GetIso(ctx, id)
	if err != nil || !iso.IsoAttached || iso.Iso != name {
		t.Fatalf("attached: %v %+v", err, iso)
	}
	if err := c.DetachIso(ctx, id); err != nil {
		t.Fatalf("DetachIso: %v", err)
	}
}

func TestMockRescueSystem(t *testing.T) {
	c, m := mockClient(t)
	ctx := context.Background()
	const id = mockapi.DefaultServerID
	if err := c.DeactivateRescueSystem(ctx, id); !HasCode(err, RescueSystemNotActive) {
		t.Fatalf("deactivating an inactive rescue system: %v", err)
	}
	if started, err := c.ActivateRescueSystem(ctx, id); !HasCode(err, "server.rescuesystem.invalidstate") || started {
		t.Fatalf("activating on a running server: %v %v", started, err)
	}
	m.ShutOff()
	if started, err := c.ActivateRescueSystem(ctx, id); err != nil || !started {
		t.Fatalf("ActivateRescueSystem: %v %v", started, err)
	}
	status, err := c.GetRescueSystem(ctx, id)
	if err != nil || !*status.Active || status.Password == "" {
		t.Fatalf("active: %v %+v", err, status)
	}
	if started, err := c.ActivateRescueSystem(ctx, id); err == nil || started {
		t.Fatalf("activating an active rescue system: %v %v", started, err)
	}
	if err := c.DeactivateRescueSystem(ctx, id); err != nil {
		t.Fatalf("DeactivateRescueSystem: %v", err)
	}
	status, err = c.GetRescueSystem(ctx, id)
	if err != nil || *status.Active || status.Password != "" {
		t.Fatalf("inactive: %v %+v", err, status)
	}
}
