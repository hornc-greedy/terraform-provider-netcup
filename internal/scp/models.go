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
	ExportCount        int    `json:"exportCount"`
	RescueSystemActive bool   `json:"rescueSystemActive"`

	LiveInfo *ServerLiveInfo `json:"serverLiveInfo"`

	Template                 *ServerTemplate `json:"template"`
	IPv4Addresses            []ServerIPv4    `json:"ipv4Addresses"`
	IPv6Addresses            []ServerIPv6    `json:"ipv6Addresses"`
	Site                     *ServerSite     `json:"site"`
	DisksAvailableSpaceInMiB *int64          `json:"disksAvailableSpaceInMiB"`
	SnapshotAllowed          *bool           `json:"snapshotAllowed"`
	GPUDriverAvailable       *bool           `json:"gpuDriverAvailable"`
}

// ServerTemplate mirrors the ServerTemplateMinimal schema.
type ServerTemplate struct {
	ID   *int64 `json:"id"`
	Name string `json:"name"`
}

// ServerSite mirrors the Site schema.
type ServerSite struct {
	ID   *int64 `json:"id"`
	Name string `json:"name"`
	City string `json:"city"`
}

// ServerIPv4 mirrors the IPv4AddressMinimal schema.
type ServerIPv4 struct {
	ID        *int64  `json:"id"`
	IP        string  `json:"ip"`
	Netmask   string  `json:"netmask"`
	Gateway   *string `json:"gateway"`
	Broadcast *string `json:"broadcast"`
}

// ServerIPv6 mirrors the IPv6AddressMinimal schema.
type ServerIPv6 struct {
	ID                  *int64  `json:"id"`
	NetworkPrefix       string  `json:"networkPrefix"`
	NetworkPrefixLength *int64  `json:"networkPrefixLength"`
	Gateway             *string `json:"gateway"`
}

// ServerLiveDisk mirrors the ServerDisk schema.
type ServerLiveDisk struct {
	Dev             string `json:"dev"`
	Driver          string `json:"driver"`
	CapacityInMiB   *int64 `json:"capacityInMiB"`
	AllocationInMiB *int64 `json:"allocationInMiB"`
}

// ServerLiveInfo is the subset of the detail endpoint's serverLiveInfo the
// provider reads. netcup returns null when the node cannot report it.
type ServerLiveInfo struct {
	State          string          `json:"state"`
	Autostart      bool            `json:"autostart"`
	UEFI           bool            `json:"uefi"`
	Bootorder      []string        `json:"bootorder"`
	OSOptimization string          `json:"osOptimization"`
	Sockets        int             `json:"sockets"`
	CoresPerSocket int             `json:"coresPerSocket"`
	KeyboardLayout string          `json:"keyboardLayout"`
	BackupTime     string          `json:"backupTime"`
	Interfaces     []LiveInterface `json:"interfaces"`

	Disks                       []ServerLiveDisk `json:"disks"`
	RequiredStorageOptimization string           `json:"requiredStorageOptimization"`
	Template                    *string          `json:"template"`
	UptimeInSeconds             *int64           `json:"uptimeInSeconds"`
	CurrentServerMemoryInMiB    *int64           `json:"currentServerMemoryInMiB"`
	MaxServerMemoryInMiB        *int64           `json:"maxServerMemoryInMiB"`
	CPUCount                    *int64           `json:"cpuCount"`
	CPUMaxCount                 *int64           `json:"cpuMaxCount"`
	LatestQemu                  *bool            `json:"latestQemu"`
	ConfigChanged               *bool            `json:"configChanged"`
	NestedGuest                 *bool            `json:"nestedGuest"`
	MachineType                 string           `json:"machineType"`
	CloudinitAttached           *bool            `json:"cloudinitAttached"`
}

// LiveInterface is the subset of an entry of serverLiveInfo.interfaces the
// provider reads.
type LiveInterface struct {
	Mac           string `json:"mac"`
	VlanInterface bool   `json:"vlanInterface"`
	VlanID        *int   `json:"vlanId"`
}

// IPv4Address mirrors an entry of an interface's ipv4Addresses. Its rDNS is a
// single hostname string, unlike IPv6 which has a per-address map.
type IPv4Address struct {
	ID            int    `json:"id"`
	IP            string `json:"ip"`
	CIDR          string `json:"cidr"`
	Gateway       string `json:"gateway"`
	RDNS          string `json:"rdns"`
	DestinationIP string `json:"destinationIp"`
	Type          string `json:"type"`
	Editable      bool   `json:"editable"`
	InterfaceMac  string `json:"interfaceMac"`
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
	DestinationIP string            `json:"destinationIp"`
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
