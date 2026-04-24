// SPDX-License-Identifier: MPL-2.0

// Package client is a small, dependency-free Go client for the netcup CCP
// (DomainWebservice) JSON API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// DefaultEndpoint is the JSON endpoint of the netcup CCP DomainWebservice.
const DefaultEndpoint = "https://ccp.netcup.net/run/webservice/servers/endpoint.php?JSON"

// Client talks to the netcup CCP API. It is safe for concurrent use: the API
// session is guarded by mu, and writes to a single DNS zone are serialized per
// domain via zoneMu (updateDnsRecords operates on a whole zone).
type Client struct {
	CustomerNumber string
	APIKey         string
	APIPassword    string
	Endpoint       string
	HTTPClient     *http.Client

	mu        sync.Mutex
	sessionID string
	zoneMu    sync.Map // domain -> *sync.Mutex
}

// New returns a Client with sane defaults.
func New(customerNumber, apiKey, apiPassword string) *Client {
	return &Client{
		CustomerNumber: customerNumber,
		APIKey:         apiKey,
		APIPassword:    apiPassword,
		Endpoint:       DefaultEndpoint,
		HTTPClient:     &http.Client{Timeout: 30 * time.Second},
	}
}

// do performs one CCP call and returns the raw responsedata on success. On any
// non-success status it returns an *APIError without touching responsedata,
// because netcup returns responsedata as a plain string on error, which would
// fail to decode into the caller's struct.
func (c *Client) do(ctx context.Context, action string, param any) (json.RawMessage, error) {
	body, err := json.Marshal(apiRequest{Action: action, Param: param})
	if err != nil {
		return nil, fmt.Errorf("encode %s request: %w", action, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var env apiResponse
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", action, err)
	}

	if env.Status != "success" {
		return nil, &APIError{
			Action:       action,
			Status:       env.Status,
			StatusCode:   env.StatusCode,
			ShortMessage: env.ShortMessage,
			LongMessage:  env.LongMessage,
		}
	}
	return env.ResponseData, nil
}

// ensureSession logs in once and caches the session id. netcup sessions expire
// after a short idle period; for the lifetime of a single Terraform run that is
// not a concern, so there is deliberately no logout/refresh dance here.
func (c *Client) ensureSession(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sessionID != "" {
		return nil
	}
	raw, err := c.do(ctx, "login", map[string]any{
		"customernumber": c.CustomerNumber,
		"apikey":         c.APIKey,
		"apipassword":    c.APIPassword,
	})
	if err != nil {
		return err
	}
	var data struct {
		APISessionID string `json:"apisessionid"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("decode login response: %w", err)
	}
	c.sessionID = data.APISessionID
	return nil
}

func (c *Client) session() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sessionID
}

// Logout ends the API session. It is rarely needed (sessions expire on their
// own) but provided for completeness.
func (c *Client) Logout(ctx context.Context) error {
	c.mu.Lock()
	sid := c.sessionID
	c.sessionID = ""
	c.mu.Unlock()
	if sid == "" {
		return nil
	}
	_, err := c.do(ctx, "logout", map[string]any{
		"customernumber": c.CustomerNumber,
		"apikey":         c.APIKey,
		"apisessionid":   sid,
	})
	return err
}

// zoneLock serializes mutating calls against a single zone and returns the
// unlock function.
func (c *Client) zoneLock(domain string) func() {
	m, _ := c.zoneMu.LoadOrStore(domain, &sync.Mutex{})
	mu, ok := m.(*sync.Mutex)
	if !ok {
		// Unreachable: only *sync.Mutex values are ever stored in zoneMu.
		panic("netcup: zoneMu held a non-*sync.Mutex value")
	}
	mu.Lock()
	return mu.Unlock
}
