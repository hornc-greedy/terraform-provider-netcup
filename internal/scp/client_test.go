// SPDX-License-Identifier: MPL-2.0

package scp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient returns a Client for an httptest server. The handler routes
// /token for the OIDC exchange and everything else as API paths.
func newTestClient(t *testing.T, api func(method, path string, body []byte) (int, string)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, `{"access_token":"AT","expires_in":300,"token_type":"Bearer"}`)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer AT" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":"401","message":"missing bearer"}`)
			return
		}
		status, resp := api(r.Method, r.URL.Path+ifQuery(r.URL.RawQuery), body)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)

	c := New("refresh-token")
	c.BaseURL = srv.URL
	c.TokenURL = srv.URL + "/token"
	c.RetryDelay = time.Millisecond
	return c
}

func ifQuery(q string) string {
	if q == "" {
		return ""
	}
	return "?" + q
}

func TestTokenRefreshAndAuth(t *testing.T) {
	c := newTestClient(t, func(method, path string, _ []byte) (int, string) {
		switch {
		case method == "GET" && path == "/servers?limit=100&offset=0":
			return 200, `[{"id":1,"name":"v1","hostname":"h","nickname":"n","disabled":false}]`
		case method == "GET" && path == "/servers?limit=100&offset=1":
			return 200, `[]`
		}
		t.Fatalf("unexpected %s %s", method, path)
		return 500, ""
	})
	servers, err := c.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(servers) != 1 || servers[0].ID != 1 {
		t.Fatalf("unexpected servers: %+v", servers)
	}
	// Token must be cached: a second call still works without a token error.
	if _, err := c.ListServers(context.Background()); err != nil {
		t.Fatalf("second ListServers: %v", err)
	}
}

func TestGetReverseDNSv4(t *testing.T) {
	c := newTestClient(t, func(method, path string, _ []byte) (int, string) {
		if method == "GET" && path == "/rdns/ipv4/203.0.113.10" {
			return 200, `{"rdns":"mail.example.com"}`
		}
		t.Fatalf("unexpected %s %s", method, path)
		return 500, ""
	})
	got, err := c.GetReverseDNS(context.Background(), "203.0.113.10")
	if err != nil {
		t.Fatalf("GetReverseDNS: %v", err)
	}
	if got != "mail.example.com" {
		t.Fatalf("rdns = %q", got)
	}
}

func TestSetReverseDNSv4SendsBody(t *testing.T) {
	var captured string
	c := newTestClient(t, func(method, path string, body []byte) (int, string) {
		if method == "POST" && path == "/rdns/ipv4" {
			captured = string(body)
			return 200, ``
		}
		t.Fatalf("unexpected %s %s", method, path)
		return 500, ""
	})
	if err := c.SetReverseDNS(context.Background(), "203.0.113.10", "mail.example.com"); err != nil {
		t.Fatalf("SetReverseDNS: %v", err)
	}
	var sent setRdns
	if err := json.Unmarshal([]byte(captured), &sent); err != nil {
		t.Fatalf("body not json: %s", captured)
	}
	if sent.IP != "203.0.113.10" || sent.RDNS != "mail.example.com" {
		t.Fatalf("unexpected body: %+v", sent)
	}
}

func TestReverseDNSv6UsesV6Path(t *testing.T) {
	hit := ""
	c := newTestClient(t, func(method, path string, _ []byte) (int, string) {
		hit = method + " " + path
		return 204, ``
	})
	if err := c.DeleteReverseDNS(context.Background(), "2001:db8::1"); err != nil {
		t.Fatalf("DeleteReverseDNS: %v", err)
	}
	if !strings.HasPrefix(hit, "DELETE /rdns/ipv6/") {
		t.Fatalf("v6 path not used: %s", hit)
	}
}

func TestGetInterfacesShape(t *testing.T) {
	c := newTestClient(t, func(method, path string, _ []byte) (int, string) {
		if method == "GET" && strings.HasPrefix(path, "/servers/5/interfaces") {
			return 200, `[{"mac":"aa:bb","driver":"virtio","speedInMBits":2500,
				"ipv4Addresses":[{"id":1,"ip":"203.0.113.10","cidr":"203.0.113.10/22","gateway":"203.0.113.1","rdns":"mail.example.com","type":"IP","editable":false,"interfaceMac":"aa:bb"}],
				"ipv6Addresses":[{"id":2,"cidr":"2a03::/64","networkPrefix":"2a03::","gateway":"fe80::1","linkLocal":false,"rdns":{"2a03::1":"mail.example.com"},"type":"IP","editable":false,"interfaceMac":"aa:bb"}]}]`
		}
		t.Fatalf("unexpected %s %s", method, path)
		return 500, ""
	})
	ifaces, err := c.GetInterfaces(context.Background(), 5)
	if err != nil {
		t.Fatalf("GetInterfaces: %v", err)
	}
	if len(ifaces) != 1 {
		t.Fatalf("got %d interfaces", len(ifaces))
	}
	if ifaces[0].IPv4Addresses[0].RDNS != "mail.example.com" {
		t.Fatalf("v4 rdns: %+v", ifaces[0].IPv4Addresses[0])
	}
	if ifaces[0].IPv6Addresses[0].RDNS["2a03::1"] != "mail.example.com" {
		t.Fatalf("v6 rdns map: %+v", ifaces[0].IPv6Addresses[0])
	}
}

func TestGetServerReadsExportCount(t *testing.T) {
	c := newTestClient(t, func(method, path string, _ []byte) (int, string) {
		return 200, `{"id":5,"exportCount":3}`
	})
	s, err := c.GetServer(context.Background(), 5)
	if err != nil {
		t.Fatalf("GetServer: %v", err)
	}
	if s.ExportCount != 3 {
		t.Fatalf("ExportCount = %d, want 3", s.ExportCount)
	}
}

func TestRetriesIdempotentRequests(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		for _, status := range []int{500, 503} {
			calls := 0
			c := newTestClient(t, func(_, _ string, _ []byte) (int, string) {
				calls++
				if calls < 3 {
					return status, `{"message":"busy"}`
				}
				return 200, `{}`
			})
			if err := c.do(context.Background(), method, "/x", nil, nil); err != nil {
				t.Fatalf("%s %d: %v", method, status, err)
			}
			if calls != 3 {
				t.Fatalf("%s %d: calls = %d, want 3", method, status, calls)
			}
		}
	}
}

// dropConnectionServer closes the connection of the first dropped requests.
func dropConnectionServer(t *testing.T, dropped int) (*Client, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, `{"access_token":"AT","expires_in":300}`)
			return
		}
		calls++
		if calls <= dropped {
			conn, _, err := http.NewResponseController(w).Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			_ = conn.Close()
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)
	c := New("refresh-token")
	c.BaseURL = srv.URL
	c.TokenURL = srv.URL + "/token"
	c.RetryDelay = time.Millisecond
	return c, &calls
}

func TestRetriesTransportErrors(t *testing.T) {
	c, calls := dropConnectionServer(t, 1)
	if err := c.do(context.Background(), http.MethodGet, "/x", nil, nil); err != nil {
		t.Fatalf("GET: %v", err)
	}
	if *calls != 2 {
		t.Fatalf("calls = %d, want 2", *calls)
	}
}

func TestPostIsNotRetriedOnTransportError(t *testing.T) {
	c, calls := dropConnectionServer(t, 1)
	if err := c.do(context.Background(), http.MethodPost, "/x", struct{}{}, nil); err == nil {
		t.Fatal("expected error")
	}
	if *calls != 1 {
		t.Fatalf("calls = %d, want 1", *calls)
	}
}

func TestCancelStopsRetrying(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	c := newTestClient(t, func(_, _ string, _ []byte) (int, string) {
		calls++
		cancel()
		return 503, `{"message":"busy"}`
	})
	c.RetryDelay = time.Hour
	if err := c.do(ctx, http.MethodGet, "/x", nil, nil); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestTokenRenewalDoesNotUseUpRetries(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(_, _ string, _ []byte) (int, string) {
		calls++
		switch calls {
		case 1:
			return 401, ``
		case 2, 3:
			return 503, `{"message":"busy"}`
		}
		return 200, `{}`
	})
	if err := c.do(context.Background(), http.MethodGet, "/x", nil, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if calls != 4 {
		t.Fatalf("calls = %d, want 4", calls)
	}
}

func TestUserIDIsRetried(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(_, path string, _ []byte) (int, string) {
		calls++
		if calls == 1 {
			return 503, `{"message":"busy"}`
		}
		return 200, `{"id":"4242"}`
	})
	c.UserinfoURL = c.BaseURL + "/userinfo"
	id, err := c.UserID(context.Background())
	if err != nil || id != 4242 {
		t.Fatalf("UserID: %d %v", id, err)
	}
}

func TestRetriesStopAfterMaxAttempts(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(method, path string, _ []byte) (int, string) {
		calls++
		return 503, `{"message":"busy"}`
	})
	_, err := c.GetReverseDNS(context.Background(), "203.0.113.10")
	if apiErr, ok := err.(*APIError); !ok || apiErr.Status != 503 {
		t.Fatalf("err = %v, want 503", err)
	}
	if calls != maxAttempts {
		t.Fatalf("calls = %d, want %d", calls, maxAttempts)
	}
}

func TestPostIsNotRetriedOnServerError(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(method, path string, _ []byte) (int, string) {
		calls++
		return 500, `{"message":"boom"}`
	})
	if err := c.SetReverseDNS(context.Background(), "203.0.113.10", "mail.example.com"); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestTooManyRequestsIsNotRetried(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(_, _ string, _ []byte) (int, string) {
		calls++
		return 429, `{"message":"slow down"}`
	})
	if err := c.do(context.Background(), http.MethodGet, "/x", nil, nil); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestTokenRenewedAfterUnauthorized(t *testing.T) {
	tokens := 0
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			tokens++
			_, _ = io.WriteString(w, `{"access_token":"AT","expires_in":300}`)
			return
		}
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"rdns":"mail.example.com"}`)
	}))
	t.Cleanup(srv.Close)
	c := New("refresh-token")
	c.BaseURL = srv.URL
	c.TokenURL = srv.URL + "/token"
	if _, err := c.GetReverseDNS(context.Background(), "203.0.113.10"); err != nil {
		t.Fatalf("GetReverseDNS: %v", err)
	}
	if tokens != 2 || requests != 2 {
		t.Fatalf("tokens = %d, requests = %d, want 2 and 2", tokens, requests)
	}
}

func TestRepeatedUnauthorizedIsReported(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = io.WriteString(w, `{"access_token":"AT","expires_in":300}`)
			return
		}
		requests++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	c := New("refresh-token")
	c.BaseURL = srv.URL
	c.TokenURL = srv.URL + "/token"
	_, err := c.GetReverseDNS(context.Background(), "203.0.113.10")
	if apiErr, ok := err.(*APIError); !ok || apiErr.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v, want 401", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestListAllReadsEveryPage(t *testing.T) {
	var requested []string
	c := newTestClient(t, func(_, path string, _ []byte) (int, string) {
		requested = append(requested, path)
		switch {
		case strings.HasSuffix(path, "offset=0"):
			return 200, `[{"id":1},{"id":2}]`
		case strings.HasSuffix(path, "offset=2"):
			return 200, `[{"id":3}]`
		}
		return 200, `[]`
	})
	servers, err := c.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(servers) != 3 {
		t.Fatalf("got %d servers", len(servers))
	}
	if len(requested) != 3 || !strings.HasSuffix(requested[2], "offset=3") {
		t.Fatalf("requests: %v", requested)
	}
}

func TestNewForServerDerivesEndpoints(t *testing.T) {
	c := NewForServer("https://scp.example", "r")
	if c.BaseURL != "https://scp.example/scp-core/api/v1" {
		t.Fatalf("BaseURL = %q", c.BaseURL)
	}
	if c.TokenURL != "https://scp.example/realms/scp/protocol/openid-connect/token" {
		t.Fatalf("TokenURL = %q", c.TokenURL)
	}
	if c.UserinfoURL != "https://scp.example/realms/scp/protocol/openid-connect/userinfo" {
		t.Fatalf("UserinfoURL = %q", c.UserinfoURL)
	}
}

func TestAPIErrorParsed(t *testing.T) {
	c := newTestClient(t, func(method, path string, _ []byte) (int, string) {
		return 422, `{"code":"422","message":"Validation error."}`
	})
	_, err := c.GetReverseDNS(context.Background(), "203.0.113.10")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("type = %T, want *APIError", err)
	}
	if apiErr.Status != 422 || apiErr.Message != "Validation error." {
		t.Fatalf("unexpected: %+v", apiErr)
	}
}

func TestAPIErrorFieldErrors(t *testing.T) {
	c := newTestClient(t, func(method, path string, _ []byte) (int, string) {
		return 422, `{"code":null,"message":"Validation failed","errors":[{"field":null,"message":"Disk name cannot be blank"},{"field":"name","message":"must not be blank"}]}`
	})
	_, err := c.GetReverseDNS(context.Background(), "203.0.113.10")
	want := "scp api error (http 422): Validation failed; Disk name cannot be blank; name: must not be blank"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %s", err, want)
	}
}
