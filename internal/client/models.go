// SPDX-License-Identifier: MPL-2.0

package client

import "encoding/json"

// apiRequest is the JSON envelope every CCP call is wrapped in.
type apiRequest struct {
	Action string `json:"action"`
	Param  any    `json:"param"`
}

// apiResponse is the JSON envelope every CCP call returns. responsedata is kept
// as RawMessage on purpose: on error netcup returns it as a *string* instead of
// an object, so it must only be decoded into a struct after status == "success".
type apiResponse struct {
	ServerRequestID string          `json:"serverrequestid"`
	ClientRequestID string          `json:"clientrequestid"`
	Action          string          `json:"action"`
	Status          string          `json:"status"`
	StatusCode      int             `json:"statuscode"`
	ShortMessage    string          `json:"shortmessage"`
	LongMessage     string          `json:"longmessage"`
	ResponseData    json.RawMessage `json:"responsedata"`
}

// DNSRecord mirrors one entry of a zone's dnsrecordset.
type DNSRecord struct {
	ID           string `json:"id,omitempty"`
	Hostname     string `json:"hostname"`
	Type         string `json:"type"`
	Priority     string `json:"priority,omitempty"`
	Destination  string `json:"destination"`
	DeleteRecord bool   `json:"deleterecord,omitempty"`
	State        string `json:"state,omitempty"`
}

// dnsRecordSet is the responsedata shape of info/updateDnsRecords and the
// dnsrecordset param of updateDnsRecords.
type dnsRecordSet struct {
	DNSRecords []DNSRecord `json:"dnsrecords"`
}
