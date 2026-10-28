// SPDX-License-Identifier: MPL-2.0

package mockapi

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
)

const (
	pathDomain     = "/v1/domain"
	pathDomainItem = pathDomain + "/{domainId}"
	pathChallenges = pathDomainItem + "/acme/challenge"
	pathChallenge  = pathChallenges + "/{scope}/{value}"
	pathRevisions  = pathDomainItem + "/dns/revision"
	pathRevision   = pathRevisions + "/{revisionId}"
	pathRecords    = pathRevision + "/record"
	pathRecord     = pathRecords + "/{recordId}"

	// APIKey is the only API key the mock accepts.
	APIKey = "mock-api-key"
	// CloudDNSDomainID and CloudDNSDomain identify a domain on CloudDNS.
	CloudDNSDomainID = 2001
	CloudDNSDomain   = "example.com"
	// LegacyDomainID identifies a domain that does not run on CloudDNS.
	LegacyDomainID = 2002
	// perPage is the page size of the real API.
	perPage = 50
	// activeRevisionState is an assumption, the specification types the state as
	// a free string.
	activeRevisionState = "active"
	retiredRevision     = "retired"
)

type dnsRecord struct {
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	RData      string `json:"rdata"`
	TTL        *int64 `json:"ttl"`
	Immutable  bool   `json:"immutable"`
}

type dnsRevision struct {
	Identifier string `json:"identifier"`
	CreatedAt  string `json:"createdAt"`
	ModifiedAt string `json:"modifiedAt"`
	State      string `json:"state"`
	Serial     *int64 `json:"serial"`

	records []dnsRecord
}

type acmeChallenge struct {
	Scope  string `json:"scope"`
	FQDN   string `json:"fqdn"`
	Value  string `json:"value"`
	Status string `json:"status"`
}

type dnsInput struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	RData string `json:"rdata"`
	TTL   int64  `json:"ttl"`
}

// cloudDNSState is everything the CloudDNS part of the mock remembers.
type cloudDNSState struct {
	nextID     int
	normalize  bool
	domains    []map[string]any
	revisions  map[int][]*dnsRevision
	challenges map[int][]acmeChallenge
}

func newCloudDNSState() *cloudDNSState {
	serial := int64(2026110601)
	return &cloudDNSState{
		nextID: 9000,
		domains: []map[string]any{
			{"id": CloudDNSDomainID, "fqdn": CloudDNSDomain, "isDnsManaged": true},
			{"id": LegacyDomainID, "fqdn": "legacy.example", "isDnsManaged": false},
		},
		revisions: map[int][]*dnsRevision{
			CloudDNSDomainID: {{
				Identifier: "00000000-0000-4000-8000-000000000001", CreatedAt: "2026-11-06T10:00:00Z", ModifiedAt: "2026-11-06T10:00:00Z",
				State: activeRevisionState, Serial: &serial,
				records: []dnsRecord{{Identifier: "00000000-0000-4000-8000-000000000002", Name: "@", Type: "NS", RData: "root-dns.netcup.net.", Immutable: true}},
			}},
		},
		challenges: map[int][]acmeChallenge{},
	}
}

// identifier returns a new id in the uuid format of the specification.
func (s *cloudDNSState) identifier() string {
	s.nextID++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", s.nextID)
}

func (s *cloudDNSState) active(domainID int) *dnsRevision {
	for _, r := range s.revisions[domainID] {
		if r.State == activeRevisionState {
			return r
		}
	}
	return nil
}

// ok wraps a result in the envelope of a successful response.
func ok(status int, result, meta any) (int, any) {
	return status, map[string]any{"success": true, "errors": []any{}, "messages": []any{}, "meta": meta, "result": result}
}

// fail wraps an error in the envelope of a failed response.
func fail(status int, code, message string) (int, any) {
	return status, cloudDNSError(code, message)
}

func cloudDNSError(code, message string) map[string]any {
	return map[string]any{
		"success": false, "errors": []map[string]string{{"code": code, "message": message}},
		"messages": []any{}, "meta": nil, "result": nil,
	}
}

// noResult is netcup's answer to GET of the revisions of a domain that does not
// run on CloudDNS. The other operations answering with it are assumed to behave
// alike.
func noResult() (int, any) {
	return fail(http.StatusBadRequest, "unexpectedError", "No result was found.")
}

// paged returns one page of a list. nextPage points at the current page on the
// last page, as netcup returns it.
func paged[T any](items []T, c *call) (int, any) {
	number := 1
	if v, err := strconv.Atoi(c.query.Get("page")); err == nil {
		number = v
	}
	last := max((len(items)+perPage-1)/perPage, 1)
	start := min((number-1)*perPage, len(items))
	end := min(start+perPage, len(items))
	next := number
	if number < last {
		next = number + 1
	}
	meta := map[string]any{
		"sort": map[string]string{"field": "id", "direction": "asc"},
		"pagination": map[string]int{
			"page": number, "perPage": perPage, "totalEntries": len(items),
			"previousPage": max(number-1, 1), "nextPage": next, "lastPage": last,
		},
	}
	return ok(http.StatusOK, slices.Clone(items[start:end]), meta)
}

func domainID(c *call) int {
	id, _ := strconv.Atoi(c.params["domainId"])
	return id
}

func (s *Server) cloudDNSHandlers() map[string]handler {
	return map[string]handler{
		"GET /v1/dns/recordType": func(*call) (int, any) {
			return ok(http.StatusOK, map[string]any{"type": []string{"A", "AAAA", "CAA", "CNAME", "DS", "MX", "NS", "OPENPGPKEY", "SRV", "SMIMEA", "SSHFP", "TLSA", "TXT"}}, nil)
		},
		"GET " + pathDomain:                     s.listDomains,
		"GET " + pathChallenges:                 s.listChallenges,
		"POST " + pathChallenges:                s.createChallenge,
		"GET " + pathChallenge:                  s.getChallenge,
		"DELETE " + pathChallenge:               s.deleteChallenge,
		"POST " + pathDomainItem + "/changeset": s.changeset,
		"GET " + pathRevisions:                  s.listRevisions,
		"GET " + pathRevision:                   s.getRevision,
		"GET " + pathRecords:                    s.listRecords,
		"GET " + pathRecord:                     s.getRecord,
	}
}

func (s *Server) listDomains(c *call) (int, any) {
	out := []map[string]any{}
	for _, d := range s.dns.domains {
		if fqdn := c.query.Get("fqdn"); fqdn == "" || d["fqdn"] == fqdn {
			out = append(out, d)
		}
	}
	return paged(out, c)
}

func (s *Server) listRevisions(c *call) (int, any) {
	revisions, ok := s.dns.revisions[domainID(c)]
	if !ok {
		return noResult()
	}
	return paged(revisions, c)
}

func (s *Server) findRevision(c *call) *dnsRevision {
	for _, r := range s.dns.revisions[domainID(c)] {
		if r.Identifier == c.params["revisionId"] {
			return r
		}
	}
	return nil
}

func (s *Server) getRevision(c *call) (int, any) {
	r := s.findRevision(c)
	if r == nil {
		return noResult()
	}
	return ok(http.StatusOK, r, nil)
}

func (s *Server) listRecords(c *call) (int, any) {
	r := s.findRevision(c)
	if r == nil {
		return noResult()
	}
	return paged(r.records, c)
}

func (s *Server) getRecord(c *call) (int, any) {
	r := s.findRevision(c)
	if r == nil {
		return noResult()
	}
	for _, rec := range r.records {
		if rec.Identifier == c.params["recordId"] {
			return ok(http.StatusOK, rec, nil)
		}
	}
	return noResult()
}

func recordMatches(rec dnsRecord, in dnsInput) bool {
	return rec.Name == in.Name && rec.Type == in.Type && rec.RData == in.RData
}

// changeset replaces the active revision. The rules of strict, the order of delete
// and create, and the new identifiers are assumptions, the specification does not
// state them.
func (s *Server) changeset(c *call) (int, any) {
	id := domainID(c)
	current := s.dns.active(id)
	if current == nil {
		return noResult()
	}
	var body struct {
		AutoCommit bool       `json:"autoCommit"`
		Strict     bool       `json:"strict"`
		Create     []dnsInput `json:"create"`
		Delete     []dnsInput `json:"delete"`
	}
	if err := c.decode(&body); err != nil {
		return fail(http.StatusBadRequest, "invalidRequest", err.Error())
	}
	if !body.AutoCommit || !body.Strict {
		s.t.Errorf("changeset without autoCommit and strict: %s", c.body)
	}
	records := slices.Clone(current.records)
	for _, in := range body.Delete {
		i := slices.IndexFunc(records, func(rec dnsRecord) bool { return recordMatches(rec, in) })
		if i < 0 {
			return fail(http.StatusUnprocessableEntity, "recordNotFound", "record to delete does not exist")
		}
		if records[i].Immutable {
			return fail(http.StatusUnprocessableEntity, "recordImmutable", "record is immutable")
		}
		records = slices.Delete(records, i, i+1)
	}
	for _, in := range body.Create {
		if slices.ContainsFunc(records, func(rec dnsRecord) bool { return recordMatches(rec, in) }) {
			return fail(http.StatusUnprocessableEntity, "recordExists", "record to create already exists")
		}
		ttl := in.TTL
		rdata := in.RData
		if s.dns.normalize {
			rdata += "."
		}
		records = append(records, dnsRecord{Name: in.Name, Type: in.Type, RData: rdata, TTL: &ttl})
	}
	for i := range records {
		records[i].Identifier = s.dns.identifier()
	}
	serial := *current.Serial + 1
	current.State = retiredRevision
	next := &dnsRevision{
		Identifier: s.dns.identifier(), CreatedAt: "2026-11-06T11:00:00Z", ModifiedAt: "2026-11-06T11:00:00Z",
		State: activeRevisionState, Serial: &serial, records: records,
	}
	s.dns.revisions[id] = append(s.dns.revisions[id], next)
	return ok(http.StatusOK, next, nil)
}

func (s *Server) listChallenges(c *call) (int, any) {
	if _, ok := s.dns.revisions[domainID(c)]; !ok {
		return noResult()
	}
	return ok(http.StatusOK, append([]acmeChallenge{}, s.dns.challenges[domainID(c)]...), nil)
}

func (s *Server) createChallenge(c *call) (int, any) {
	id := domainID(c)
	if _, ok := s.dns.revisions[id]; !ok {
		return noResult()
	}
	var body struct {
		Scope string `json:"scope"`
		Value string `json:"value"`
	}
	if err := c.decode(&body); err != nil {
		return fail(http.StatusBadRequest, "invalidRequest", err.Error())
	}
	fqdn := "_acme-challenge." + CloudDNSDomain
	if body.Scope != "@" {
		fqdn = "_acme-challenge." + body.Scope + "." + CloudDNSDomain
	}
	challenge := acmeChallenge{Scope: body.Scope, FQDN: fqdn, Value: body.Value, Status: "pending"}
	s.dns.challenges[id] = append(s.dns.challenges[id], challenge)
	return ok(http.StatusCreated, []acmeChallenge{challenge}, nil)
}

func (s *Server) challengeIndex(c *call) int {
	return slices.IndexFunc(s.dns.challenges[domainID(c)], func(a acmeChallenge) bool {
		return a.Scope == c.params["scope"] && a.Value == c.params["value"]
	})
}

func (s *Server) getChallenge(c *call) (int, any) {
	i := s.challengeIndex(c)
	if i < 0 {
		return fail(http.StatusNotFound, "notFound", "challenge not found")
	}
	return ok(http.StatusOK, []acmeChallenge{s.dns.challenges[domainID(c)][i]}, nil)
}

func (s *Server) deleteChallenge(c *call) (int, any) {
	i := s.challengeIndex(c)
	if i < 0 {
		return fail(http.StatusNotFound, "notFound", "challenge not found")
	}
	id := domainID(c)
	s.dns.challenges[id] = slices.Delete(s.dns.challenges[id], i, i+1)
	return http.StatusNoContent, nil
}

// NormalizeRData makes the mock store rdata in another form than it was sent.
func (s *Server) NormalizeRData() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dns.normalize = true
}

// RecordRData returns the rdata of the active revision for a name and type.
func (s *Server) RecordRData(name, recordType string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, rec := range s.dns.active(CloudDNSDomainID).records {
		if rec.Name == name && rec.Type == recordType {
			out = append(out, rec.RData)
		}
	}
	return out
}

// ChallengeCount returns the number of ACME challenges of the CloudDNS domain.
func (s *Server) ChallengeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.dns.challenges[CloudDNSDomainID])
}

// SetRecordRData changes a record outside of Terraform.
func (s *Server) SetRecordRData(name, recordType, rdata string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	active := s.dns.active(CloudDNSDomainID)
	for i := range active.records {
		if active.records[i].Name == name && active.records[i].Type == recordType {
			active.records[i].RData = rdata
		}
	}
}
