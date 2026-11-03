// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// UserID returns the internal SCP user id (not the customer number), resolved
// once via /userinfo and cached. Many endpoints are scoped to /users/{userId}.
func (c *Client) UserID(ctx context.Context) (int, error) {
	c.mu.Lock()
	if c.userID != 0 {
		id := c.userID
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	// netcup returns id as a JSON string; json.Number accepts it.
	var parsed struct {
		ID json.Number `json:"id"`
	}
	if err := c.sendTo(ctx, http.MethodGet, c.UserinfoURL, "", nil, &parsed); err != nil {
		return 0, fmt.Errorf("userinfo: %w", err)
	}
	id, err := parsed.ID.Int64()
	if err != nil {
		return 0, fmt.Errorf("userinfo id %q is not an integer: %w", parsed.ID.String(), err)
	}

	c.mu.Lock()
	c.userID = int(id)
	c.mu.Unlock()
	return int(id), nil
}

// User mirrors the User schema.
type User struct {
	ID                     int64   `json:"id"`
	Username               string  `json:"username"`
	Firstname              string  `json:"firstname"`
	Lastname               string  `json:"lastname"`
	Email                  string  `json:"email"`
	Company                *string `json:"company"`
	Language               string  `json:"language"`
	TimeZone               *string `json:"timeZone"`
	ShowNickname           *bool   `json:"showNickname"`
	PasswordlessMode       *bool   `json:"passwordlessMode"`
	APIIPLoginRestrictions *string `json:"apiIpLoginRestrictions"`
}

// UserSave mirrors the UserSave schema.
type UserSave struct {
	ID                     int64   `json:"id"`
	Language               string  `json:"language"`
	TimeZone               string  `json:"timeZone"`
	APIIPLoginRestrictions *string `json:"apiIpLoginRestrictions,omitempty"`
	Password               *string `json:"password,omitempty"`
	OldPassword            *string `json:"oldPassword,omitempty"`
	SoapWebservicePassword *string `json:"soapWebservicePassword,omitempty"`
	ShowNickname           *bool   `json:"showNickname,omitempty"`
	PasswordlessMode       *bool   `json:"passwordlessMode,omitempty"`
}

func (c *Client) userPath(ctx context.Context) (string, error) {
	id, err := c.UserID(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("/users/%d", id), nil
}

// GetUser returns the user of the refresh token.
func (c *Client) GetUser(ctx context.Context) (*User, error) {
	path, err := c.userPath(ctx)
	if err != nil {
		return nil, err
	}
	var user User
	if err := c.do(ctx, http.MethodGet, path, nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// UpdateUser changes the user of the refresh token. A request with a password is
// sent once: oldPassword is stale after a lost answer.
func (c *Client) UpdateUser(ctx context.Context, save UserSave) error {
	path, err := c.userPath(ctx)
	if err != nil {
		return err
	}
	if save.Password != nil || save.SoapWebservicePassword != nil {
		ctx = context.WithValue(ctx, sendOnceKey{}, true)
	}
	return c.do(ctx, http.MethodPut, path, save, nil)
}

// ListUserLogs returns the logs of the user of the refresh token.
func (c *Client) ListUserLogs(ctx context.Context, page Page) ([]Log, error) {
	path, err := c.userPath(ctx)
	if err != nil {
		return nil, err
	}
	return listPage[Log](ctx, c, path+"/logs", nil, page)
}
