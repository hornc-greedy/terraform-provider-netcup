// SPDX-License-Identifier: MPL-2.0

// Package clouddns is a client for the DNS operations of the netcup API at
// api.netcup.com. It has its own host and authenticates with a bearer API key.
package clouddns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBaseURL is the host of the netcup API.
	DefaultBaseURL = "https://api.netcup.com"
	// maxAttempts bounds how often one request is sent.
	maxAttempts = 3
	// activeState is the assumed state of the served revision. The specification
	// types the state as a free string.
	activeState = "active"
)

// Client talks to the netcup API.
type Client struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	// RetryDelay is the wait before the first retry and doubles for each further
	// one.
	RetryDelay time.Duration

	mu       sync.Mutex
	domainMu map[int64]*sync.Mutex
}

// New returns a Client of the given host.
func New(baseURL, apiKey string) *Client {
	return &Client{
		APIKey:     apiKey,
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		RetryDelay: time.Second,
	}
}

// Message is one entry of the errors of a response.
type Message struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// APIError is a failed response of the netcup API.
type APIError struct {
	Status   int
	Messages []Message
}

func (e *APIError) Error() string {
	details := make([]string, 0, len(e.Messages))
	for _, m := range e.Messages {
		details = append(details, m.Code+": "+m.Message)
	}
	return fmt.Sprintf("netcup api error (http %d): %s", e.Status, strings.Join(details, "; "))
}

// IsNotFound reports whether err is an *APIError with HTTP 404.
func IsNotFound(err error) bool {
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		return apiErr.Status == http.StatusNotFound
	}
	return false
}

// pagination is the part of meta that list operations return.
type pagination struct {
	Page     int `json:"page"`
	LastPage int `json:"lastPage"`
}

// envelope wraps every response. nextPage points at the current page on the last
// page, so only lastPage marks the end of a list.
type envelope struct {
	Errors []Message `json:"errors"`
	Meta   *struct {
		Pagination *pagination `json:"pagination"`
	} `json:"meta"`
	Result json.RawMessage `json:"result"`
}

// do sends a request and returns the decoded envelope.
func (c *Client) do(ctx context.Context, method, path string, body any) (*envelope, error) {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = b
	}
	delay := c.RetryDelay
	for attempt := 1; ; attempt++ {
		status, raw, err := c.request(ctx, method, path, payload)
		if attempt < maxAttempts && retryable(method, status, err) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
			continue
		}
		if err != nil {
			return nil, err
		}
		return decodeEnvelope(method, path, status, raw)
	}
}

// retryable reports whether sending the request again has no further effect.
func retryable(method string, status int, err error) bool {
	idempotent := method == http.MethodGet || method == http.MethodDelete
	return idempotent && (err != nil || status >= http.StatusInternalServerError)
}

func (c *Client) request(ctx context.Context, method, path string, payload []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read %s %s response: %w", method, path, err)
	}
	return res.StatusCode, raw, nil
}

func decodeEnvelope(method, path string, status int, raw []byte) (*envelope, error) {
	var env envelope
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &env); err != nil {
			if status < 200 || status >= 300 {
				return nil, &APIError{Status: status, Messages: []Message{{Message: strings.TrimSpace(string(raw))}}}
			}
			return nil, fmt.Errorf("decode %s %s response: %w", method, path, err)
		}
	}
	if status < 200 || status >= 300 {
		return nil, &APIError{Status: status, Messages: env.Errors}
	}
	return &env, nil
}

// get reads a single result into out.
func (c *Client) get(ctx context.Context, path string, out any) error {
	env, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(env.Result, out)
}

// listAll reads every page of a list operation. Not every operation declares the
// page parameter, so the first request does not send it.
func listAll[T any](ctx context.Context, c *Client, path string, query url.Values) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		values := url.Values{}
		maps.Copy(values, query)
		if page > 1 {
			values.Set("page", strconv.Itoa(page))
		}
		target := path
		if len(values) > 0 {
			target += "?" + values.Encode()
		}
		env, err := c.do(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		var items []T
		if err := json.Unmarshal(env.Result, &items); err != nil {
			return nil, fmt.Errorf("decode %s response: %w", path, err)
		}
		all = append(all, items...)
		if env.Meta == nil || env.Meta.Pagination == nil || page >= env.Meta.Pagination.LastPage {
			return all, nil
		}
	}
}

// Domain mirrors the Domain schema.
type Domain struct {
	ID           int64  `json:"id"`
	FQDN         string `json:"fqdn"`
	IsDNSManaged bool   `json:"isDnsManaged"`
}

// Revision mirrors the DomainDnsRevision schema.
type Revision struct {
	Identifier string `json:"identifier"`
	CreatedAt  string `json:"createdAt"`
	ModifiedAt string `json:"modifiedAt"`
	State      string `json:"state"`
	Serial     *int64 `json:"serial"`
}

// Record mirrors the DomainDnsRecord schema. TTL may be null.
type Record struct {
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	RData      string `json:"rdata"`
	TTL        *int64 `json:"ttl"`
	Immutable  bool   `json:"immutable"`
}

// RecordInput mirrors the DnsRecordInput schema.
type RecordInput struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	RData string `json:"rdata"`
	TTL   int64  `json:"ttl"`
}

// Challenge mirrors the DomainAcmeChallenge schema.
type Challenge struct {
	Scope  string `json:"scope"`
	FQDN   string `json:"fqdn"`
	Value  string `json:"value"`
	Status string `json:"status"`
}

func domainPath(id int64) string {
	return "/v1/domain/" + strconv.FormatInt(id, 10)
}

// RecordTypes returns the record types netcup supports.
func (c *Client) RecordTypes(ctx context.Context) ([]string, error) {
	var out struct {
		Type []string `json:"type"`
	}
	if err := c.get(ctx, "/v1/dns/recordType", &out); err != nil {
		return nil, err
	}
	return out.Type, nil
}

// Domains returns all domains of the API key.
func (c *Client) Domains(ctx context.Context) ([]Domain, error) {
	return listAll[Domain](ctx, c, "/v1/domain", nil)
}

// DomainByFQDN returns the domain with the given name, or nil if there is none.
func (c *Client) DomainByFQDN(ctx context.Context, fqdn string) (*Domain, error) {
	domains, err := listAll[Domain](ctx, c, "/v1/domain", url.Values{"fqdn": {fqdn}})
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	for i := range domains {
		if domains[i].FQDN == fqdn {
			return &domains[i], nil
		}
	}
	return nil, nil
}

// Revisions returns all revisions of a domain.
func (c *Client) Revisions(ctx context.Context, domainID int64) ([]Revision, error) {
	return listAll[Revision](ctx, c, domainPath(domainID)+"/dns/revision", nil)
}

// Revision returns one revision of a domain.
func (c *Client) Revision(ctx context.Context, domainID int64, identifier string) (*Revision, error) {
	var out Revision
	if err := c.get(ctx, domainPath(domainID)+"/dns/revision/"+url.PathEscape(identifier), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ActiveRevision returns the revision the domain currently serves.
func (c *Client) ActiveRevision(ctx context.Context, domainID int64) (*Revision, error) {
	revisions, err := c.Revisions(ctx, domainID)
	if err != nil {
		if isNoResult(err) {
			return nil, fmt.Errorf("domain %d does not run on CloudDNS: %w", domainID, err)
		}
		return nil, err
	}
	for i := range revisions {
		if revisions[i].State == activeState {
			return &revisions[i], nil
		}
	}
	return nil, fmt.Errorf("domain %d has no revision in state %s", domainID, activeState)
}

// isNoResult reports netcup's answer for a domain that does not run on CloudDNS.
func isNoResult(err error) bool {
	apiErr, ok := errors.AsType[*APIError](err)
	return ok && apiErr.Status == http.StatusBadRequest && slices.ContainsFunc(apiErr.Messages, func(m Message) bool {
		return m.Message == "No result was found."
	})
}

// Records returns the records of a revision.
func (c *Client) Records(ctx context.Context, domainID int64, revision string) ([]Record, error) {
	return listAll[Record](ctx, c, domainPath(domainID)+"/dns/revision/"+url.PathEscape(revision)+"/record", nil)
}

// RecordByIdentifier returns one record of a revision.
func (c *Client) RecordByIdentifier(ctx context.Context, domainID int64, revision, identifier string) (*Record, error) {
	var out Record
	path := domainPath(domainID) + "/dns/revision/" + url.PathEscape(revision) + "/record/" + url.PathEscape(identifier)
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// changeset is the body of POST /v1/domain/{domainId}/changeset.
type changeset struct {
	AutoCommit bool          `json:"autoCommit"`
	Strict     bool          `json:"strict"`
	Create     []RecordInput `json:"create,omitempty"`
	Delete     []RecordInput `json:"delete,omitempty"`
}

// ChangeRecords sends one committed changeset. Each one creates a revision from
// the active one, so changes to a domain run one at a time.
func (c *Client) ChangeRecords(ctx context.Context, domainID int64, create, remove []RecordInput) error {
	unlock := c.lockDomain(domainID)
	defer unlock()
	_, err := c.do(ctx, http.MethodPost, domainPath(domainID)+"/changeset", changeset{
		AutoCommit: true, Strict: true, Create: create, Delete: remove,
	})
	return err
}

func (c *Client) lockDomain(domainID int64) func() {
	c.mu.Lock()
	if c.domainMu == nil {
		c.domainMu = map[int64]*sync.Mutex{}
	}
	mu, ok := c.domainMu[domainID]
	if !ok {
		mu = &sync.Mutex{}
		c.domainMu[domainID] = mu
	}
	c.mu.Unlock()
	mu.Lock()
	return mu.Unlock
}

// Challenges returns the ACME challenges of a domain.
func (c *Client) Challenges(ctx context.Context, domainID int64) ([]Challenge, error) {
	var out []Challenge
	if err := c.get(ctx, domainPath(domainID)+"/acme/challenge", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func challengePath(domainID int64, scope, value string) string {
	return domainPath(domainID) + "/acme/challenge/" + url.PathEscape(scope) + "/" + url.PathEscape(value)
}

// Challenge returns one ACME challenge, or nil if it does not exist.
func (c *Client) Challenge(ctx context.Context, domainID int64, scope, value string) (*Challenge, error) {
	var out []Challenge
	if err := c.get(ctx, challengePath(domainID, scope, value), &out); err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	for i := range out {
		if out[i].Scope == scope && out[i].Value == value {
			return &out[i], nil
		}
	}
	return nil, nil
}

// CreateChallenge creates an ACME challenge.
func (c *Client) CreateChallenge(ctx context.Context, domainID int64, scope, value string) (*Challenge, error) {
	env, err := c.do(ctx, http.MethodPost, domainPath(domainID)+"/acme/challenge", map[string]string{"scope": scope, "value": value})
	if err != nil {
		return nil, err
	}
	var out []Challenge
	if err := json.Unmarshal(env.Result, &out); err != nil {
		return nil, fmt.Errorf("decode challenge: %w", err)
	}
	for i := range out {
		if out[i].Scope == scope && out[i].Value == value {
			return &out[i], nil
		}
	}
	return nil, fmt.Errorf("created challenge %s/%s is not in the response", scope, value)
}

// DeleteChallenge removes an ACME challenge.
func (c *Client) DeleteChallenge(ctx context.Context, domainID int64, scope, value string) error {
	_, err := c.do(ctx, http.MethodDelete, challengePath(domainID, scope, value), nil)
	return err
}
