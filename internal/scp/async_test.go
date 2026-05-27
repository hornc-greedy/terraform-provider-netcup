// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"strings"
	"testing"
)

func TestSnapshotCreateWaitsForTask(t *testing.T) {
	polls := 0
	c := newTestClient(t, func(method, path string, _ []byte) (int, string) {
		switch {
		case method == "POST" && path == "/servers/5/snapshots":
			return 202, `{"uuid":"task-1","name":"create","state":"PENDING"}`
		case method == "GET" && path == "/tasks/task-1":
			polls++
			if polls < 2 {
				return 200, `{"uuid":"task-1","state":"RUNNING"}`
			}
			return 200, `{"uuid":"task-1","state":"FINISHED"}`
		case method == "GET" && path == "/servers/5/snapshots/snap1":
			return 200, `{"uuid":"s-1","name":"snap1","state":"SHUTOFF","disks":["scsi0"]}`
		}
		t.Fatalf("unexpected %s %s", method, path)
		return 500, ""
	})
	snap, err := c.CreateSnapshot(context.Background(), 5, "snap1", "d", "", false)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if snap.Name != "snap1" {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	if polls < 2 {
		t.Fatalf("expected task to be polled until done, polls=%d", polls)
	}
}

func TestTaskErrorIsReported(t *testing.T) {
	c := newTestClient(t, func(method, path string, _ []byte) (int, string) {
		switch {
		case method == "DELETE" && strings.HasPrefix(path, "/servers/5/snapshots/"):
			return 202, `{"uuid":"task-x","state":"PENDING"}`
		case path == "/tasks/task-x":
			return 200, `{"uuid":"task-x","state":"ERROR","message":"boom"}`
		}
		t.Fatalf("unexpected %s %s", method, path)
		return 500, ""
	})
	err := c.DeleteSnapshot(context.Background(), 5, "snap1")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected task error, got %v", err)
	}
}

func TestRouteFailoverSendsServerID(t *testing.T) {
	var body string
	c := sshTestClient(t, func(method, path string, b []byte) (int, string) {
		switch {
		case method == "PATCH" && path == "/users/78513/failoverips/v4/9":
			body = string(b)
			return 202, `{"uuid":"t","state":"FINISHED"}`
		case path == "/tasks/t":
			return 200, `{"uuid":"t","state":"FINISHED"}`
		}
		t.Fatalf("unexpected %s %s", method, path)
		return 500, ""
	})
	if err := c.RouteFailover(context.Background(), 9, 296155, false); err != nil {
		t.Fatalf("RouteFailover: %v", err)
	}
	if !strings.Contains(body, `"serverId":296155`) {
		t.Fatalf("serverId not sent: %s", body)
	}
}

func TestSetServerFirewallBody(t *testing.T) {
	var body string
	c := newTestClient(t, func(method, path string, b []byte) (int, string) {
		switch {
		case method == "PUT" && path == "/servers/5/interfaces/aa:bb/firewall":
			body = string(b)
			return 202, `{"uuid":"t","state":"FINISHED"}`
		case path == "/tasks/t":
			return 200, `{"uuid":"t","state":"FINISHED"}`
		}
		t.Fatalf("unexpected %s %s", method, path)
		return 500, ""
	})
	if err := c.SetServerFirewall(context.Background(), 5, "aa:bb", []int{7, 8}, true); err != nil {
		t.Fatalf("SetServerFirewall: %v", err)
	}
	if !strings.Contains(body, `"userPolicies":[{"id":7},{"id":8}]`) || !strings.Contains(body, `"active":true`) {
		t.Fatalf("unexpected body: %s", body)
	}
}
