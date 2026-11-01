// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

var (
	_ datasource.DataSource              = (*serverInterfacesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*serverInterfacesDataSource)(nil)
)

type serverInterfacesDataSource struct {
	client *scp.Client
}

// NewServerInterfacesDataSource is the data source factory.
func NewServerInterfacesDataSource() datasource.DataSource {
	return &serverInterfacesDataSource{}
}

type serverInterfacesModel struct {
	ServerID   types.Int64      `tfsdk:"server_id"`
	Interfaces []interfaceModel `tfsdk:"interfaces"`
}

type interfaceModel struct {
	Mac           types.String `tfsdk:"mac"`
	Driver        types.String `tfsdk:"driver"`
	SpeedInMBits  types.Int64  `tfsdk:"speed_in_mbits"`
	IPv4Addresses []ipv4Model  `tfsdk:"ipv4_addresses"`
	IPv6Addresses []ipv6Model  `tfsdk:"ipv6_addresses"`
}

type ipv4Model struct {
	ID            types.Int64  `tfsdk:"id"`
	InterfaceMac  types.String `tfsdk:"interface_mac"`
	Type          types.String `tfsdk:"type"`
	IP            types.String `tfsdk:"ip"`
	CIDR          types.String `tfsdk:"cidr"`
	Gateway       types.String `tfsdk:"gateway"`
	RDNS          types.String `tfsdk:"rdns"`
	DestinationIP types.String `tfsdk:"destination_ip"`
	Editable      types.Bool   `tfsdk:"editable"`
}

type ipv6Model struct {
	ID            types.Int64  `tfsdk:"id"`
	InterfaceMac  types.String `tfsdk:"interface_mac"`
	Type          types.String `tfsdk:"type"`
	NetworkPrefix types.String `tfsdk:"network_prefix"`
	CIDR          types.String `tfsdk:"cidr"`
	Gateway       types.String `tfsdk:"gateway"`
	LinkLocal     types.Bool   `tfsdk:"link_local"`
	RDNS          types.Map    `tfsdk:"rdns"`
	DestinationIP types.String `tfsdk:"destination_ip"`
	Editable      types.Bool   `tfsdk:"editable"`
}

// interfaceAttributes is the schema of one interface.
func interfaceAttributes() map[string]schema.Attribute {
	common := func(attributes map[string]schema.Attribute) map[string]schema.Attribute {
		attributes["id"] = schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the address."}
		attributes["interface_mac"] = schema.StringAttribute{Computed: true, MarkdownDescription: "MAC address of the interface."}
		attributes["type"] = schema.StringAttribute{Computed: true, MarkdownDescription: "`IP` or `ROUTED_IP`."}
		attributes["destination_ip"] = schema.StringAttribute{Computed: true, MarkdownDescription: "Destination IP."}
		attributes["editable"] = schema.BoolAttribute{Computed: true, MarkdownDescription: "The `editable` value of the SCP REST API."}
		return attributes
	}
	return map[string]schema.Attribute{
		"mac":            schema.StringAttribute{Computed: true, MarkdownDescription: "MAC address."},
		"driver":         schema.StringAttribute{Computed: true, MarkdownDescription: "NIC driver."},
		"speed_in_mbits": schema.Int64Attribute{Computed: true, MarkdownDescription: "Link speed in MBit/s."},
		"ipv4_addresses": schema.ListNestedAttribute{
			Computed:            true,
			MarkdownDescription: "IPv4 addresses.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: common(map[string]schema.Attribute{
					"ip":      schema.StringAttribute{Computed: true, MarkdownDescription: "IPv4 address."},
					"cidr":    schema.StringAttribute{Computed: true, MarkdownDescription: "CIDR."},
					"gateway": schema.StringAttribute{Computed: true, MarkdownDescription: "Gateway."},
					"rdns":    schema.StringAttribute{Computed: true, MarkdownDescription: "Reverse DNS hostname."},
				}),
			},
		},
		"ipv6_addresses": schema.ListNestedAttribute{
			Computed:            true,
			MarkdownDescription: "IPv6 addresses.",
			NestedObject: schema.NestedAttributeObject{
				Attributes: common(map[string]schema.Attribute{
					"network_prefix": schema.StringAttribute{Computed: true, MarkdownDescription: "Network prefix."},
					"cidr":           schema.StringAttribute{Computed: true, MarkdownDescription: "CIDR."},
					"gateway":        schema.StringAttribute{Computed: true, MarkdownDescription: "Gateway."},
					"link_local":     schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is a link-local address."},
					"rdns": schema.MapAttribute{
						Computed:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Reverse DNS entries, keyed by full IPv6 address.",
					},
				}),
			},
		},
	}
}

// interfaceToModel converts an API interface.
func interfaceToModel(ctx context.Context, ifc scp.Interface, diags *diag.Diagnostics) interfaceModel {
	im := interfaceModel{
		Mac:          types.StringValue(ifc.Mac),
		Driver:       types.StringValue(ifc.Driver),
		SpeedInMBits: types.Int64Value(int64(ifc.SpeedInMBits)),
	}
	for _, a := range ifc.IPv4Addresses {
		im.IPv4Addresses = append(im.IPv4Addresses, ipv4Model{
			ID:            types.Int64Value(int64(a.ID)),
			InterfaceMac:  types.StringValue(a.InterfaceMac),
			Type:          types.StringValue(a.Type),
			IP:            types.StringValue(a.IP),
			CIDR:          types.StringValue(a.CIDR),
			Gateway:       types.StringValue(a.Gateway),
			RDNS:          types.StringValue(a.RDNS),
			DestinationIP: optString(a.DestinationIP),
			Editable:      types.BoolValue(a.Editable),
		})
	}
	for _, a := range ifc.IPv6Addresses {
		rdns, d := types.MapValueFrom(ctx, types.StringType, a.RDNS)
		diags.Append(d...)
		im.IPv6Addresses = append(im.IPv6Addresses, ipv6Model{
			ID:            types.Int64Value(int64(a.ID)),
			InterfaceMac:  types.StringValue(a.InterfaceMac),
			Type:          types.StringValue(a.Type),
			NetworkPrefix: types.StringValue(a.NetworkPrefix),
			CIDR:          types.StringValue(a.CIDR),
			Gateway:       types.StringValue(a.Gateway),
			LinkLocal:     types.BoolValue(a.LinkLocal),
			RDNS:          rdns,
			DestinationIP: optString(a.DestinationIP),
			Editable:      types.BoolValue(a.Editable),
		})
	}
	return im
}

func (d *serverInterfacesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_interfaces"
}

func (d *serverInterfacesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Network interfaces of a netcup server, including IPv4/IPv6 addresses " +
			"and their reverse DNS entries (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Server id to read interfaces for.",
			},
			"interfaces": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Interfaces of the server.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: interfaceAttributes(),
				},
			},
		},
	}
}

func (d *serverInterfacesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	d.client = requireSCP(data, &resp.Diagnostics)
}

func (d *serverInterfacesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg serverInterfacesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ifaces, err := d.client.GetInterfaces(ctx, int(cfg.ServerID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading server interfaces", err.Error())
		return
	}

	cfg.Interfaces = make([]interfaceModel, 0, len(ifaces))
	for _, ifc := range ifaces {
		cfg.Interfaces = append(cfg.Interfaces, interfaceToModel(ctx, ifc, &resp.Diagnostics))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// macPattern is the format the specification states for a MAC address.
var macPattern = regexp.MustCompile(`^[a-fA-F0-9]{2}(:[a-fA-F0-9]{2}){5}$`)

// withNullValues turns what the API sends as null or an empty list into null
// values and empty lists. netcup_server_interfaces keeps "" and null lists for
// compatibility.
func withNullValues(m interfaceModel) interfaceModel {
	if m.IPv4Addresses == nil {
		m.IPv4Addresses = []ipv4Model{}
	}
	if m.IPv6Addresses == nil {
		m.IPv6Addresses = []ipv6Model{}
	}
	for i := range m.IPv4Addresses {
		m.IPv4Addresses[i].Gateway = optString(m.IPv4Addresses[i].Gateway.ValueString())
		m.IPv4Addresses[i].RDNS = optString(m.IPv4Addresses[i].RDNS.ValueString())
	}
	for i := range m.IPv6Addresses {
		m.IPv6Addresses[i].Gateway = optString(m.IPv6Addresses[i].Gateway.ValueString())
	}
	return m
}

// serverInterfaceDataSource reads one interface of a server.
type serverInterfaceDataSource struct{ scpDataSource }

// NewServerInterfaceDataSource is the data source factory.
func NewServerInterfaceDataSource() datasource.DataSource { return &serverInterfaceDataSource{} }

func (d *serverInterfaceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_interface"
}

func (d *serverInterfaceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := interfaceAttributes()
	attributes["server_id"] = serverIDAttribute()
	attributes["mac"] = schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "MAC address of the interface.",
		Validators:          []validator.String{stringvalidator.RegexMatches(macPattern, "must be a MAC address")},
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "One network interface of a netcup server, including its addresses and their reverse DNS entries (SCP REST API).",
		Attributes:          attributes,
	}
}

func (d *serverInterfaceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		interfaceModel
		ServerID types.Int64 `tfsdk:"server_id"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ifc, err := d.client.GetInterface(ctx, int(cfg.ServerID.ValueInt64()), cfg.Mac.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading server interface", err.Error())
		return
	}
	cfg.interfaceModel = withNullValues(interfaceToModel(ctx, *ifc, &resp.Diagnostics))
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
