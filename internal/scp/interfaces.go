// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"
)

func interfacesPath(serverID int) string {
	return serverPath(serverID) + "/interfaces"
}

func interfacePath(serverID int, mac string) string {
	return interfacesPath(serverID) + "/" + url.PathEscape(mac)
}

// GetInterface returns one network interface of a server, including IPs and
// rDNS entries.
func (c *Client) GetInterface(ctx context.Context, serverID int, mac string) (*Interface, error) {
	var out Interface
	if err := c.do(ctx, http.MethodGet, interfacePath(serverID, mac)+"?loadRdns=true", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// createInterfaceBody mirrors the ServerCreateNicVlan schema.
type createInterfaceBody struct {
	VlanID        int    `json:"vlanId"`
	NetworkDriver string `json:"networkDriver"`
}

// CreateInterface adds an interface in a VLAN, waits for the task and returns its
// MAC. It also returns the MAC with the error if the task did not finish cleanly.
func (c *Client) CreateInterface(ctx context.Context, serverID, vlanID int, driver string) (string, error) {
	unlock, err := c.lockServer(ctx, serverID)
	if err != nil {
		return "", err
	}
	defer unlock()
	before, err := c.GetInterfaces(ctx, serverID)
	if err != nil {
		return "", err
	}
	var task TaskInfo
	if err := c.do(ctx, http.MethodPost, interfacesPath(serverID), createInterfaceBody{VlanID: vlanID, NetworkDriver: driver}, &task); err != nil {
		return "", err
	}
	if err := c.waitForTask(ctx, task.UUID); err != nil {
		// The interface may exist already, so it must not stay untracked.
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		mac, _ := c.newInterface(recovery, serverID, vlanID, before)
		return mac, fmt.Errorf("task %s: %w", task.UUID, err)
	}
	return c.newInterface(ctx, serverID, vlanID, before)
}

// newInterface finds the MAC that was not there before, because the specification
// does not describe the task result. It also compares the VLAN, because other
// clients can add interfaces at the same time.
func (c *Client) newInterface(ctx context.Context, serverID, vlanID int, before []Interface) (string, error) {
	after, err := c.GetInterfaces(ctx, serverID)
	if err != nil {
		return "", err
	}
	server, err := c.GetServer(ctx, serverID)
	if err != nil {
		return "", err
	}
	var found []string
	for _, candidate := range after {
		if slices.ContainsFunc(before, func(known Interface) bool { return known.Mac == candidate.Mac }) {
			continue
		}
		if vlan, known := server.VlanOf(candidate.Mac); !known || vlan != nil && *vlan == vlanID {
			found = append(found, candidate.Mac)
		}
	}
	if len(found) != 1 {
		return "", fmt.Errorf("server %d has %d new interfaces in VLAN %d after the task, expected 1", serverID, len(found), vlanID)
	}
	return found[0], nil
}

// VlanOf returns the VLAN of an interface from the live info. known is false if
// netcup does not report the interface.
func (s *Server) VlanOf(mac string) (vlan *int, known bool) {
	if s.LiveInfo == nil {
		return nil, false
	}
	for _, i := range s.LiveInfo.Interfaces {
		if i.Mac == mac {
			return i.VlanID, true
		}
	}
	return nil, false
}

// UpdateInterface changes the driver of an interface and waits for the task.
func (c *Client) UpdateInterface(ctx context.Context, serverID int, mac, driver string) error {
	var task TaskInfo
	if err := c.do(ctx, http.MethodPut, interfacePath(serverID, mac), map[string]string{"driver": driver}, &task); err != nil {
		return err
	}
	return c.waitForTask(ctx, task.UUID)
}

// DeleteInterface removes an interface and waits for the task.
func (c *Client) DeleteInterface(ctx context.Context, serverID int, mac string) error {
	var task TaskInfo
	if err := c.do(ctx, http.MethodDelete, interfacePath(serverID, mac), nil, &task); err != nil {
		return err
	}
	return c.waitForTask(ctx, task.UUID)
}

// lockServer lets interface creates of a server run one at a time and returns the
// unlock function. Waiting ends with the context.
func (c *Client) lockServer(ctx context.Context, serverID int) (func(), error) {
	c.mu.Lock()
	if c.serverLocks == nil {
		c.serverLocks = map[int]chan struct{}{}
	}
	lock, ok := c.serverLocks[serverID]
	if !ok {
		lock = make(chan struct{}, 1)
		c.serverLocks[serverID] = lock
	}
	c.mu.Unlock()
	select {
	case lock <- struct{}{}:
		return func() { <-lock }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
