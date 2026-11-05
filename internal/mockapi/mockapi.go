// SPDX-License-Identifier: MPL-2.0

// Package mockapi is a stateful stand-in for the netcup APIs in tests. It checks
// requests and responses against the specifications in spec/ and states each
// departure from them.
package mockapi

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

//go:embed spec/*.json
var specs embed.FS

const (
	// RefreshToken is the only refresh token the mock accepts.
	RefreshToken = "mock-refresh-token"
	// UserID is the SCP user of the mock.
	UserID = 4242

	accessToken = "mock-access-token"
	scpPrefix   = "/scp-core"
	realmPrefix = "/realms/scp/protocol/openid-connect"
)

// Request is one API request the mock received.
type Request struct {
	Method      string
	Path        string
	Query       url.Values
	Body        string
	ContentType string
}

// Server is a running mock of the netcup APIs.
type Server struct {
	// URL is the host to configure the clients with.
	URL string

	t         testing.TB
	router    routers.Router
	dnsRouter routers.Router
	scpDoc    *openapi3.T
	handlers  map[string]handler

	mu       sync.Mutex
	requests []Request
	faults   []*fault
	scp      *scpState
	dns      *cloudDNSState
	ccp      *ccpState
}

type fault struct {
	method, path string
	prefix       bool
	status       int
	remaining    int
}

// call is one validated request.
type call struct {
	params map[string]string
	query  url.Values
	body   []byte
}

// decode reads the request body into v.
func (c *call) decode(v any) error {
	return json.Unmarshal(c.body, v)
}

// handler answers a call with Server.mu held. A nil value sends no body.
type handler func(*call) (int, any)

// New starts a mock that stops at the end of the test.
func New(t testing.TB) *Server {
	t.Helper()
	doc, err := loadSpec("spec/scp.json")
	if err != nil {
		t.Fatalf("load scp specification: %v", err)
	}
	doc.Servers = openapi3.Servers{{URL: "/"}}
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		t.Fatalf("route scp specification: %v", err)
	}
	dnsDoc, err := loadSpec("spec/netcup.json")
	if err != nil {
		t.Fatalf("load netcup specification: %v", err)
	}
	dnsDoc.Servers = openapi3.Servers{{URL: "/"}}
	dnsRouter, err := gorillamux.NewRouter(dnsDoc)
	if err != nil {
		t.Fatalf("route netcup specification: %v", err)
	}
	s := &Server{t: t, router: router, dnsRouter: dnsRouter, scpDoc: doc, scp: newSCPState(), ccp: newCCPState(), dns: newCloudDNSState()}
	s.handlers = s.scpHandlers()
	maps.Copy(s.handlers, s.cloudDNSHandlers())
	maps.Copy(s.handlers, s.objectHandlers("images"))
	maps.Copy(s.handlers, s.objectHandlers("isos"))
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	s.URL = srv.URL
	return s
}

func init() {
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC4122))
}

func loadSpec(name string) (*openapi3.T, error) {
	data, err := specs.ReadFile(name)
	if err != nil {
		return nil, err
	}
	doc, err := openapi3.NewLoader().LoadFromData(data)
	if err != nil {
		return nil, err
	}
	for _, schema := range doc.Components.Schemas {
		dropUnsupportedPatterns(schema, map[*openapi3.Schema]bool{})
	}
	return doc, nil
}

// dropUnsupportedPatterns removes patterns Go cannot compile, because some of
// netcup's patterns use lookahead.
func dropUnsupportedPatterns(ref *openapi3.SchemaRef, seen map[*openapi3.Schema]bool) {
	if ref == nil || ref.Value == nil || seen[ref.Value] {
		return
	}
	schema := ref.Value
	seen[schema] = true
	if _, err := regexp.Compile(schema.Pattern); err != nil {
		schema.Pattern = ""
	}
	dropUnsupportedPatterns(schema.Items, seen)
	for _, property := range schema.Properties {
		dropUnsupportedPatterns(property, seen)
	}
	for _, list := range []openapi3.SchemaRefs{schema.OneOf, schema.AnyOf, schema.AllOf} {
		for _, entry := range list {
			dropUnsupportedPatterns(entry, seen)
		}
	}
}

// FailNext makes the next times requests of method to path answer with status.
func (s *Server) FailNext(method, path string, status, times int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults = append(s.faults, &fault{method: method, path: path, status: status, remaining: times})
}

// FailNextPrefix fails the next request below a path of unknown end.
func (s *Server) FailNextPrefix(method, prefix string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults = append(s.faults, &fault{method: method, path: prefix, prefix: true, status: status, remaining: 1})
}

// Requests returns the API requests received so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// activeFault returns the status of a matching fault and consumes one use.
func (s *Server) activeFault(method, path string) (int, bool) {
	for _, f := range s.faults {
		if f.remaining > 0 && f.method == method && (f.path == path || f.prefix && strings.HasPrefix(path, f.path)) {
			f.remaining--
			return f.status, true
		}
	}
	return 0, false
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.t.Errorf("read request body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(r.URL.Path, realmPrefix) {
		s.serveRealm(w, r, body)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/") {
		s.serveCloudDNS(w, r, body)
		return
	}
	if r.URL.Path == CCPPath {
		s.serveCCP(w, r, body)
		return
	}
	if strings.HasPrefix(r.URL.Path, s3Prefix) {
		s.serveS3(w, r, len(body))
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+accessToken {
		writeJSON(w, http.StatusUnauthorized, responseError{Code: "error.unauthorized", Message: "missing bearer"})
		return
	}
	s.serveSpec(w, r, body, strings.TrimPrefix(r.URL.Path, scpPrefix), s.router)
}

// serveCloudDNS answers the netcup API. Its messages for a missing or wrong key
// are those of the real API.
func (s *Server) serveCloudDNS(w http.ResponseWriter, r *http.Request, body []byte) {
	switch r.Header.Get("Authorization") {
	case "Bearer " + APIKey:
		s.serveSpec(w, r, body, r.URL.Path, s.dnsRouter)
	case "":
		writeJSON(w, http.StatusBadRequest, cloudDNSError("authenticationError", "Authentication token was not passed."))
	default:
		writeJSON(w, http.StatusBadRequest, cloudDNSError("authenticationError", "Authentication token is not valid or has expired."))
	}
}

// serveRealm answers the OIDC endpoints, which no specification covers.
func (s *Server) serveRealm(w http.ResponseWriter, r *http.Request, body []byte) {
	switch r.URL.Path {
	case realmPrefix + "/token":
		form, err := url.ParseQuery(string(body))
		if err != nil || form.Get("grant_type") != "refresh_token" || form.Get("client_id") != "scp" || form.Get("refresh_token") != RefreshToken {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant", "error_description": "Offline user session not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"access_token": accessToken, "expires_in": 300})
	case realmPrefix + "/userinfo":
		if r.Header.Get("Authorization") != "Bearer "+accessToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// netcup returns the id as a string.
		writeJSON(w, http.StatusOK, map[string]string{"id": fmt.Sprint(UserID)})
	default:
		s.t.Errorf("unexpected realm request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *Server) serveSpec(w http.ResponseWriter, r *http.Request, body []byte, path string, router routers.Router) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, Request{
		Method: r.Method, Path: path, Query: r.URL.Query(), Body: string(body), ContentType: r.Header.Get("Content-Type"),
	})

	if status, ok := s.activeFault(r.Method, path); ok {
		writeJSON(w, status, responseError{Code: "error.mock", Message: "injected failure"})
		return
	}

	checked := r.Clone(context.Background())
	checked.URL.Path = path
	checked.RequestURI = ""
	checked.Body = io.NopCloser(bytes.NewReader(body))
	route, params, err := router.FindRoute(checked)
	if err != nil {
		s.t.Errorf("%s %s is not in the specification: %v", r.Method, path, err)
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	key := r.Method + " " + route.Path

	// netcup rejects a POST or PUT without a body when the Content-Type is
	// missing. The specification does not mention it.
	if router == s.router && len(body) == 0 && (r.Method == http.MethodPost || r.Method == http.MethodPut) && r.Header.Get("Content-Type") == "" {
		writeJSON(w, http.StatusBadRequest, responseError{Code: "error.content.type.invalid", Message: "The HTTP header Content-Type must be 'type/subtype'"})
		return
	}

	input := &openapi3filter.RequestValidationInput{
		Request: checked, PathParams: params, Route: route,
		Options: &openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
			// One body can match several schemas of this oneOf. patchServer checks
			// the body against the schema of its attribute instead.
			ExcludeRequestBody: key == "PATCH "+pathServer,
		},
	}
	if err := openapi3filter.ValidateRequest(context.Background(), input); err != nil {
		s.t.Errorf("%s %s violates the specification: %v", r.Method, path, err)
		writeJSON(w, http.StatusBadRequest, responseError{Code: "error.mock", Message: "request violates the specification"})
		return
	}

	h, ok := s.handlers[key]
	if !ok {
		s.t.Errorf("mock does not implement %s", key)
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	status, value := h(&call{params: params, query: r.URL.Query(), body: body})

	var payload []byte
	if value != nil {
		if payload, err = json.Marshal(value); err != nil {
			s.t.Errorf("encode %s response: %v", key, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
	}
	out := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: input, Status: status, Header: w.Header(),
		Body: io.NopCloser(bytes.NewReader(payload)),
		// A 204 has no content (RFC 9110), even where the spec lists some.
		Options: &openapi3filter.Options{
			IncludeResponseStatus: !undocumentedStatus(key, status),
			ExcludeResponseBody:   status == http.StatusNoContent,
		},
	}
	if err := openapi3filter.ValidateResponse(context.Background(), out); err != nil {
		s.t.Errorf("response of %s violates the specification: %v", key, err)
	}
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
