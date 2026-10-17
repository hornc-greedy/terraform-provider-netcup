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
)

// newTestClient wires a Client to an httptest server. The handler routes /token
// for the OIDC exchange and everything else as API paths.
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
		if method == "GET" && path == "/servers" {
			return 200, `[{"id":1,"name":"v1","hostname":"h","nickname":"n","disabled":false}]`
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
	if err := c.DeleteReverseDNS(context.Background(), "2a03:4000:47:584::1"); err != nil {
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
