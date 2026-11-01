// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// Site mirrors the Site schema.
type Site struct {
	ID   *int64 `json:"id"`
	Name string `json:"name"`
	City string `json:"city"`
}

// BandwidthClass mirrors the BandwidthClass schema.
type BandwidthClass struct {
	ID          *int64 `json:"id"`
	Name        string `json:"name"`
	SpeedInMBit *int64 `json:"speedInMBit"`
}

// VLan mirrors the VLan schema.
type VLan struct {
	VlanID         *int64          `json:"vlanId"`
	Name           string          `json:"name"`
	User           *UserMinimal    `json:"user"`
	Site           *Site           `json:"site"`
	BandwidthClass *BandwidthClass `json:"bandwidthClass"`
}

func (c *Client) userVlansPath(ctx context.Context) (string, error) {
	uid, err := c.UserID(ctx)
	if err != nil {
		return "", err
	}
	return "/users/" + strconv.Itoa(uid) + "/vlans", nil
}

// ListUserVlans returns the VLANs of the user. A nil serverID does not filter.
func (c *Client) ListUserVlans(ctx context.Context, serverID *int64) ([]VLan, error) {
	base, err := c.userVlansPath(ctx)
	if err != nil {
		return nil, err
	}
	if serverID != nil {
		base += "?serverId=" + strconv.FormatInt(*serverID, 10)
	}
	var out []VLan
	if err := c.do(ctx, http.MethodGet, base, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetUserVlan returns one VLAN of the user.
func (c *Client) GetUserVlan(ctx context.Context, vlanID int64) (*VLan, error) {
	base, err := c.userVlansPath(ctx)
	if err != nil {
		return nil, err
	}
	var out VLan
	if err := c.do(ctx, http.MethodGet, base+"/"+strconv.FormatInt(vlanID, 10), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetVlan returns one VLAN by its id alone.
func (c *Client) GetVlan(ctx context.Context, vlanID int64) (*VLan, error) {
	var out VLan
	if err := c.do(ctx, http.MethodGet, "/vlans/"+strconv.FormatInt(vlanID, 10), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// vlanPollInterval is the wait between two checks of a VLAN update.
const vlanPollInterval = 2 * time.Second

// UpdateUserVlan renames a VLAN. netcup answers 202 without a task, so it reads
// the VLAN until the name has arrived or the context ends.
func (c *Client) UpdateUserVlan(ctx context.Context, vlanID int64, name string) (*VLan, error) {
	base, err := c.userVlansPath(ctx)
	if err != nil {
		return nil, err
	}
	path := base + "/" + strconv.FormatInt(vlanID, 10)
	if err := c.do(ctx, http.MethodPut, path, map[string]string{"name": name}, nil); err != nil {
		return nil, err
	}
	for {
		vlan, err := c.GetUserVlan(ctx, vlanID)
		if err != nil {
			return nil, err
		}
		if vlan.Name == name {
			return vlan, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(vlanPollInterval):
		}
	}
}
