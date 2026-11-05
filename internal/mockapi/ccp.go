// SPDX-License-Identifier: MPL-2.0

package mockapi

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// The CCP has no machine readable specification, the mock follows its function
// list. netcup documents no status codes. Assumptions: 4013 for a wrong
// password, a record id or a bad number, 5029 for an unknown record, state
// "yes", and the objects in the update answers.
const (
	// CCPPath is the path of the mocked CCP endpoint.
	CCPPath = "/run/webservice/servers/endpoint.php"
	// CCPCustomer, CCPKey and CCPPassword are the accepted credentials.
	CCPCustomer = "12345"
	CCPKey      = "mock-ccp-key"
	CCPPassword = "mock-ccp-password"
	// CCPDomain is the only zone.
	CCPDomain = "example.com"

	ccpStatusOK       = 2000
	ccpStatusLogin    = 4013
	ccpStatusSession  = 4001
	ccpStatusRequest  = 4013
	ccpStatusNotFound = 5029
)

// ccpRecord mirrors the Dnsrecord object.
type ccpRecord struct {
	ID           string `json:"id"`
	Hostname     string `json:"hostname"`
	Type         string `json:"type"`
	Priority     string `json:"priority"`
	Destination  string `json:"destination"`
	DeleteRecord bool   `json:"deleterecord"`
	State        string `json:"state"`
}

// ccpZone mirrors the Dnszone object. netcup returns its numbers as strings.
type ccpZone struct {
	Name         string `json:"name"`
	TTL          string `json:"ttl"`
	Serial       string `json:"serial"`
	Refresh      string `json:"refresh"`
	Retry        string `json:"retry"`
	Expire       string `json:"expire"`
	DNSSECStatus bool   `json:"dnssecstatus"`
}

type ccpState struct {
	sessions map[string]bool
	nextID   int
	records  map[string]*ccpRecord
	zone     ccpZone
	logouts  int
	foreign  *ccpRecord
}

func newCCPState() *ccpState {
	return &ccpState{
		nextID:   1,
		sessions: map[string]bool{},
		records:  map[string]*ccpRecord{},
		zone:     ccpZone{Name: CCPDomain, TTL: "86400", Serial: "2026010100", Refresh: "28800", Retry: "7200", Expire: "1209600"},
	}
}

// ccpFunction is one function of the CCP with its required fields.
type ccpFunction struct {
	required []string
	run      func(param map[string]json.RawMessage) ccpAnswer
}

type ccpAnswer struct {
	status      int
	short, long string
	data        any
}

func ccpError(status int, short, long string) ccpAnswer {
	return ccpAnswer{status: status, short: short, long: long}
}

func ccpSuccess(message string, data any) ccpAnswer {
	return ccpAnswer{status: ccpStatusOK, short: message, long: message, data: data}
}

func (s *Server) ccpFunctions() map[string]ccpFunction {
	zone := []string{"domainname", "customernumber", "apikey", "apisessionid"}
	return map[string]ccpFunction{
		"login":            {[]string{"customernumber", "apikey", "apipassword"}, s.ccpLogin},
		"logout":           {[]string{"customernumber", "apikey", "apisessionid"}, s.ccpLogout},
		"infoDnsZone":      {zone, s.ccpInfoZone},
		"infoDnsRecords":   {zone, s.ccpInfoRecords},
		"updateDnsZone":    {append(slices.Clone(zone), "dnszone"), s.ccpUpdateZone},
		"updateDnsRecords": {append(slices.Clone(zone), "dnsrecordset"), s.ccpUpdateRecords},
	}
}

// ccpText reads a field. The client sends customernumber, a positiveInteger, as
// a string.
func ccpText(param map[string]json.RawMessage, name string) string {
	return strings.Trim(string(param[name]), `"`)
}

// serveCCP answers the JSON binding of the CCP, which reports errors with HTTP
// 200 and a string in responsedata.
func (s *Server) serveCCP(w http.ResponseWriter, r *http.Request, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method != http.MethodPost || !r.URL.Query().Has("JSON") || r.Header.Get("Content-Type") != "application/json" {
		s.t.Errorf("CCP request %s %s with Content-Type %q", r.Method, r.URL, r.Header.Get("Content-Type"))
	}
	var call struct {
		Action string                     `json:"action"`
		Param  map[string]json.RawMessage `json:"param"`
	}
	if err := json.Unmarshal(body, &call); err != nil {
		s.t.Errorf("CCP body %s: %v", body, err)
	}
	s.requests = append(s.requests, Request{Method: r.Method, Path: call.Action, Body: string(body), ContentType: r.Header.Get("Content-Type")})
	answer := s.ccpCall(call.Action, call.Param)
	response := map[string]any{
		"serverrequestid": "mock", "clientrequestid": "", "action": call.Action, "status": "success",
		"statuscode": answer.status, "shortmessage": answer.short, "longmessage": answer.long, "responsedata": answer.data,
	}
	if answer.status != ccpStatusOK {
		response["status"], response["responsedata"] = "error", ""
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) ccpCall(action string, param map[string]json.RawMessage) ccpAnswer {
	function, ok := s.ccpFunctions()[action]
	if !ok {
		s.t.Errorf("unknown CCP function %q", action)
		return ccpError(ccpStatusRequest, "Validation Error.", "unknown function")
	}
	for _, name := range function.required {
		if _, ok := param[name]; !ok {
			s.t.Errorf("%s without %s", action, name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(param)) {
		if !slices.Contains(function.required, name) && name != "clientrequestid" {
			s.t.Errorf("%s with undocumented field %s", action, name)
		}
	}
	if ccpText(param, "customernumber") != CCPCustomer || ccpText(param, "apikey") != CCPKey {
		return ccpError(ccpStatusLogin, "Validation Error.", "Invalid entry for field apikey")
	}
	if action != "login" && !s.ccp.sessions[ccpText(param, "apisessionid")] {
		return ccpError(ccpStatusSession, "Validation Error.", "The session id is not in a valid format.")
	}
	if slices.Contains(function.required, "domainname") && ccpText(param, "domainname") != CCPDomain {
		return ccpError(ccpStatusNotFound, "Getting DNS records failed", "Can not get DNS records for zone. Domain not found.")
	}
	return function.run(param)
}

func (s *Server) ccpLogin(param map[string]json.RawMessage) ccpAnswer {
	if ccpText(param, "apipassword") != CCPPassword {
		return ccpError(ccpStatusLogin, "Validation Error.", "Invalid entry for field apikey")
	}
	session := "mock-ccp-session-" + strconv.Itoa(s.ccp.nextID)
	s.ccp.nextID++
	s.ccp.sessions[session] = true
	return ccpSuccess("Login successful", map[string]string{"apisessionid": session})
}

func (s *Server) ccpLogout(param map[string]json.RawMessage) ccpAnswer {
	delete(s.ccp.sessions, ccpText(param, "apisessionid"))
	s.ccp.logouts++
	return ccpSuccess("Logout successful", nil)
}

func (s *Server) ccpInfoZone(map[string]json.RawMessage) ccpAnswer {
	return ccpSuccess("DNS zone found", s.ccp.zone)
}

// ccpRecordSet is the zone in the order of its ids.
func (s *Server) ccpRecordSet() map[string]any {
	ids := slices.SortedFunc(maps.Keys(s.ccp.records), func(a, b string) int {
		x, _ := strconv.Atoi(a)
		y, _ := strconv.Atoi(b)
		return x - y
	})
	records := make([]ccpRecord, 0, len(ids))
	for _, id := range ids {
		records = append(records, *s.ccp.records[id])
	}
	return map[string]any{"dnsrecords": records}
}

func (s *Server) ccpInfoRecords(map[string]json.RawMessage) ccpAnswer {
	if len(s.ccp.records) == 0 {
		return ccpError(ccpStatusNotFound, "Getting DNS records failed", "Can not get DNS records for zone. The zone does not "+CCPDomain+" contain any DNS records.")
	}
	return ccpSuccess("DNS records found", s.ccpRecordSet())
}

func (s *Server) ccpUpdateZone(param map[string]json.RawMessage) ccpAnswer {
	var zone ccpZone
	if err := json.Unmarshal(param["dnszone"], &zone); err != nil {
		s.t.Errorf("dnszone: %v", err)
	}
	for _, value := range []string{zone.TTL, zone.Refresh, zone.Retry, zone.Expire} {
		if n, err := strconv.Atoi(value); err != nil || n < 1 {
			return ccpError(ccpStatusRequest, "Validation Error.", "Validation Error.")
		}
	}
	if zone.Name != CCPDomain {
		s.t.Errorf("dnszone name %q", zone.Name)
	}
	serial, _ := strconv.Atoi(s.ccp.zone.Serial)
	zone.Serial = strconv.Itoa(serial + 1)
	s.ccp.zone = zone
	return ccpSuccess("DNS zone was changed", zone)
}

// ccpUpdateRecords applies a delta and returns the zone. Records it does not
// name stay, as the documentation calls the deletion of others optional.
func (s *Server) ccpUpdateRecords(param map[string]json.RawMessage) ccpAnswer {
	var set struct {
		DNSRecords []ccpRecord `json:"dnsrecords"`
	}
	if err := json.Unmarshal(param["dnsrecordset"], &set); err != nil {
		s.t.Errorf("dnsrecordset: %v", err)
	}
	if foreign := s.ccp.foreign; foreign != nil {
		foreign.ID = strconv.Itoa(s.ccp.nextID)
		s.ccp.nextID++
		s.ccp.records[foreign.ID] = foreign
		s.ccp.foreign = nil
	}
	for _, record := range set.DNSRecords {
		if record.Hostname == "" || record.Type == "" || record.Destination == "" || (record.Type == "MX" && record.Priority == "") {
			return ccpError(ccpStatusRequest, "Validation Error.", "Validation Error.")
		}
		if record.ID != "" && s.ccp.records[record.ID] == nil {
			return ccpError(ccpStatusNotFound, "Updating DNS records failed", "DNS record not found")
		}
		if record.DeleteRecord {
			delete(s.ccp.records, record.ID)
			continue
		}
		if record.ID == "" {
			record.ID = strconv.Itoa(s.ccp.nextID)
			s.ccp.nextID++
		}
		record.State = "yes"
		s.ccp.records[record.ID] = &record
	}
	return ccpSuccess("DNS records were changed", s.ccpRecordSet())
}

// AddCCPRecordOnNextWrite adds a record just before the next update, as another
// client could.
func (s *Server) AddCCPRecordOnNextWrite(hostname, kind, destination string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ccp.foreign = &ccpRecord{Hostname: hostname, Type: kind, Destination: destination, State: "yes"}
}

// EndCCPSession ends the session outside of the client.
func (s *Server) EndCCPSession() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.ccp.sessions)
}

// RemoveCCPRecord deletes a record outside of Terraform.
func (s *Server) RemoveCCPRecord(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.ccp.records, id)
}

// CCPRecordCount returns the number of records of the zone.
func (s *Server) CCPRecordCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ccp.records)
}

// CCPLogouts returns the number of logouts.
func (s *Server) CCPLogouts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ccp.logouts
}
