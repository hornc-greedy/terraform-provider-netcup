// SPDX-License-Identifier: MPL-2.0

package mockapi

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
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
	exportedSizeInKiB = 1048576
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
	UUID              string   `json:"uuid"`
	Name              string   `json:"name"`
	Description       *string  `json:"description"`
	Disks             []string `json:"disks"`
	CreationTime      string   `json:"creationTime"`
	State             string   `json:"state"`
	Online            bool     `json:"online"`
	Exported          bool     `json:"exported"`
	ExportedSizeInKiB *int64   `json:"exportedSizeInKiB"`
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
	nextID            int
	servers           map[int]*server
	firewalls         map[string]*serverFirewall
	snapshots         map[int]map[string]*snapshot
	policies          map[int]*firewallPolicy
	sshKeys           map[int]*sshKey
	rdns              map[string]string
	failoverV4        map[int]*failoverIP
	failoverV6        map[int]*failoverIP
	tasks             map[string]*taskInfo
	taskOverride      *taskInfo
	user              user
	objects           map[string]map[string]*mockObject
	uploads           map[string]*mockUpload
	partStatus        int
	partFailures      int
	completeDelay     time.Duration
	gpuAvailable      bool
	secondDiskDriver  string
	diskDrivers       map[int]string
	attachedIso       map[int]string
	rescueActive      map[int]bool
	isoBody           map[int]string
	vlans             map[int]*vlan
	interfaces        map[int][]*serverInterface
	guestAgentMissing bool
	taskServers       map[string]int
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
		interfaces: map[int][]*serverInterface{defaultServerID: {{mac: defaultMAC, driver: "VIRTIO"}}},
		vlans:      map[int]*vlan{VlanID: {name: "default"}, ForeignVlanID: {name: "foreign", foreign: true}},
		user: user{
			ID: UserID, Username: "mock", Firstname: "Ada", Lastname: "Lovelace", Email: "ada@example.com",
			Language: "en", TimeZone: &defaultTimeZone, ShowNickname: true,
		},
		objects:      map[string]map[string]*mockObject{"images": {}, "isos": {}},
		uploads:      map[string]*mockUpload{},
		attachedIso:  map[int]string{},
		rescueActive: map[int]bool{},
		isoBody:      map[int]string{},
		diskDrivers:  map[int]string{},
		firewalls:    map[string]*serverFirewall{defaultMAC: {copied: []int{copiedPolicyID}, active: true}},
		snapshots:    map[int]map[string]*snapshot{defaultServerID: {}},
		policies:     map[int]*firewallPolicy{copiedPolicyID: {ID: copiedPolicyID, Name: "copied", Rules: []firewallRule{}}},
		sshKeys:      map[int]*sshKey{},
		rdns:         map[string]string{},
		failoverV4:   map[int]*failoverIP{failoverV4ID: {ID: failoverV4ID, Address: defaultFailoverV4, ServerID: defaultServerID}},
		failoverV6:   map[int]*failoverIP{failoverV6ID: {ID: failoverV6ID, Address: "2001:db8:1::", ServerID: 0}},
		tasks:        map[string]*taskInfo{},
		taskServers:  map[string]int{},
	}
}

func (s *scpState) newID() int {
	s.nextID++
	return s.nextID
}

// startTask returns a pending task. Polling it reports FINISHED, or the failure
// set by FailTasks.
func (s *scpState) startTask(name string, serverID int) *taskInfo {
	t := &taskInfo{UUID: fmt.Sprintf("task-%d", s.newID()), Name: name, State: "PENDING"}
	s.tasks[t.UUID] = t
	s.taskServers[t.UUID] = serverID
	return t
}

// ClearFailedTasks lets tasks succeed again.
func (s *Server) ClearFailedTasks() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.taskOverride = nil
}

// FailTasks makes every task end in state ERROR with the given error.
func (s *Server) FailTasks(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.taskOverride = &taskInfo{State: "ERROR", ResponseError: &responseError{Code: "error.mock", Message: message}}
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
	switch status {
	case http.StatusNotFound:
		return key == "GET "+pathPolicy
	case http.StatusBadRequest:
		return key == "POST "+pathUser+"/images/{key}" || key == "POST "+pathUser+"/isos/{key}"
	}
	return false
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

		"GET /api/v1/servers":                               s.listServers,
		"GET " + pathServer:                                 s.getServer,
		"PATCH " + pathServer:                               s.patchServer,
		"GET " + pathInterfaces:                             s.listInterfaces,
		"POST " + pathServer + "/disks/{diskName}:format":   s.formatDisk,
		"POST " + pathServer + "/image":                     s.setupImage,
		"POST " + pathServer + "/user-image":                s.setupUserImage,
		"POST " + pathServer + "/storageoptimization":       s.optimizeStorage,
		"POST " + pathSnapshot + "/revert":                  s.revertSnapshot,
		"POST " + pathSnapshot + "/export":                  s.exportSnapshot,
		"POST " + pathFirewall + ":reapply":                 s.reapplyFirewall,
		"POST " + pathFirewall + ":restore-copied-policies": s.restoreCopiedPolicies,
		"PUT " + pathTask + ":cancel":                       s.cancelTask,
		"POST " + pathServer + "/iso":                       s.attachIso,
		"GET " + pathServer + "/iso":                        s.getIso,
		"DELETE " + pathServer + "/iso":                     s.detachIso,
		"POST " + pathServer + "/rescuesystem":              s.activateRescue,
		"GET " + pathServer + "/rescuesystem":               s.getRescue,
		"DELETE " + pathServer + "/rescuesystem":            s.deactivateRescue,
		"GET " + pathUser + "/vlans":                        s.listUserVlans,
		"GET " + pathUser + "/vlans/{vlanId}":               s.getUserVlan,
		"PUT " + pathUser + "/vlans/{vlanId}":               s.putUserVlan,
		"GET /api/v1/vlans/{vlanId}":                        s.getVlan,
		"POST " + pathInterfaces:                            s.createInterface,
		"GET " + pathInterfaces + "/{mac}":                  s.getInterface,
		"PUT " + pathInterfaces + "/{mac}":                  s.putInterface,
		"DELETE " + pathInterfaces + "/{mac}":               s.deleteInterface,
		"GET " + pathFirewall:                               s.getFirewall,
		"PUT " + pathFirewall:                               s.putFirewall,

		"POST " + pathSnapshots:  s.createSnapshot,
		"GET " + pathSnapshot:    s.getSnapshot,
		"DELETE " + pathSnapshot: s.deleteSnapshot,

		"GET " + pathServer + "/disks":                   s.listDisks,
		"PATCH " + pathServer + "/disks":                 s.patchDisks,
		"GET " + pathServer + "/disks/{diskName}":        s.getDisk,
		"GET " + pathServer + "/disks/supported-drivers": s.listDrivers,
		"GET " + pathServer + "/gpu-driver":              s.getGPUDriver,
		"GET " + pathServer + "/guest-agent":             s.getGuestAgent,
		"GET " + pathServer + "/guest-agent/status":      s.getGuestAgentStatus,
		"GET " + pathServer + "/imageflavours":           s.listFlavours,
		"GET " + pathServer + "/isoimages":               s.listIsoImages,
		"GET " + pathServer + "/logs":                    s.listServerLogs,
		"GET " + pathServer + "/metrics/cpu":             s.getMetrics("cpu0"),
		"GET " + pathServer + "/metrics/disk":            s.getMetrics("vda"),
		"GET " + pathServer + "/metrics/network":         s.getMetrics("eth0"),
		"GET " + pathServer + "/metrics/network/packet":  s.getMetrics("eth0packets"),
		"GET " + pathSnapshots:                           s.listSnapshots,
		"POST " + pathSnapshots + ":dryrun":              s.dryRunSnapshot,
		"GET /api/v1/tasks":                              s.listTasks,
		"GET /api/v1/users/{userId}":                     s.getUser,
		"PUT /api/v1/users/{userId}":                     s.putUser,
		"GET /api/v1/users/{userId}/logs":                s.listUserLogs,

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
	if t.State == "WAITING_FOR_CANCEL" {
		return http.StatusOK, t
	}
	if f := s.scp.taskOverride; f != nil {
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
	type liveInterface struct {
		Mac           string `json:"mac"`
		VlanInterface bool   `json:"vlanInterface"`
		VlanID        *int   `json:"vlanId,omitempty"`
	}
	type live struct {
		liveInfo
		Interfaces []liveInterface `json:"interfaces"`
	}
	type detail struct {
		*server
		ServerLiveInfo live `json:"serverLiveInfo"`
	}
	out := detail{server: v, ServerLiveInfo: live{liveInfo: v.live, Interfaces: []liveInterface{}}}
	for _, i := range s.scp.interfaces[v.ID] {
		entry := liveInterface{Mac: i.mac, VlanInterface: i.vlanID != 0}
		if i.vlanID != 0 {
			entry.VlanID = &i.vlanID
		}
		out.ServerLiveInfo.Interfaces = append(out.ServerLiveInfo.Interfaces, entry)
	}
	return http.StatusOK, out
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
	if option := c.query.Get("stateOption"); option != "" {
		state := ""
		if err := json.Unmarshal(patch["state"], &state); err != nil {
			s.t.Errorf("stateOption %s without state: %s", option, c.body)
		}
		valid := map[string][]string{"ON": {"POWERCYCLE", "RESET"}, "OFF": {"POWEROFF"}}
		if !slices.Contains(valid[state], option) {
			s.t.Errorf("stateOption %s does not fit state %s", option, state)
		}
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
	return http.StatusAccepted, s.scp.startTask("ServerPatch", v.ID)
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

type serverInterface struct {
	mac, driver string
	vlanID      int
}

func (s *Server) interfaceJSON(i *serverInterface) map[string]any {
	// The specification types the driver as a free string here. Lower case makes
	// the tests cover the provider against either form.
	out := map[string]any{"mac": i.mac, "driver": strings.ToLower(i.driver), "speedInMBits": 2500, "ipv4Addresses": []any{}, "ipv6Addresses": []any{}}
	if i.mac != defaultMAC {
		return out
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
	out["ipv4Addresses"] = []map[string]any{{
		"id": 1, "interfaceMac": defaultMAC, "type": "IP", "ip": defaultIPv4, "cidr": defaultIPv4 + "/22", "gateway": "203.0.113.1",
		"rdns": ipv4RDNS, "destinationIp": nil, "editable": false,
	}, {
		"id": 3, "interfaceMac": defaultMAC, "type": "ROUTED_IP", "ip": "203.0.113.11", "cidr": "203.0.113.11/32", "gateway": nil,
		"rdns": nil, "destinationIp": defaultIPv4, "editable": true,
	}}
	out["ipv6Addresses"] = []map[string]any{{
		"id": 2, "interfaceMac": defaultMAC, "type": "IP", "networkPrefix": "2001:db8::", "cidr": "2001:db8::/64", "gateway": "fe80::1",
		"linkLocal": false, "rdns": ipv6RDNS, "destinationIp": nil, "editable": false,
	}}
	return out
}

func (s *Server) listInterfaces(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	out := []map[string]any{}
	for _, i := range s.scp.interfaces[v.ID] {
		out = append(out, s.interfaceJSON(i))
	}
	return http.StatusOK, out
}

func (s *Server) findInterface(c *call) *serverInterface {
	v := s.scpServer(c)
	if v == nil {
		return nil
	}
	for _, i := range s.scp.interfaces[v.ID] {
		if i.mac == c.params["mac"] {
			return i
		}
	}
	return nil
}

func (s *Server) getInterface(c *call) (int, any) {
	i := s.findInterface(c)
	if i == nil {
		return notFound()
	}
	return http.StatusOK, s.interfaceJSON(i)
}

// maxInterfaces is the limit the specification states.
const maxInterfaces = 10

func (s *Server) createInterface(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		s.checkServer(c)
		return validationError("server not found")
	}
	if len(s.scp.interfaces[v.ID]) >= maxInterfaces {
		return http.StatusBadRequest, map[string]any{"code": "error.mock", "message": "Maximum amount of interfaces per server is 10", "errors": []any{}}
	}
	var body struct {
		VlanID        int    `json:"vlanId"`
		NetworkDriver string `json:"networkDriver"`
	}
	if err := c.decode(&body); err != nil {
		return validationError(err.Error())
	}
	mac := fmt.Sprintf("aa:bb:cc:dd:ff:%02x", s.scp.newID()%256)
	s.scp.interfaces[v.ID] = append(s.scp.interfaces[v.ID], &serverInterface{mac: mac, driver: body.NetworkDriver, vlanID: body.VlanID})
	// Assumption, a new interface has an empty firewall.
	s.scp.firewalls[mac] = &serverFirewall{}
	return http.StatusAccepted, s.scp.startTask("ServerCreateNic", v.ID)
}

func (s *Server) putInterface(c *call) (int, any) {
	i := s.findInterface(c)
	if i == nil {
		return notFound()
	}
	var body struct {
		Driver string `json:"driver"`
	}
	if err := c.decode(&body); err != nil {
		s.t.Errorf("PUT interface: %v", err)
		return notFound()
	}
	if body.Driver == i.driver {
		return http.StatusNoContent, nil
	}
	i.driver = body.Driver
	return http.StatusAccepted, s.scp.startTask("ServerUpdateNic", s.scpServer(c).ID)
}

func (s *Server) deleteInterface(c *call) (int, any) {
	i := s.findInterface(c)
	if i == nil {
		// Assumption, the specification lists only 422 as an error here.
		return validationError("interface not found")
	}
	id := s.scpServer(c).ID
	s.scp.interfaces[id] = slices.DeleteFunc(s.scp.interfaces[id], func(other *serverInterface) bool { return other == i })
	delete(s.scp.firewalls, i.mac)
	return http.StatusAccepted, s.scp.startTask("ServerDeleteNic", id)
}

func validationError(message string) (int, any) {
	return http.StatusUnprocessableEntity, map[string]any{"code": "error.mock", "message": message, "errors": []any{}}
}

// InterfaceDriver returns the driver of an interface, or "" if there is none.
func (s *Server) InterfaceDriver(mac string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, i := range s.scp.interfaces[defaultServerID] {
		if i.mac == mac {
			return i.driver
		}
	}
	return ""
}

// InterfaceVlan returns the VLAN of an interface.
func (s *Server) InterfaceVlan(mac string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, i := range s.scp.interfaces[defaultServerID] {
		if i.mac == mac {
			return i.vlanID
		}
	}
	return 0
}

// RemoveInterface makes an interface disappear outside of Terraform.
func (s *Server) RemoveInterface(mac string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.interfaces[defaultServerID] = slices.DeleteFunc(s.scp.interfaces[defaultServerID], func(i *serverInterface) bool { return i.mac == mac })
}

// InterfaceCount returns the number of interfaces of the default server.
func (s *Server) InterfaceCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.scp.interfaces[defaultServerID])
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
	return http.StatusAccepted, s.scp.startTask("ServerFirewallSave", defaultServerID)
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
	return http.StatusAccepted, s.scp.startTask("ServerSnapshotCreate", v.ID)
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
	return http.StatusAccepted, s.scp.startTask("ServerSnapshotDelete", v.ID)
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
		return http.StatusAccepted, s.scp.startTask("FailoverRoute", body.ServerID)
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
	return http.StatusAccepted, map[string]any{"firewallPolicy": returnedPolicy(p), "taskInfo": s.scp.startTask("FirewallPolicyUpdate", 0)}
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

// checkServer fails the test if the server is unknown, for the operations whose
// specification has no 404.
func (s *Server) checkServer(c *call) {
	if s.scpServer(c) == nil {
		s.t.Errorf("request for unknown server %s", c.params["serverId"])
	}
}

var mockUser = map[string]any{"id": UserID, "username": "mock", "firstname": "Mo", "lastname": "Ck", "email": "mock@example.com", "company": nil}

func (s *Server) diskJSON(c *call, name string) map[string]any {
	driver := "VIRTIO"
	if v := s.scpServer(c); v != nil && s.scp.diskDrivers[v.ID] != "" {
		driver = s.scp.diskDrivers[v.ID]
	}
	if name == "vdb" && s.scp.secondDiskDriver != "" {
		driver = s.scp.secondDiskDriver
	}
	return map[string]any{"name": name, "allocationInMiB": 1024, "capacityInMiB": 8192, "storageDriver": driver}
}

// SetSecondDiskDriver gives the second disk another driver than the first.
func (s *Server) SetSecondDiskDriver(driver string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.secondDiskDriver = driver
}

// DiskDriver returns the storage driver of the default server.
func (s *Server) DiskDriver() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scp.diskDrivers[defaultServerID]
}

// SetDiskDriver changes the storage driver outside of Terraform.
func (s *Server) SetDiskDriver(driver string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.diskDrivers[defaultServerID] = driver
}

func (s *Server) patchDisks(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		s.checkServer(c)
		return http.StatusAccepted, s.scp.startTask("ServerPatchDisks", 0)
	}
	var body struct {
		Driver string `json:"driver"`
	}
	if err := c.decode(&body); err != nil {
		s.t.Errorf("PATCH disks: %v", err)
	}
	s.scp.diskDrivers[v.ID] = body.Driver
	s.scp.secondDiskDriver = ""
	return http.StatusAccepted, s.scp.startTask("ServerPatchDisks", v.ID)
}

func (s *Server) listDisks(c *call) (int, any) {
	s.checkServer(c)
	return http.StatusOK, []map[string]any{s.diskJSON(c, "vda"), s.diskJSON(c, "vdb")}
}

func (s *Server) getDisk(c *call) (int, any) {
	s.checkServer(c)
	if c.params["diskName"] != "vda" && c.params["diskName"] != "vdb" {
		s.t.Errorf("request for unknown disk %s", c.params["diskName"])
	}
	return http.StatusOK, s.diskJSON(c, c.params["diskName"])
}

func (s *Server) listDrivers(c *call) (int, any) {
	if s.scpServer(c) == nil {
		return notFound()
	}
	return http.StatusOK, []string{"VIRTIO", "VIRTIO_SCSI"}
}

func (s *Server) getGPUDriver(c *call) (int, any) {
	s.checkServer(c)
	if !s.scp.gpuAvailable {
		return badRequest("GPU driver or vGPU not available")
	}
	return http.StatusOK, map[string]any{
		"filename": "driver.run", "presignedUrl": "https://s3.example/driver.run?signature=mock",
		"presignedUrlValidityDurationInHours": 24, "headers": map[string][]string{"x-amz-checksum-mode": {"ENABLED"}},
	}
}

// RemoveGuestAgent makes the guest agent unavailable.
func (s *Server) RemoveGuestAgent() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.guestAgentMissing = true
}

// MarkSnapshotExported sets the export of a snapshot of the default server.
func (s *Server) MarkSnapshotExported(name string, sizeInKiB int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := s.scp.snapshots[defaultServerID][name]
	snap.Exported = true
	snap.ExportedSizeInKiB = &sizeInKiB
}

// EnableGPU makes the GPU driver of every server available.
func (s *Server) EnableGPU() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.gpuAvailable = true
}

func (s *Server) getGuestAgent(c *call) (int, any) {
	if s.scpServer(c) == nil {
		return notFound()
	}
	if s.scp.guestAgentMissing {
		return http.StatusOK, map[string]any{"guestAgentAvailable": false}
	}
	return http.StatusOK, map[string]any{"guestAgentAvailable": true, "guestAgentData": map[string]any{"version": "9.0"}}
}

func (s *Server) getGuestAgentStatus(c *call) (int, any) {
	if s.scpServer(c) == nil {
		return notFound()
	}
	return http.StatusOK, map[string]any{"available": !s.scp.guestAgentMissing}
}

func (s *Server) listFlavours(c *call) (int, any) {
	if s.scpServer(c) == nil {
		return notFound()
	}
	// netcup names the system in the image, the flavour only says "minimal".
	return http.StatusOK, []map[string]any{
		{"id": 119, "name": "minimal", "alias": "Minimal", "text": "minimal", "image": map[string]any{"id": 2, "name": "Debian 13.7.0 BIOS amd64"}},
		{"id": 128, "name": "minimal", "alias": "Minimal", "text": "minimal", "image": map[string]any{"id": 3, "name": "Debian 13.7.0 UEFI amd64"}},
	}
}

func (s *Server) listIsoImages(c *call) (int, any) {
	s.checkServer(c)
	// The specification requires filename without describing it.
	return http.StatusOK, []map[string]any{{"id": 3, "name": "rescue.iso", "filename": "rescue.iso", "description": nil, "architecture": "AMD64"}}
}

func (s *Server) listServerLogs(c *call) (int, any) {
	s.checkServer(c)
	all := []map[string]any{
		{"type": "INFO", "date": snapshotCreated, "logKey": "server.started", "message": "started", "executingUser": mockUser},
		{"type": "WARNING", "date": snapshotCreated, "logKey": "server.rebooted", "message": "rebooted"},
	}
	return http.StatusOK, page(all, c)
}

func (s *Server) getMetrics(key string) handler {
	return func(c *call) (int, any) {
		s.checkServer(c)
		return http.StatusOK, map[string]any{snapshotCreated: map[string]float64{key: 0.5}}
	}
}

func (s *Server) listSnapshots(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	return http.StatusOK, sortedValues(s.scp.snapshots[v.ID])
}

// dryRunSnapshot answers the dry run with codes the specification lists. Which
// code applies when is an assumption from its descriptions.
func (s *Server) dryRunSnapshot(c *call) (int, any) {
	if s.scpServer(c) == nil {
		return notFound()
	}
	var body struct {
		DiskName       *string `json:"diskName"`
		OnlineSnapshot bool    `json:"onlineSnapshot"`
	}
	if err := c.decode(&body); err != nil {
		return badRequest(err.Error())
	}
	switch {
	case body.OnlineSnapshot && body.DiskName != nil:
		return http.StatusBadRequest, []responseError{{Code: "server.snapshot.create.error.online.diskselected", Message: "Online snapshot cannot select a disk"}}
	case !body.OnlineSnapshot && body.DiskName == nil:
		return http.StatusBadRequest, []responseError{{Code: "server.snapshot.create.error.disk", Message: "Disk name cannot be blank"}}
	}
	return http.StatusOK, []responseError{}
}

func (s *Server) listTasks(c *call) (int, any) {
	type task struct {
		taskInfo
		ExecutingUser map[string]any `json:"executingUser"`
		StartedAt     string         `json:"startedAt"`
		FinishedAt    string         `json:"finishedAt"`
		OnRollback    bool           `json:"onRollback"`
		TaskProgress  map[string]any `json:"taskProgress"`
	}
	all := []task{}
	for _, t := range sortedValues(s.scp.tasks) {
		if state := c.query.Get("state"); state != "" && state != "FINISHED" {
			continue
		}
		if q := c.query.Get("q"); q != "" && !strings.Contains(t.Name, q) {
			continue
		}
		if id := c.query.Get("serverId"); id != "" && id != strconv.Itoa(s.scp.taskServers[t.UUID]) {
			continue
		}
		all = append(all, task{
			taskInfo: taskInfo{UUID: t.UUID, Name: t.Name, State: "FINISHED"}, ExecutingUser: mockUser,
			StartedAt: snapshotCreated, FinishedAt: snapshotCreated,
			TaskProgress: map[string]any{"expectedFinishedAt": snapshotCreated, "progressInPercent": 100},
		})
	}
	return http.StatusOK, page(all, c)
}

// VlanID identifies the VLAN every mock starts with. ForeignVlanID identifies a
// VLAN of another user, which only GET /vlans/{vlanId} finds. That difference is
// an assumption from the 404 texts of the specification.
const (
	VlanID        = 100
	ForeignVlanID = 200
)

// vlan is a VLAN whose new name arrives after readsUntilRename reads, because
// netcup answers an update with 202 and no task.
type vlan struct {
	name, pending    string
	readsUntilRename int
	foreign          bool
	rejectChange     bool
}

// RemoveVlan makes the VLAN disappear outside of Terraform.
func (s *Server) RemoveVlan() {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.scp.vlans, VlanID)
}

// ClearVlanName makes netcup return the VLAN without a name.
func (s *Server) ClearVlanName() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.vlans[VlanID].name = ""
}

// RejectVlanChange makes a change answer the documented 400.
func (s *Server) RejectVlanChange() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.vlans[VlanID].rejectChange = true
}

// VlanPuts returns how many PUT requests the VLANs received.
func (s *Server) VlanPuts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, r := range s.requests {
		if r.Method == http.MethodPut && strings.Contains(r.Path, "/vlans/") {
			count++
		}
	}
	return count
}

// DelayVlanRename makes a new VLAN name arrive only after this many reads.
func (s *Server) DelayVlanRename(reads int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.vlans[VlanID].readsUntilRename = reads
}

// VlanName returns the name of the VLAN.
func (s *Server) VlanName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scp.vlans[VlanID].name
}

func vlanJSON(id int, v *vlan) map[string]any {
	var name any
	if v.name != "" {
		name = v.name
	}
	return map[string]any{
		"vlanId": id, "name": name, "user": mockUser,
		// The specification requires name without describing it.
		"site":           map[string]any{"id": 1, "name": "nue", "city": "Nuernberg"},
		"bandwidthClass": map[string]any{"id": 2, "name": "1G", "speedInMBit": 1000},
	}
}

func (s *Server) readVlan(c *call) (int, *vlan) {
	id, _ := strconv.Atoi(c.params["vlanId"])
	v, ok := s.scp.vlans[id]
	if !ok {
		return id, nil
	}
	if v.pending != "" {
		if v.readsUntilRename > 0 {
			v.readsUntilRename--
		} else {
			v.name, v.pending = v.pending, ""
		}
	}
	return id, v
}

func (s *Server) listUserVlans(c *call) (int, any) {
	s.checkUser(c)
	if c.query.Has("serverId") && c.query.Get("serverId") != strconv.Itoa(defaultServerID) {
		return http.StatusOK, []map[string]any{}
	}
	out := []map[string]any{}
	for _, id := range slices.Sorted(maps.Keys(s.scp.vlans)) {
		if !s.scp.vlans[id].foreign {
			out = append(out, vlanJSON(id, s.scp.vlans[id]))
		}
	}
	return http.StatusOK, out
}

func (s *Server) getUserVlan(c *call) (int, any) {
	s.checkUser(c)
	if id, v := s.readVlan(c); v != nil && v.foreign {
		return notFound()
	} else if v != nil {
		return http.StatusOK, vlanJSON(id, v)
	}
	return notFound()
}

func (s *Server) getVlan(c *call) (int, any) {
	id, v := s.readVlan(c)
	if v == nil {
		return notFound()
	}
	return http.StatusOK, vlanJSON(id, v)
}

func (s *Server) putUserVlan(c *call) (int, any) {
	s.checkUser(c)
	id, _ := strconv.Atoi(c.params["vlanId"])
	v, ok := s.scp.vlans[id]
	if !ok || v.foreign {
		return notFound()
	}
	if v.rejectChange {
		return badRequest("Server must be in state SHUTOFF.")
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := c.decode(&body); err != nil {
		return badRequest(err.Error())
	}
	v.pending = body.Name
	return http.StatusAccepted, nil
}

// AttachedIso returns the ISO of the default server and its attach body.
func (s *Server) AttachedIso() (name, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scp.attachedIso[defaultServerID], s.scp.isoBody[defaultServerID]
}

// SetAttachedIso changes the ISO of the default server outside of Terraform.
func (s *Server) SetAttachedIso(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.attachedIso[defaultServerID] = name
}

// RescueActive reports whether the rescue system of the default server is active.
func (s *Server) RescueActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scp.rescueActive[defaultServerID]
}

// SetRescueActive changes the rescue system outside of Terraform.
func (s *Server) SetRescueActive(active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.rescueActive[defaultServerID] = active
}

// ShutOff sets the default server to state SHUTOFF.
func (s *Server) ShutOff() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.servers[defaultServerID].live.State = "SHUTOFF"
}

func (s *Server) attachIso(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		s.checkServer(c)
		return validationError("server not found")
	}
	var body struct {
		IsoID       *int    `json:"isoId"`
		UserIsoName *string `json:"userIsoName"`
	}
	if err := c.decode(&body); err != nil {
		return validationError(err.Error())
	}
	// Assumptions, the specification does not say what iso contains, that both or
	// no name are invalid, or that an attached ISO is replaced without an error.
	switch {
	case body.IsoID != nil && body.UserIsoName == nil:
		s.scp.attachedIso[v.ID] = "rescue.iso"
	case body.UserIsoName != nil && body.IsoID == nil:
		s.scp.attachedIso[v.ID] = *body.UserIsoName
	default:
		return validationError("exactly one of isoId and userIsoName is needed")
	}
	s.scp.isoBody[v.ID] = string(c.body)
	return http.StatusAccepted, s.scp.startTask("ServerAttachIso", v.ID)
}

func (s *Server) getIso(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil || s.scp.attachedIso[v.ID] == "" {
		s.checkServer(c)
		return http.StatusOK, map[string]any{"isoAttached": false, "iso": nil}
	}
	return http.StatusOK, map[string]any{"isoAttached": true, "iso": s.scp.attachedIso[v.ID]}
}

func (s *Server) detachIso(c *call) (int, any) {
	s.checkServer(c)
	if v := s.scpServer(c); v != nil {
		delete(s.scp.attachedIso, v.ID)
	}
	return http.StatusNoContent, nil
}

func (s *Server) activateRescue(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	if s.scp.rescueActive[v.ID] {
		return badRequest("Rescue system currently active.")
	}
	// netcup answers so, the specification does not say it.
	if v.live.State != "SHUTOFF" {
		return http.StatusBadRequest, responseError{Code: "server.rescuesystem.invalidstate", Message: "Server must be in state 'SHUTOFF'. (current state: '" + v.live.State + "')"}
	}
	s.scp.rescueActive[v.ID] = true
	return http.StatusAccepted, s.scp.startTask("ServerActivateRescueSystem", v.ID)
}

func (s *Server) getRescue(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	if !s.scp.rescueActive[v.ID] {
		return http.StatusOK, map[string]any{"active": false, "password": nil}
	}
	return http.StatusOK, map[string]any{"active": true, "password": "mock-rescue-password"}
}

func (s *Server) deactivateRescue(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	// netcup answers with this code and shuts the server down, the specification
	// does not say either.
	if !s.scp.rescueActive[v.ID] {
		return http.StatusBadRequest, responseError{Code: "server.rescuesystem.notactive", Message: "Rescue system is currently not active."}
	}
	s.scp.rescueActive[v.ID] = false
	v.live.State = "SHUTOFF"
	return http.StatusAccepted, s.scp.startTask("ServerDeactivateRescueSystem", v.ID)
}

// PatchBodies returns the bodies of all PATCH requests in the order they came.
func (s *Server) PatchBodies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, r := range s.requests {
		if r.Method == http.MethodPatch {
			out = append(out, r.Body)
		}
	}
	return out
}

// taskFor starts a task for an operation whose only effect tests read from
// Requests.
func (s *Server) taskFor(c *call, name string) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	return http.StatusAccepted, s.scp.startTask(name, v.ID)
}

func (s *Server) formatDisk(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	disk := c.params["diskName"]
	if disk != "vda" && disk != "vdb" {
		return notFound()
	}
	if v.live.State != "SHUTOFF" {
		return badRequest("server is not in state SHUTOFF")
	}
	for _, snapshot := range s.scp.snapshots[v.ID] {
		if slices.Contains(snapshot.Disks, disk) {
			return badRequest("disk has snapshots")
		}
	}
	return http.StatusAccepted, s.scp.startTask("ServerFormatDisk", v.ID)
}

func (s *Server) setupImage(c *call) (int, any) {
	if s.scpServer(c) == nil {
		s.checkServer(c)
		return validationError("server not found")
	}
	return s.taskFor(c, "ServerSetupImage")
}

func (s *Server) setupUserImage(c *call) (int, any) {
	if s.scpServer(c) == nil {
		s.checkServer(c)
		return validationError("server not found")
	}
	return s.taskFor(c, "ServerSetupUserImage")
}

func (s *Server) optimizeStorage(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		s.checkServer(c)
		return http.StatusServiceUnavailable, responseError{Code: "error.mock", Message: "server not found"}
	}
	return http.StatusAccepted, s.scp.startTask("ServerStorageOptimization", v.ID)
}

func (s *Server) revertSnapshot(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	if _, ok := s.scp.snapshots[v.ID][c.params["name"]]; !ok {
		return notFound()
	}
	return http.StatusAccepted, s.scp.startTask("ServerSnapshotRevert", v.ID)
}

func (s *Server) exportSnapshot(c *call) (int, any) {
	v := s.scpServer(c)
	if v == nil {
		return notFound()
	}
	snap, ok := s.scp.snapshots[v.ID][c.params["name"]]
	if !ok {
		return notFound()
	}
	if v.live.State != "SHUTOFF" || v.ExportCount == 0 {
		return badRequest("no more snapshots left or server is not in state shutoff")
	}
	size := int64(exportedSizeInKiB)
	snap.Exported, snap.ExportedSizeInKiB = true, &size
	return http.StatusAccepted, s.scp.startTask("ServerSnapshotExport", v.ID)
}

func (s *Server) reapplyFirewall(c *call) (int, any) {
	if s.findInterface(c) == nil {
		s.t.Errorf("reapply for unknown interface %s", c.params["mac"])
	}
	return http.StatusAccepted, s.scp.startTask("ServerFirewallReapply", defaultServerID)
}

func (s *Server) restoreCopiedPolicies(c *call) (int, any) {
	if s.findInterface(c) == nil {
		return notFound()
	}
	return s.taskFor(c, "ServerFirewallRestore")
}

func (s *Server) cancelTask(c *call) (int, any) {
	t, ok := s.scp.tasks[c.params["uuid"]]
	if !ok {
		s.t.Errorf("cancel of unknown task %s", c.params["uuid"])
		return http.StatusNoContent, nil
	}
	if t.State == "FINISHED" {
		return http.StatusNoContent, nil
	}
	t.State = "WAITING_FOR_CANCEL"
	return http.StatusAccepted, t
}

// AddSnapshot adds a snapshot of the default server outside of Terraform.
func (s *Server) AddSnapshot(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.snapshots[defaultServerID][name] = &snapshot{
		UUID: "snap-" + name, Name: name, Disks: []string{"vda"}, CreationTime: snapshotCreated, State: "SHUTOFF",
	}
}

// HoldTasks keeps every task in state RUNNING.
func (s *Server) HoldTasks() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.taskOverride = &taskInfo{State: "RUNNING"}
}

// FinishTask ends a task started with StartTask.
func (s *Server) FinishTask(uuid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.tasks[uuid].State = "FINISHED"
}

// StartTask starts a task outside of Terraform and returns its UUID.
func (s *Server) StartTask() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scp.startTask("ServerMockTask", defaultServerID).UUID
}

var defaultTimeZone = "Europe/Berlin"

// user mirrors the User schema.
type user struct {
	ID                     int     `json:"id"`
	Username               string  `json:"username"`
	Firstname              string  `json:"firstname"`
	Lastname               string  `json:"lastname"`
	Email                  string  `json:"email"`
	Company                *string `json:"company"`
	Language               string  `json:"language"`
	TimeZone               *string `json:"timeZone"`
	ShowNickname           bool    `json:"showNickname"`
	PasswordlessMode       bool    `json:"passwordlessMode"`
	APIIPLoginRestrictions *string `json:"apiIpLoginRestrictions"`
}

func (s *Server) getUser(c *call) (int, any) {
	s.checkUser(c)
	return http.StatusOK, s.scp.user
}

// putUser stores a UserSave. Assumption: an empty apiIpLoginRestrictions removes
// the restrictions and reads back as null.
func (s *Server) putUser(c *call) (int, any) {
	s.checkUser(c)
	var body struct {
		ID                     int     `json:"id"`
		Language               string  `json:"language"`
		TimeZone               string  `json:"timeZone"`
		APIIPLoginRestrictions *string `json:"apiIpLoginRestrictions"`
		ShowNickname           *bool   `json:"showNickname"`
		PasswordlessMode       *bool   `json:"passwordlessMode"`
	}
	if err := c.decode(&body); err != nil {
		return validationError(err.Error())
	}
	u := &s.scp.user
	u.Language, u.TimeZone = body.Language, &body.TimeZone
	if r := body.APIIPLoginRestrictions; r != nil {
		u.APIIPLoginRestrictions = r
		if *r == "" {
			u.APIIPLoginRestrictions = nil
		}
	}
	if body.ShowNickname != nil {
		u.ShowNickname = *body.ShowNickname
	}
	if body.PasswordlessMode != nil {
		u.PasswordlessMode = *body.PasswordlessMode
	}
	return http.StatusOK, map[string]any{"id": u.ID, "language": u.Language, "timeZone": body.TimeZone}
}

// SetUserSettings changes settings of the user outside of Terraform.
func (s *Server) SetUserSettings(company, restrictions string, passwordless bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.user.Company, s.scp.user.APIIPLoginRestrictions, s.scp.user.PasswordlessMode = &company, &restrictions, passwordless
}

// ChangeUserID makes GET of the user return another id, as another refresh token
// would.
func (s *Server) ChangeUserID(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.scp.user.ID = id
}

func (s *Server) listUserLogs(c *call) (int, any) {
	s.checkUser(c)
	all := []map[string]any{
		{"type": "INFO", "date": snapshotCreated, "logKey": "user.login", "message": "login", "executingUser": map[string]any{"id": UserID, "username": "mock"}},
		{"type": "WARNING", "date": snapshotCreated, "logKey": "user.changed", "message": "changed"},
	}
	return http.StatusOK, page(all, c)
}
