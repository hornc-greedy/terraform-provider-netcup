// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"fmt"
	"time"
)

// TaskInfo is the (subset of the) response of asynchronous operations and of
// GET /tasks/{uuid}.
type TaskInfo struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	State   string `json:"state"`
	Message string `json:"message"`
}

// done reports whether the task has reached a terminal state and polling can
// stop.
func (t TaskInfo) done() bool {
	switch t.State {
	case "FINISHED", "ERROR", "CANCELED", "ROLLBACK":
		return true
	}
	return false
}

// GetTask returns the current state of a task.
func (c *Client) GetTask(ctx context.Context, uuid string) (*TaskInfo, error) {
	var out TaskInfo
	if err := c.do(ctx, "GET", "/tasks/"+uuid, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// waitForTask polls a task to a terminal state. It returns an error if the task
// ends in any state other than FINISHED, or if the context is canceled.
func (c *Client) waitForTask(ctx context.Context, uuid string) error {
	if uuid == "" {
		return nil // synchronous operation, nothing to wait for
	}
	const interval = 2 * time.Second
	for {
		t, err := c.GetTask(ctx, uuid)
		if err != nil {
			return err
		}
		if t.done() {
			if t.State != "FINISHED" {
				return fmt.Errorf("task %s ended in state %s: %s", uuid, t.State, t.Message)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
