// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"strings"
	"testing"
)

// userinfoTestClient wires a client whose userinfo endpoint also points at the
// test server, so UserID() resolves without hitting the real realm.
func sshTestClient(t *testing.T, api func(method, path string, body []byte) (int, string)) *Client {
	c := newTestClient(t, func(method, path string, body []byte) (int, string) {
		if strings.HasSuffix(path, "/userinfo") {
			return 200, `{"id":"78513"}`
		}
		return api(method, path, body)
	})
	c.UserinfoURL = c.BaseURL + "/userinfo"
	return c
}

func TestSSHKeyCRUD(t *testing.T) {
	c := sshTestClient(t, func(method, path string, _ []byte) (int, string) {
		switch {
		case method == "POST" && path == "/users/78513/ssh-keys":
			return 201, `{"id":42,"name":"k","key":"ssh-ed25519 AAA","createdAt":"2026-01-01T00:00:00Z"}`
		case method == "GET" && path == "/users/78513/ssh-keys":
			return 200, `[{"id":42,"name":"k","key":"ssh-ed25519 AAA","createdAt":"2026-01-01T00:00:00Z"}]`
		case method == "DELETE" && path == "/users/78513/ssh-keys/42":
			return 204, ``
		}
		t.Fatalf("unexpected %s %s", method, path)
		return 500, ""
	})
	ctx := context.Background()

	created, err := c.CreateSSHKey(ctx, "k", "ssh-ed25519 AAA")
	if err != nil {
		t.Fatalf("CreateSSHKey: %v", err)
	}
	if created.ID != 42 {
		t.Fatalf("id = %d, want 42", created.ID)
	}
	got, err := c.GetSSHKey(ctx, 42)
	if err != nil || got == nil {
		t.Fatalf("GetSSHKey: %v / %v", got, err)
	}
	if got.Name != "k" {
		t.Fatalf("name = %q", got.Name)
	}
	if err := c.DeleteSSHKey(ctx, 42); err != nil {
		t.Fatalf("DeleteSSHKey: %v", err)
	}
}

func TestGetSSHKeyMissing(t *testing.T) {
	c := sshTestClient(t, func(method, path string, _ []byte) (int, string) {
		return 200, `[]`
	})
	got, err := c.GetSSHKey(context.Background(), 99)
	if err != nil {
		t.Fatalf("GetSSHKey: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing key, got %+v", got)
	}
}

// firewall + maintenance tests live in firewall_test.go.
