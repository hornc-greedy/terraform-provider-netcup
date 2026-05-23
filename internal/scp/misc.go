// SPDX-License-Identifier: MPL-2.0

package scp

import "context"

// Maintenance mirrors the Maintenance schema. Both timestamps are null when no
// maintenance is scheduled.
type Maintenance struct {
	StartAt  string `json:"startAt"`
	FinishAt string `json:"finishAt"`
}

// GetMaintenance returns the system maintenance window, if any.
func (c *Client) GetMaintenance(ctx context.Context) (*Maintenance, error) {
	var out Maintenance
	if err := c.do(ctx, "GET", "/maintenance", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
