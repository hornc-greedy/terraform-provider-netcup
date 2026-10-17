// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"testing"
)

func TestPatchServerWaitsForTask(t *testing.T) {
	var sent string
	c := newTestClient(t, func(method, path string, body []byte) (int, string) {
		switch {
		case method == "PATCH" && path == "/servers/5":
			sent = string(body)
			return 202, `{"uuid":"task-1","state":"PENDING"}`
		case method == "GET" && path == "/tasks/task-1":
			return 200, `{"uuid":"task-1","state":"FINISHED"}`
		}
		t.Fatalf("unexpected %s %s", method, path)
		return 500, ""
	})
	if err := c.PatchServer(context.Background(), 5, map[string]string{"nickname": "web"}); err != nil {
		t.Fatalf("PatchServer: %v", err)
	}
	if sent != `{"nickname":"web"}` {
		t.Fatalf("body = %s", sent)
	}
}

func TestGetServerLiveNull(t *testing.T) {
	c := newTestClient(t, func(method, path string, _ []byte) (int, string) {
		return 200, `{"id":5,"serverLiveInfo":null}`
	})
	if _, err := c.GetServerLive(context.Background(), 5); err == nil {
		t.Fatal("expected error for null serverLiveInfo")
	}
}
