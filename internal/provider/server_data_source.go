// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

var (
	_ datasource.DataSource              = (*serverDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*serverDataSource)(nil)
)

type serverDataSource struct {
	client *scp.Client
}

// NewServerDataSource is the data source factory.
func NewServerDataSource() datasource.DataSource {
	return &serverDataSource{}
}

type serverModel struct {
	ID                 types.Int64  `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Hostname           types.String `tfsdk:"hostname"`
	Nickname           types.String `tfsdk:"nickname"`
	Disabled           types.Bool   `tfsdk:"disabled"`
	Architecture       types.String `tfsdk:"architecture"`
	MaxCPUCount        types.Int64  `tfsdk:"max_cpu_count"`
	SnapshotCount      types.Int64  `tfsdk:"snapshot_count"`
	RescueSystemActive types.Bool   `tfsdk:"rescue_system_active"`

	ExportCount              types.Int64      `tfsdk:"export_count"`
	LoadServerLiveInfo       types.Bool       `tfsdk:"load_server_live_info"`
	Template                 *templateModel   `tfsdk:"template"`
	IPv4Addresses            []serverIPv4     `tfsdk:"ipv4_addresses"`
	IPv6Addresses            []serverIPv6     `tfsdk:"ipv6_addresses"`
	Site                     *siteModel       `tfsdk:"site"`
	DisksAvailableSpaceInMiB types.Int64      `tfsdk:"disks_available_space_in_mib"`
	SnapshotAllowed          types.Bool       `tfsdk:"snapshot_allowed"`
	GPUDriverAvailable       types.Bool       `tfsdk:"gpu_driver_available"`
	ServerLiveInfo           *serverLiveModel `tfsdk:"server_live_info"`
}

type templateModel struct {
	ID   types.Int64  `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
}

type siteModel struct {
	ID   types.Int64  `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	City types.String `tfsdk:"city"`
}

type serverIPv4 struct {
	ID        types.Int64  `tfsdk:"id"`
	IP        types.String `tfsdk:"ip"`
	Netmask   types.String `tfsdk:"netmask"`
	Gateway   types.String `tfsdk:"gateway"`
	Broadcast types.String `tfsdk:"broadcast"`
}

type serverIPv6 struct {
	ID                  types.Int64  `tfsdk:"id"`
	NetworkPrefix       types.String `tfsdk:"network_prefix"`
	NetworkPrefixLength types.Int64  `tfsdk:"network_prefix_length"`
	Gateway             types.String `tfsdk:"gateway"`
}

type serverLiveDisk struct {
	Dev             types.String `tfsdk:"dev"`
	Driver          types.String `tfsdk:"driver"`
	CapacityInMiB   types.Int64  `tfsdk:"capacity_in_mib"`
	AllocationInMiB types.Int64  `tfsdk:"allocation_in_mib"`
}

type serverLiveModel struct {
	State                       types.String     `tfsdk:"state"`
	Autostart                   types.Bool       `tfsdk:"autostart"`
	UEFI                        types.Bool       `tfsdk:"uefi"`
	Bootorder                   []string         `tfsdk:"bootorder"`
	Disks                       []serverLiveDisk `tfsdk:"disks"`
	RequiredStorageOptimization types.String     `tfsdk:"required_storage_optimization"`
	Template                    types.String     `tfsdk:"template"`
	UptimeInSeconds             types.Int64      `tfsdk:"uptime_in_seconds"`
	CurrentServerMemoryInMiB    types.Int64      `tfsdk:"current_server_memory_in_mib"`
	MaxServerMemoryInMiB        types.Int64      `tfsdk:"max_server_memory_in_mib"`
	CPUCount                    types.Int64      `tfsdk:"cpu_count"`
	CPUMaxCount                 types.Int64      `tfsdk:"cpu_max_count"`
	Sockets                     types.Int64      `tfsdk:"sockets"`
	CoresPerSocket              types.Int64      `tfsdk:"cores_per_socket"`
	LatestQemu                  types.Bool       `tfsdk:"latest_qemu"`
	ConfigChanged               types.Bool       `tfsdk:"config_changed"`
	OSOptimization              types.String     `tfsdk:"os_optimization"`
	NestedGuest                 types.Bool       `tfsdk:"nested_guest"`
	MachineType                 types.String     `tfsdk:"machine_type"`
	KeyboardLayout              types.String     `tfsdk:"keyboard_layout"`
	CloudinitAttached           types.Bool       `tfsdk:"cloudinit_attached"`
	BackupTime                  types.String     `tfsdk:"backup_time"`
}

// serverLive converts the live info, which netcup may not report.
func serverLive(l *scp.ServerLiveInfo) *serverLiveModel {
	if l == nil {
		return nil
	}
	disks := make([]serverLiveDisk, 0, len(l.Disks))
	for _, d := range l.Disks {
		disks = append(disks, serverLiveDisk{
			Dev: optString(d.Dev), Driver: optString(d.Driver),
			CapacityInMiB: types.Int64PointerValue(d.CapacityInMiB), AllocationInMiB: types.Int64PointerValue(d.AllocationInMiB),
		})
	}
	return &serverLiveModel{
		State: optString(l.State), Autostart: types.BoolValue(l.Autostart), UEFI: types.BoolValue(l.UEFI),
		Bootorder: append([]string{}, l.Bootorder...), Disks: disks,
		RequiredStorageOptimization: optString(l.RequiredStorageOptimization),
		Template:                    types.StringPointerValue(l.Template),
		UptimeInSeconds:             types.Int64PointerValue(l.UptimeInSeconds),
		CurrentServerMemoryInMiB:    types.Int64PointerValue(l.CurrentServerMemoryInMiB),
		MaxServerMemoryInMiB:        types.Int64PointerValue(l.MaxServerMemoryInMiB),
		CPUCount:                    types.Int64PointerValue(l.CPUCount),
		CPUMaxCount:                 types.Int64PointerValue(l.CPUMaxCount),
		Sockets:                     types.Int64Value(int64(l.Sockets)),
		CoresPerSocket:              types.Int64Value(int64(l.CoresPerSocket)),
		LatestQemu:                  types.BoolPointerValue(l.LatestQemu),
		ConfigChanged:               types.BoolPointerValue(l.ConfigChanged),
		OSOptimization:              optString(l.OSOptimization),
		NestedGuest:                 types.BoolPointerValue(l.NestedGuest),
		MachineType:                 optString(l.MachineType),
		KeyboardLayout:              optString(l.KeyboardLayout),
		CloudinitAttached:           types.BoolPointerValue(l.CloudinitAttached),
		BackupTime:                  optString(l.BackupTime),
	}
}

func (d *serverDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server"
}

func (d *serverDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Detailed information about a single netcup server (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Server id to look up.",
			},
			"name":          schema.StringAttribute{Computed: true, MarkdownDescription: "Server name."},
			"hostname":      schema.StringAttribute{Computed: true, MarkdownDescription: "Hostname."},
			"nickname":      schema.StringAttribute{Computed: true, MarkdownDescription: "Nickname."},
			"disabled":      schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the server is disabled."},
			"architecture":  schema.StringAttribute{Computed: true, MarkdownDescription: "CPU architecture, e.g. `AMD64`."},
			"max_cpu_count": schema.Int64Attribute{Computed: true, MarkdownDescription: "Maximum CPU count."},
			"snapshot_count": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Deprecated, use `export_count`, which has the same value.",
				DeprecationMessage:  "netcup removes snapshotCount by 31.12.2026. Use export_count.",
			},
			"export_count":         schema.Int64Attribute{Computed: true, MarkdownDescription: "The `exportCount` of the server."},
			"rescue_system_active": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the rescue system is active."},
			"load_server_live_info": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "The `loadServerLiveInfo` query parameter. Defaults to `true`.",
			},
			"template": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Template of the server.",
				Attributes: map[string]schema.Attribute{
					"id":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the template."},
					"name": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the template."},
				},
			},
			"ipv4_addresses": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "IPv4 addresses of the server.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":        schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the address."},
					"ip":        schema.StringAttribute{Computed: true, MarkdownDescription: "IP address."},
					"netmask":   schema.StringAttribute{Computed: true, MarkdownDescription: "Netmask."},
					"gateway":   schema.StringAttribute{Computed: true, MarkdownDescription: "Gateway."},
					"broadcast": schema.StringAttribute{Computed: true, MarkdownDescription: "Broadcast address."},
				}},
			},
			"ipv6_addresses": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "IPv6 networks of the server.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":                    schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the network."},
					"network_prefix":        schema.StringAttribute{Computed: true, MarkdownDescription: "Network prefix."},
					"network_prefix_length": schema.Int64Attribute{Computed: true, MarkdownDescription: "Length of the prefix."},
					"gateway":               schema.StringAttribute{Computed: true, MarkdownDescription: "Gateway."},
				}},
			},
			"site": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Site of the server.",
				Attributes: map[string]schema.Attribute{
					"id":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the site."},
					"name": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the site, null if netcup sends none."},
					"city": schema.StringAttribute{Computed: true, MarkdownDescription: "City of the site."},
				},
			},
			"disks_available_space_in_mib": schema.Int64Attribute{Computed: true, MarkdownDescription: "Space available for disks in MiB."},
			"snapshot_allowed":             schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the server allows snapshots."},
			"gpu_driver_available":         schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether `netcup_server_gpu_driver` has a driver for the server."},
			"server_live_info": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Live state of the server. Null if not loaded or if netcup cannot report it. `netcup_server_interfaces` has the interfaces.",
				Attributes: map[string]schema.Attribute{
					"state":     schema.StringAttribute{Computed: true, MarkdownDescription: "Power state."},
					"autostart": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the server starts with its node."},
					"uefi":      schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the server boots with UEFI."},
					"bootorder": schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Boot order."},
					"disks": schema.ListNestedAttribute{
						Computed:            true,
						MarkdownDescription: "Disks of the server.",
						NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
							"dev":               schema.StringAttribute{Computed: true, MarkdownDescription: "Device name."},
							"driver":            schema.StringAttribute{Computed: true, MarkdownDescription: "Driver."},
							"capacity_in_mib":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Capacity in MiB."},
							"allocation_in_mib": schema.Int64Attribute{Computed: true, MarkdownDescription: "Allocation in MiB."},
						}},
					},
					"required_storage_optimization": schema.StringAttribute{Computed: true, MarkdownDescription: "`INCONSISTENT`, `COMPAT`, `SLOW`, `FAST` or `NO`. `netcup_server_storage_optimization` runs an optimization."},
					"template":                      schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the template."},
					"uptime_in_seconds":             schema.Int64Attribute{Computed: true, MarkdownDescription: "Uptime in seconds."},
					"current_server_memory_in_mib":  schema.Int64Attribute{Computed: true, MarkdownDescription: "Current memory in MiB."},
					"max_server_memory_in_mib":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Maximum memory in MiB."},
					"cpu_count":                     schema.Int64Attribute{Computed: true, MarkdownDescription: "CPU count."},
					"cpu_max_count":                 schema.Int64Attribute{Computed: true, MarkdownDescription: "Maximum CPU count."},
					"sockets":                       schema.Int64Attribute{Computed: true, MarkdownDescription: "CPU sockets."},
					"cores_per_socket":              schema.Int64Attribute{Computed: true, MarkdownDescription: "Cores per socket."},
					"latest_qemu":                   schema.BoolAttribute{Computed: true, MarkdownDescription: "The `latestQemu` value of netcup."},
					"config_changed":                schema.BoolAttribute{Computed: true, MarkdownDescription: "The `configChanged` value of netcup."},
					"os_optimization":               schema.StringAttribute{Computed: true, MarkdownDescription: "OS optimization."},
					"nested_guest":                  schema.BoolAttribute{Computed: true, MarkdownDescription: "The `nestedGuest` value of netcup."},
					"machine_type":                  schema.StringAttribute{Computed: true, MarkdownDescription: "Machine type."},
					"keyboard_layout":               schema.StringAttribute{Computed: true, MarkdownDescription: "Keyboard layout."},
					"cloudinit_attached":            schema.BoolAttribute{Computed: true, MarkdownDescription: "The `cloudinitAttached` value of netcup."},
					"backup_time":                   schema.StringAttribute{Computed: true, MarkdownDescription: "Daily backup start, e.g. `01:30:00+02:00`."},
				},
			},
		},
	}
}

func (d *serverDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	d.client = requireSCP(data, &resp.Diagnostics)
}

func (d *serverDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg serverModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	load := cfg.LoadServerLiveInfo.IsNull() || cfg.LoadServerLiveInfo.ValueBool()
	s, err := d.client.GetServerWithLiveInfo(ctx, int(cfg.ID.ValueInt64()), load)
	if err != nil {
		resp.Diagnostics.AddError("Error reading server", err.Error())
		return
	}
	cfg.Name = types.StringValue(s.Name)
	cfg.Hostname = types.StringValue(s.Hostname)
	cfg.Nickname = types.StringValue(s.Nickname)
	cfg.Disabled = types.BoolValue(s.Disabled)
	cfg.Architecture = types.StringValue(s.Architecture)
	cfg.MaxCPUCount = types.Int64Value(int64(s.MaxCPUCount))
	cfg.SnapshotCount = types.Int64Value(int64(s.ExportCount))
	cfg.RescueSystemActive = types.BoolValue(s.RescueSystemActive)
	cfg.ExportCount = types.Int64Value(int64(s.ExportCount))
	cfg.DisksAvailableSpaceInMiB = types.Int64PointerValue(s.DisksAvailableSpaceInMiB)
	cfg.SnapshotAllowed = types.BoolPointerValue(s.SnapshotAllowed)
	cfg.GPUDriverAvailable = types.BoolPointerValue(s.GPUDriverAvailable)
	cfg.ServerLiveInfo = serverLive(s.LiveInfo)
	cfg.Template = template(s.Template)
	if s.Site != nil {
		cfg.Site = &siteModel{ID: types.Int64PointerValue(s.Site.ID), Name: optString(s.Site.Name), City: optString(s.Site.City)}
	}
	cfg.IPv4Addresses = make([]serverIPv4, 0, len(s.IPv4Addresses))
	for _, a := range s.IPv4Addresses {
		cfg.IPv4Addresses = append(cfg.IPv4Addresses, serverIPv4{
			ID: types.Int64PointerValue(a.ID), IP: optString(a.IP), Netmask: optString(a.Netmask),
			Gateway: types.StringPointerValue(a.Gateway), Broadcast: types.StringPointerValue(a.Broadcast),
		})
	}
	cfg.IPv6Addresses = make([]serverIPv6, 0, len(s.IPv6Addresses))
	for _, a := range s.IPv6Addresses {
		cfg.IPv6Addresses = append(cfg.IPv6Addresses, serverIPv6{
			ID: types.Int64PointerValue(a.ID), NetworkPrefix: optString(a.NetworkPrefix),
			NetworkPrefixLength: types.Int64PointerValue(a.NetworkPrefixLength), Gateway: types.StringPointerValue(a.Gateway),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
