// SPDX-License-Identifier: MPL-2.0

package scp

// Server is the subset of server fields the provider exposes. The list
// endpoint populates only the first group; the detail endpoint fills the rest.
type Server struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Nickname string `json:"nickname"`
	Disabled bool   `json:"disabled"`

	Architecture       string `json:"architecture"`
	MaxCPUCount        int    `json:"maxCpuCount"`
	SnapshotCount      int    `json:"snapshotCount"`
	RescueSystemActive bool   `json:"rescueSystemActive"`
}

// IPv4Address mirrors an entry of an interface's ipv4Addresses. Its rDNS is a
// single hostname string, unlike IPv6 which carries a per-address map.
type IPv4Address struct {
	ID           int    `json:"id"`
	IP           string `json:"ip"`
	CIDR         string `json:"cidr"`
	Gateway      string `json:"gateway"`
	RDNS         string `json:"rdns"`
	Type         string `json:"type"`
	Editable     bool   `json:"editable"`
	InterfaceMac string `json:"interfaceMac"`
}

// IPv6Address mirrors an entry of an interface's ipv6Addresses. Its rDNS is a
// map keyed by full IPv6 address, because one prefix can hold many addresses,
// each with its own PTR.
type IPv6Address struct {
	ID            int               `json:"id"`
	CIDR          string            `json:"cidr"`
	NetworkPrefix string            `json:"networkPrefix"`
	Gateway       string            `json:"gateway"`
	LinkLocal     bool              `json:"linkLocal"`
	RDNS          map[string]string `json:"rdns"`
	Type          string            `json:"type"`
	Editable      bool              `json:"editable"`
	InterfaceMac  string            `json:"interfaceMac"`
}

// Interface mirrors one network interface of a server.
type Interface struct {
	Mac           string        `json:"mac"`
	Driver        string        `json:"driver"`
	SpeedInMBits  int           `json:"speedInMBits"`
	IPv4Addresses []IPv4Address `json:"ipv4Addresses"`
	IPv6Addresses []IPv6Address `json:"ipv6Addresses"`
}

// rdnsResponse is the body of GET /rdns/ipv4|ipv6/{ip}.
type rdnsResponse struct {
	RDNS string `json:"rdns"`
}

// setRdns is the body of POST /rdns/ipv4|ipv6.
type setRdns struct {
	IP   string `json:"ip"`
	RDNS string `json:"rdns"`
}
