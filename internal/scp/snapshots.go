// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"net/url"
	"strconv"
)

// Snapshot mirrors the Snapshot schema.
type Snapshot struct {
	UUID         string   `json:"uuid"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Disks        []string `json:"disks"`
	CreationTime string   `json:"creationTime"`
	State        string   `json:"state"`
	Online       bool     `json:"online"`
}

// snapshotCreate is the body of POST .../snapshots.
type snapshotCreate struct {
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	DiskName       string `json:"diskName,omitempty"`
	OnlineSnapshot bool   `json:"onlineSnapshot,omitempty"`
}

func snapBase(serverID int) string {
	return "/servers/" + strconv.Itoa(serverID) + "/snapshots"
}

// GetSnapshot returns one snapshot by name, or nil if it does not exist.
func (c *Client) GetSnapshot(ctx context.Context, serverID int, name string) (*Snapshot, error) {
	var out Snapshot
	err := c.do(ctx, "GET", snapBase(serverID)+"/"+url.PathEscape(name), nil, &out)
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

// CreateSnapshot creates a snapshot of the given server and waits for the
// asynchronous task to finish. An empty diskName snapshots all disks.
func (c *Client) CreateSnapshot(ctx context.Context, serverID int, name, description, diskName string, online bool) (*Snapshot, error) {
	body := snapshotCreate{Name: name, Description: description, DiskName: diskName, OnlineSnapshot: online}
	var task TaskInfo
	if err := c.do(ctx, "POST", snapBase(serverID), body, &task); err != nil {
		return nil, err
	}
	if err := c.waitForTask(ctx, task.UUID); err != nil {
		return nil, err
	}
	return c.GetSnapshot(ctx, serverID, name)
}

// DeleteSnapshot deletes a snapshot and waits for the asynchronous task to
// finish.
func (c *Client) DeleteSnapshot(ctx context.Context, serverID int, name string) error {
	var task TaskInfo
	if err := c.do(ctx, "DELETE", snapBase(serverID)+"/"+url.PathEscape(name), nil, &task); err != nil {
		return err
	}
	return c.waitForTask(ctx, task.UUID)
}
