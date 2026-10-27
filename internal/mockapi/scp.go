// SPDX-License-Identifier: MPL-2.0

package mockapi

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
)

const (
	pathServer        = "/api/v1/servers/{serverId}"
	pathInterfaces    = pathServer + "/interfaces"
	pathFirewall      = pathInterfaces + "/{mac}/firewall"
	pathSnapshots     = pathServer + "/snapshots"
	pathSnapshot      = pathSnapshots + "/{name}"
	pathUser          = "/api/v1/users/{userId}"
	pathPolicies      = pathUser + "/firewall-policies"
	pathPolicy        = pathPolicies + "/{id}"
	pathSSHKeys       = pathUser + "/ssh-keys"
	pathFailoverV4    = pathUser + "/failoverips/v4"
	pathFailoverV6    = pathUser + "/failoverips/v6"
	pathTask          = "/api/v1/tasks/{uuid}"
	createdAtNanos    = "2026-11-06T10:00:00.123456789Z"
	createdAtMicros   = "2026-11-06T10:00:00.123457Z"
	snapshotCreated   = "2026-11-06T10:00:00Z"
	defaultServerID   = 1001
	defaultMAC        = "aa:bb:cc:dd:ee:01"
	defaultIPv4       = "203.0.113.10"
	defaultIPv6       = "2001:db8::1"
	copiedPolicyID    = 900
	failoverV4ID      = 10
	failoverV6ID      = 20
	defaultFailoverV4 = "203.0.113.200"
)

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type taskInfo struct {
	UUID          string         `json:"uuid"`
	Name          string         `json:"name,omitempty"`
	State         string         `json:"state"`
	Message       *string        `json:"message,omitempty"`
	ResponseError *responseError `json:"responseError,omitempty"`
}

type liveInfo struct {
	State          string   `json:"state"`
	Autostart      bool     `json:"autostart"`
	UEFI           bool     `json:"uefi"`
	Bootorder      []string `json:"bootorder"`
	OSOptimization string   `json:"osOptimization"`
	Sockets        int      `json:"sockets"`
	CoresPerSocket int      `json:"coresPerSocket"`
	KeyboardLayout string   `json:"keyboardLayout"`
	BackupTime     string   `json:"backupTime"`
}

type server struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	Hostname           string `json:"hostname"`
	Nickname           string `json:"nickname"`
	Disabled           bool   `json:"disabled"`
	Architecture       string `json:"architecture"`
	MaxCPUCount        int    `json:"maxCpuCount"`
	ExportCount        int    `json:"exportCount"`
	RescueSystemActive bool   `json:"rescueSystemActive"`

	live liveInfo
}

type firewallRule struct {
	Description      *string  `json:"description,omitempty"`
	Direction        string   `json:"direction"`
	Protocol         string   `json:"protocol"`
	Action           string   `json:"action"`
	Sources          []string `json:"sources,omitempty"`
	SourcePorts      string   `json:"sourcePorts,omitempty"`
	Destinations     []string `json:"destinations,omitempty"`
	DestinationPorts string   `json:"destinationPorts,omitempty"`
}

type firewallPolicy struct {
	ID          int            `json:"id"`
	Name        string         `json:"name"`
	Description *string        `json:"description"`
	Rules       []firewallRule `json:"rules"`
}

type serverFirewall struct {
	copied, user []int
	active       bool
}

type snapshot struct {
	UUID         string   `json:"uuid"`
	Name         string   `json:"name"`
	Description  *string  `json:"description"`
	Disks        []string `json:"disks"`
	CreationTime string   `json:"creationTime"`
	State        string   `json:"state"`
	Online       bool     `json:"online"`
}

type sshKey struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	CreatedAt string `json:"createdAt"`
}

type failoverIP struct {
	ID       int
	Address  string
	ServerID int
}

// scpState is everything the SCP part of the mock remembers.
type scpState struct {
	nextID     int
	servers    map[int]*server
	firewalls  map[string]*serverFirewall
	snapshots  map[int]map[string]*snapshot
	policies   map[int]*firewallPolicy
	sshKeys    map[int]*sshKey
	rdns       map[string]string
	failoverV4 map[int]*failoverIP
	failoverV6 map[int]*failoverIP
	tasks      map[string]*taskInfo
	failedTask *taskInfo
}

func newSCPState() *scpState {
	return &scpState{
		nextID: 5000,
		servers: map[int]*server{
			defaultServerID: {
				ID: defaultServerID, Name: "v1001", Hostname: "mock.example.com", Nickname: "mock",
				Architecture: "AMD64", MaxCPUCount: 4, ExportCount: DefaultExportCount,
				live: liveInfo{
					State: "RUNNING", Autostart: false, Bootorder: []string{"HDD", "CDROM"}, OSOptimization: "LINUX",
					Sockets: 1, CoresPerSocket: 2, KeyboardLayout: "de", BackupTime: "00:00:00+00:00",
				},
			},
		},
		firewalls:  map[string]*serverFirewall{defaultMAC: {copied: []int{copiedPolicyID}, active: true}},
		snapshots:  map[int]map[string]*snapshot{defaultServerID: {}},
		policies:   map[int]*firewallPolicy{copiedPolicyID: {ID: copiedPolicyID, Name: "copied", Rules: []firewallRule{}}},
		sshKeys:    map[int]*sshKey{},
		rdns:       map[string]string{},
		failoverV4: map[int]*failoverIP{failoverV4ID: {ID: failoverV4ID, Address: defaultFailoverV4, ServerID: defaultServerID}},
		failoverV6: map[int]*failoverIP{failoverV6ID: {ID: failoverV6ID, Address: "2001:db8:1::", ServerID: 0}},
		tasks:      map[string]*taskInfo{},
	}
}

func (s *scpState) newID() int {
	s.nextID++
	return s.nextID
}

// startTask returns a pending task. Polling it reports FINISHED, or the failure
// set by FailTasks.
func (s *scpState) startTask(name string) *taskInfo {
	t := &taskInfo{UUID: fmt.Sprintf("task-%d", s.newID()), Name: name, State: "PENDING"}
	s.tasks[t.UUID] = t
	return t
}

// FailTasks makes every task end in state ERROR with the given error.
func (s *Server) FailTasks(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.failedTask = &taskInfo{State: "ERROR", ResponseError: &responseError{Code: "error.mock", Message: message}}
}

// DefaultServerID, DefaultMAC, DefaultIPv4 and the failover ids identify the
// resources every mock starts with.
const (
	DefaultServerID = defaultServerID
	DefaultMAC      = defaultMAC
	DefaultIPv4     = defaultIPv4
	FailoverV4ID    = failoverV4ID
	FailoverV6ID    = failoverV6ID
	CopiedPolicyID  = copiedPolicyID
	// DefaultExportCount is exportCount of the default server.
	DefaultExportCount = 2
)

// RemoveFailover makes a failover IP disappear outside of Terraform.
func (s *Server) RemoveFailover(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.scp.failoverV4, id)
	delete(s.scp.failoverV6, id)
}

// FirewallOf returns the copied and user policy ids and the active flag of an
// interface.
func (s *Server) FirewallOf(mac string) (copied, user []int, active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fw := s.scp.firewalls[mac]
	return slices.Clone(fw.copied), slices.Clone(fw.user), fw.active
}

func notFound() (int, any) {
	return http.StatusNotFound, responseError{Code: "error.notfound", Message: "Resource not found"}
}

func badRequest(message string) (int, any) {
	return http.StatusBadRequest, responseError{Code: "error.mock", Message: message}
}

// page applies limit and offset of a list endpoint.
func page[T any](items []T, c *call) []T {
	offset, _ := strconv.Atoi(c.query.Get("offset"))
	offset = min(offset, len(items))
	items = items[offset:]
	if limit, err := strconv.Atoi(c.query.Get("limit")); err == nil {
		items = items[:min(limit, len(items))]
	}
	return items
}

func sortedValues[K int | string, V any](m map[K]V) []V {
	out := make([]V, 0, len(m))
	for _, k := range slices.Sorted(maps.Keys(m)) {
		out = append(out, m[k])
	}
	return out
}

func (s *Server) scpServer(c *call) *server {
	id, _ := strconv.Atoi(c.params["serverId"])
	return s.scp.servers[id]
}

// checkUser fails the test unless the provider asks for the user it got from
// userinfo.
func (s *Server) checkUser(c *call) {
	if c.params["userId"] != strconv.Itoa(UserID) {
		s.t.Errorf("request for user %s, userinfo returned %d", c.params["userId"], UserID)
	}
}

// undocumentedStatus reports whether netcup answers key with status although
// the specification omits it.
func undocumentedStatus(key string, status int) bool {
	return key == "GET "+pathPolicy && status == http.StatusNotFound
}

func (s *Server) scpHandlers() map[string]handler {
	return map[string]handler{
		"GET /api/v1/maintenance": func(*call) (int, any) {
			return http.StatusOK, map[string]any{"startAt": nil, "finishAt": nil}
		},
		"GET " + pathTask: s.getTask,

		"POST /api/v1/rdns/ipv4":        s.setRDNS,
		"POST /api/v1/rdns/ipv6":        s.setRDNS,
		"GET /api/v1/rdns/ipv4/{ip}":    s.getRDNS,
		"GET /api/v1/rdns/ipv6/{ip}":    s.getRDNS,
		"DELETE /api/v1/rdns/ipv4/{ip}": s.deleteRDNS,
		"DELETE /api/v1/rdns/ipv6/{ip}": s.deleteRDNS,

		"GET /api/v1/servers":   s.listServers,
		"GET " + pathServer:     s.getServer,
		"PATCH " + pathServer:   s.patchServer,
		"GET " + pathInterfaces: s.listInterfaces,
		"GET " + pathFirewall:   s.getFirewall,
		"PUT " + pathFirewall:   s.putFirewall,

		"POST " + pathSnapshots:  s.createSnapshot,
		"GET " + pathSnapshot:    s.getSnapshot,
		"DELETE " + pathSnapshot: s.deleteSnapshot,

		"GET " + pathFailoverV4:             s.listFailoverV4,
		"GET " + pathFailoverV6:             s.listFailoverV6,
		"PATCH " + pathFailoverV4 + "/{id}": s.routeFailover(func() map[int]*failoverIP { return s.scp.failoverV4 }),
		"PATCH " + pathFailoverV6 + "/{id}": s.routeFailover(func() map[int]*failoverIP { return s.scp.failoverV6 }),

		"GET " + pathPolicies:  s.listPolicies,
		"POST " + pathPolicies: s.createPolicy,
		"GET " + pathPolicy:    s.getPolicy,
		"PUT " + pathPolicy:    s.putPolicy,
		"DELETE " + pathPolicy: s.deletePolicy,

		"GET " + pathSSHKeys:              s.listSSHKeys,
		"POST " + pathSSHKeys:             s.createSSHKey,
		"DELETE " + pathSSHKeys + "/{id}": s.deleteSSHKey,
	}
}

func (s *Server) getTask(c *call) (int, any) {
	t, ok := s.scp.tasks[c.params["uuid"]]
	if !ok {
		return notFound()
	}
	if f := s.scp.failedTask; f != nil {
		return http.StatusOK, &taskInfo{UUID: t.UUID, Name: t.Name, State: f.State, ResponseError: f.ResponseError}
	}
	return http.StatusOK, &taskInfo{UUID: t.UUID, Name: t.Name, State: "FINISHED"}
}

func (s *Server) setRDNS(c *call) (int, any) {
	var body struct {
		IP   string `json:"ip"`
		RDNS string `json:"rdns"`
	}
	if err := c.decode(&body); err != nil {
		return badRequest(err.Error())
	}
	s.scp.rdns[body.IP] = body.RDNS
	return http.StatusOK, map[string]any{}
}

func (s *Server) getRDNS(c *call) (int, any) {
	if name, ok := s.scp.rdns[c.params["ip"]]; ok {
		return http.StatusOK, map[string]any{"rdns": name}
	}
	return http.StatusOK, map[string]any{"rdns": nil}
}

func (s *Server) deleteRDNS(c *call) (int, any) {
	delete(s.scp.rdns, c.params["ip"])
	return http.StatusNoContent, nil
}

func (s *Server) listServers(c *call) (int, any) {
	type minimal struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Hostname string `json:"hostname"`
		Nickname string `json:"nickname"`
		Disabled bool   `json:"disabled"`
	}
	all := make([]minimal, 0, len(s.scp.servers))
	for _, v := range sortedValues(s.scp.servers) {
		all = append(all, minimal{v.ID, v.Name, v.Hostname, v.Nickname, v.Disabled})
	}
	return http.StatusOK, page(all, c)
}

func (s *Server) getServer(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	type detail struct {
		*server
		ServerLiveInfo liveInfo `json:"serverLiveInfo"`
	}
	return http.StatusOK, detail{server: v, ServerLiveInfo: v.live}
}

// serverPatchSchemas maps each attribute of PATCH server to its schema.
var serverPatchSchemas = map[string]string{
	"state":           "ServerStatePatch",
	"autostart":       "ServerAutostartPatch",
	"bootorder":       "ServerBootorderPatch",
	"os_optimization": "ServerOsOptimizationPatch",
	"cpuTopology":     "ServerCpuTopologyPatch",
	"uefi":            "ServerUEFIPatch",
	"hostname":        "ServerHostnamePatch",
	"nickname":        "ServerNicknamePatch",
	"keyboardLayout":  "ServerKeyboardLayoutPatch",
	"rootPassword":    "ServerSetRootPasswordPatch",
	"backupTime":      "ServerBackupTimePatch",
}

// patchServer enforces what the specification states in prose, one attribute per
// request. The status codes are the documented ones, so a missing server is 503.
func (s *Server) patchServer(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return http.StatusServiceUnavailable, responseError{Code: "error.mock", Message: "server not found"}
	}
	var patch map[string]json.RawMessage
	if err := c.decode(&patch); err != nil || len(patch) != 1 {
		s.t.Errorf("PATCH server takes one attribute per request: %s", c.body)
		return http.StatusServiceUnavailable, responseError{Code: "error.mock", Message: "only one schema at a time"}
	}
	for key, raw := range patch {
		if err := s.checkServerPatch(key, c.body); err != nil {
			s.t.Errorf("PATCH server %s violates the specification: %v", key, err)
			return http.StatusServiceUnavailable, responseError{Code: "error.mock", Message: err.Error()}
		}
		if err := applyServerPatch(v, key, raw); err != nil {
			s.t.Errorf("PATCH server %s: %v", key, err)
			return http.StatusServiceUnavailable, responseError{Code: "error.mock", Message: err.Error()}
		}
	}
	return http.StatusAccepted, s.scp.startTask("ServerPatch")
}

// checkServerPatch validates the body against the schema of its attribute.
func (s *Server) checkServerPatch(key string, body []byte) error {
	name, ok := serverPatchSchemas[key]
	if !ok {
		return fmt.Errorf("unknown attribute")
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return err
	}
	return s.scpDoc.Components.Schemas[name].Value.VisitJSON(value)
}

func applyServerPatch(v *server, key string, raw json.RawMessage) error {
	switch key {
	case "state":
		var state string
		if err := json.Unmarshal(raw, &state); err != nil {
			return err
		}
		v.live.State = map[string]string{"ON": "RUNNING", "OFF": "SHUTOFF", "SUSPENDED": "PAUSED"}[state]
		return nil
	case "hostname":
		return json.Unmarshal(raw, &v.Hostname)
	case "nickname":
		return json.Unmarshal(raw, &v.Nickname)
	case "autostart":
		return json.Unmarshal(raw, &v.live.Autostart)
	case "uefi":
		return json.Unmarshal(raw, &v.live.UEFI)
	case "bootorder":
		return json.Unmarshal(raw, &v.live.Bootorder)
	case "os_optimization":
		return json.Unmarshal(raw, &v.live.OSOptimization)
	case "keyboardLayout":
		return json.Unmarshal(raw, &v.live.KeyboardLayout)
	case "backupTime":
		return json.Unmarshal(raw, &v.live.BackupTime)
	case "rootPassword":
		return nil
	case "cpuTopology":
		var topology struct {
			SocketCount         int `json:"socketCount"`
			CoresPerSocketCount int `json:"coresPerSocketCount"`
		}
		if err := json.Unmarshal(raw, &topology); err != nil {
			return err
		}
		v.live.Sockets, v.live.CoresPerSocket = topology.SocketCount, topology.CoresPerSocketCount
		return nil
	}
	return fmt.Errorf("mock does not implement attribute")
}

func (s *Server) listInterfaces(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	// netcup returns {} for an IPv6 address without PTR.
	ipv6RDNS := map[string]string{}
	if name, ok := s.scp.rdns[defaultIPv6]; ok {
		ipv6RDNS[defaultIPv6] = name
	}
	var ipv4RDNS any
	if name, ok := s.scp.rdns[defaultIPv4]; ok {
		ipv4RDNS = name
	}
	return http.StatusOK, []map[string]any{{
		"mac": defaultMAC, "driver": "virtio", "speedInMBits": 2500,
		"ipv4Addresses": []map[string]any{{
			"id": 1, "interfaceMac": defaultMAC, "ip": defaultIPv4, "cidr": defaultIPv4 + "/22", "gateway": "203.0.113.1",
			"rdns": ipv4RDNS, "editable": false,
		}},
		"ipv6Addresses": []map[string]any{{
			"id": 2, "interfaceMac": defaultMAC, "networkPrefix": "2001:db8::", "cidr": "2001:db8::/64", "gateway": "fe80::1",
			"linkLocal": false, "rdns": ipv6RDNS, "editable": false,
		}},
	}}
}

func (s *Server) policiesByID(ids []int) []firewallPolicy {
	out := make([]firewallPolicy, 0, len(ids))
	for _, id := range ids {
		if p, ok := s.scp.policies[id]; ok {
			out = append(out, *p)
		}
	}
	return out
}

func (s *Server) getFirewall(c *call) (int, any) {
	if s.scpServer(c) == nil {
		return notFound()
	}
	fw, ok := s.scp.firewalls[c.params["mac"]]
	if !ok {
		return notFound()
	}
	return http.StatusOK, map[string]any{
		"copiedPolicies": s.policiesByID(fw.copied), "userPolicies": s.policiesByID(fw.user),
		"consistent": true, "active": fw.active,
	}
}

func (s *Server) putFirewall(c *call) (int, any) {
	if s.scpServer(c) == nil {
		return notFound()
	}
	fw, ok := s.scp.firewalls[c.params["mac"]]
	if !ok {
		return notFound()
	}
	type identifier struct {
		ID int `json:"id"`
	}
	var body struct {
		CopiedPolicies []identifier `json:"copiedPolicies"`
		UserPolicies   []identifier `json:"userPolicies"`
		Active         *bool        `json:"active"`
	}
	if err := c.decode(&body); err != nil {
		return badRequest(err.Error())
	}
	fw.copied, fw.user = nil, nil
	for _, p := range body.CopiedPolicies {
		fw.copied = append(fw.copied, p.ID)
	}
	for _, p := range body.UserPolicies {
		fw.user = append(fw.user, p.ID)
	}
	// The specification states that a missing value activates the firewall.
	fw.active = body.Active == nil || *body.Active
	return http.StatusAccepted, s.scp.startTask("ServerFirewallSave")
}

func (s *Server) createSnapshot(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	var body struct {
		Name           string  `json:"name"`
		Description    *string `json:"description"`
		DiskName       *string `json:"diskName"`
		OnlineSnapshot bool    `json:"onlineSnapshot"`
	}
	if err := c.decode(&body); err != nil {
		return badRequest(err.Error())
	}
	if !body.OnlineSnapshot && (body.DiskName == nil || *body.DiskName == "") {
		return badRequest("Disk name cannot be blank")
	}
	disks := []string{"vda"}
	if body.DiskName != nil {
		disks = []string{*body.DiskName}
	}
	if _, exists := s.scp.snapshots[v.ID][body.Name]; exists {
		return badRequest("snapshot already exists")
	}
	s.scp.snapshots[v.ID][body.Name] = &snapshot{
		UUID: fmt.Sprintf("snap-%d", s.scp.newID()), Name: body.Name, Description: body.Description,
		Disks: disks, CreationTime: snapshotCreated, State: "SHUTOFF", Online: body.OnlineSnapshot,
	}
	return http.StatusAccepted, s.scp.startTask("ServerSnapshotCreate")
}

func (s *Server) getSnapshot(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	snap, ok := s.scp.snapshots[v.ID][c.params["name"]]
	if !ok {
		return notFound()
	}
	return http.StatusOK, snap
}

func (s *Server) deleteSnapshot(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	if _, ok := s.scp.snapshots[v.ID][c.params["name"]]; !ok {
		return notFound()
	}
	delete(s.scp.snapshots[v.ID], c.params["name"])
	return http.StatusAccepted, s.scp.startTask("ServerSnapshotDelete")
}

// withServer adds the server a failover IP is routed to. The specification does
// not allow null, so an unrouted IP has no server field.
func withServer(ip map[string]any, serverID int) map[string]any {
	if serverID != 0 {
		ip["server"] = map[string]any{"id": serverID, "name": fmt.Sprintf("v%d", serverID)}
	}
	return ip
}

func (s *Server) listFailoverV4(c *call) (int, any) {
	s.checkUser(c)
	out := []map[string]any{}
	for _, ip := range sortedValues(s.scp.failoverV4) {
		out = append(out, withServer(map[string]any{"id": ip.ID, "ip": ip.Address, "cidrSuffix": 32, "editable": true}, ip.ServerID))
	}
	return http.StatusOK, out
}

func (s *Server) listFailoverV6(c *call) (int, any) {
	s.checkUser(c)
	out := []map[string]any{}
	for _, ip := range sortedValues(s.scp.failoverV6) {
		out = append(out, withServer(map[string]any{"id": ip.ID, "networkPrefix": ip.Address, "networkPrefixLength": 64, "editable": true}, ip.ServerID))
	}
	return http.StatusOK, out
}

func (s *Server) routeFailover(ips func() map[int]*failoverIP) handler {
	return func(c *call) (int, any) {
		id, _ := strconv.Atoi(c.params["id"])
		s.checkUser(c)
		ip, ok := ips()[id]
		if !ok {
			return notFound()
		}
		var body struct {
			ServerID int `json:"serverId"`
		}
		if err := c.decode(&body); err != nil {
			return badRequest(err.Error())
		}
		if _, ok := s.scp.servers[body.ServerID]; !ok {
			return notFound()
		}
		ip.ServerID = body.ServerID
		return http.StatusAccepted, s.scp.startTask("FailoverRoute")
	}
}

func (s *Server) listPolicies(c *call) (int, any) {
	s.checkUser(c)
	out := []firewallPolicy{}
	for _, id := range slices.Sorted(maps.Keys(s.scp.policies)) {
		if id != copiedPolicyID {
			out = append(out, returnedPolicy(s.scp.policies[id]))
		}
	}
	return http.StatusOK, page(out, c)
}

// returnedPolicy reverses the addresses, because netcup returns them in another
// order than they were sent.
func returnedPolicy(p *firewallPolicy) firewallPolicy {
	out := *p
	out.Rules = make([]firewallRule, len(p.Rules))
	for i, r := range p.Rules {
		r.Sources = slices.Clone(r.Sources)
		r.Destinations = slices.Clone(r.Destinations)
		slices.Reverse(r.Sources)
		slices.Reverse(r.Destinations)
		out.Rules[i] = r
	}
	return out
}

func decodePolicy(c *call) (*firewallPolicy, error) {
	var p firewallPolicy
	if err := c.decode(&p); err != nil {
		return nil, err
	}
	if p.Rules == nil {
		p.Rules = []firewallRule{}
	}
	return &p, nil
}

func (s *Server) createPolicy(c *call) (int, any) {
	s.checkUser(c)
	p, err := decodePolicy(c)
	if err != nil {
		return badRequest(err.Error())
	}
	p.ID = s.scp.newID()
	s.scp.policies[p.ID] = p
	return http.StatusCreated, returnedPolicy(p)
}

func (s *Server) getPolicy(c *call) (int, any) {
	id, _ := strconv.Atoi(c.params["id"])
	s.checkUser(c)
	p, ok := s.scp.policies[id]
	if !ok {
		return notFound()
	}
	return http.StatusOK, returnedPolicy(p)
}

func (s *Server) putPolicy(c *call) (int, any) {
	id, _ := strconv.Atoi(c.params["id"])
	s.checkUser(c)
	if _, ok := s.scp.policies[id]; !ok {
		return badRequest("firewall policy not found")
	}
	p, err := decodePolicy(c)
	if err != nil {
		return badRequest(err.Error())
	}
	p.ID = id
	s.scp.policies[id] = p
	return http.StatusAccepted, map[string]any{"firewallPolicy": returnedPolicy(p), "taskInfo": s.scp.startTask("FirewallPolicyUpdate")}
}

func (s *Server) deletePolicy(c *call) (int, any) {
	id, _ := strconv.Atoi(c.params["id"])
	s.checkUser(c)
	if _, ok := s.scp.policies[id]; !ok {
		return notFound()
	}
	delete(s.scp.policies, id)
	return http.StatusNoContent, nil
}

func (s *Server) listSSHKeys(c *call) (int, any) {
	s.checkUser(c)
	out := []sshKey{}
	for _, k := range sortedValues(s.scp.sshKeys) {
		listed := *k
		listed.CreatedAt = createdAtMicros
		out = append(out, listed)
	}
	return http.StatusOK, out
}

func (s *Server) createSSHKey(c *call) (int, any) {
	s.checkUser(c)
	var k sshKey
	if err := c.decode(&k); err != nil {
		return badRequest(err.Error())
	}
	k.ID = s.scp.newID()
	k.CreatedAt = createdAtNanos
	s.scp.sshKeys[k.ID] = &k
	return http.StatusCreated, k
}

func (s *Server) deleteSSHKey(c *call) (int, any) {
	id, _ := strconv.Atoi(c.params["id"])
	s.checkUser(c)
	if _, ok := s.scp.sshKeys[id]; !ok {
		return notFound()
	}
	delete(s.scp.sshKeys, id)
	return http.StatusNoContent, nil
}
