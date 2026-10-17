// SPDX-License-Identifier: MPL-2.0

// Package scp is a small client for the netcup Server Control Panel (SCP)
// REST API at servercontrolpanel.de. It is independent of the CCP/Domain
// client: separate base URL and separate authentication (OAuth2/OIDC).
package scp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBaseURL is the SCP REST API base (note the scp-core prefix).
	DefaultBaseURL = "https://www.servercontrolpanel.de/scp-core/api/v1"
	// DefaultTokenURL is the OIDC token endpoint of the scp realm.
	DefaultTokenURL = "https://www.servercontrolpanel.de/realms/scp/protocol/openid-connect/token"
	// clientID is the public OAuth2 client; no secret is required.
	clientID = "scp"
)

// Client talks to the SCP REST API. Authentication uses an offline refresh
// token (obtained once via the OIDC device flow) which is exchanged for a
// short-lived access token. netcup's offline refresh token may be reused across
// runs as long as it is used at least once every 30 days, so the same token
// from provider configuration works for every Terraform run.
type Client struct {
	RefreshToken string
	BaseURL      string
	TokenURL     string
	UserinfoURL  string
	HTTPClient   *http.Client

	mu          sync.Mutex
	accessToken string
	expiry      time.Time
	userID      int
}

// New returns a Client for the given offline refresh token.
func New(refreshToken string) *Client {
	return &Client{
		RefreshToken: refreshToken,
		BaseURL:      DefaultBaseURL,
		TokenURL:     DefaultTokenURL,
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// APIError carries a non-2xx SCP response.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("scp api error (http %d): %s", e.Status, e.Message)
	}
	return fmt.Sprintf("scp api error (http %d)", e.Status)
}

// ensureToken returns a valid access token, refreshing it when needed. A 30s
// safety margin avoids using a token that expires mid-request.
func (c *Client) ensureToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && time.Now().Before(c.expiry) {
		return c.accessToken, nil
	}

	form := url.Values{
		"client_id":     {clientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.RefreshToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token refresh failed (http %d): %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return "", fmt.Errorf("token refresh returned no access_token")
	}
	c.accessToken = tok.AccessToken
	c.expiry = time.Now().Add(time.Duration(tok.ExpiresIn)*time.Second - 30*time.Second)
	return c.accessToken, nil
}

// do performs an authenticated request. body may be nil. On success it returns
// the raw response body (which may be empty for 204). out, if non-nil, is JSON
// decoded from the body when there is one.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	token, err := c.ensureToken(ctx)
	if err != nil {
		return err
	}

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read %s %s response: %w", method, path, err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		apiErr := &APIError{Status: res.StatusCode}
		var parsed struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Errors  []struct {
				Field   string `json:"field"`
				Message string `json:"message"`
			} `json:"errors"`
		}
		if json.Unmarshal(raw, &parsed) == nil {
			apiErr.Code = parsed.Code
			apiErr.Message = parsed.Message
			// A ValidationError names the offending fields only in errors.
			for _, fe := range parsed.Errors {
				detail := fe.Message
				if fe.Field != "" {
					detail = fe.Field + ": " + detail
				}
				apiErr.Message += "; " + detail
			}
		}
		if apiErr.Message == "" {
			apiErr.Message = strings.TrimSpace(string(raw))
		}
		return apiErr
	}

	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode %s %s response: %w", method, path, err)
		}
	}
	return nil
}
