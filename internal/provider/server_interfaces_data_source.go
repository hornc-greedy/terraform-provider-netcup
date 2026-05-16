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
	IP      types.String `tfsdk:"ip"`
	CIDR    types.String `tfsdk:"cidr"`
	Gateway types.String `tfsdk:"gateway"`
	RDNS    types.String `tfsdk:"rdns"`
}

type ipv6Model struct {
	NetworkPrefix types.String `tfsdk:"network_prefix"`
	CIDR          types.String `tfsdk:"cidr"`
	Gateway       types.String `tfsdk:"gateway"`
	LinkLocal     types.Bool   `tfsdk:"link_local"`
	RDNS          types.Map    `tfsdk:"rdns"`
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
					Attributes: map[string]schema.Attribute{
						"mac":            schema.StringAttribute{Computed: true, MarkdownDescription: "MAC address."},
						"driver":         schema.StringAttribute{Computed: true, MarkdownDescription: "NIC driver."},
						"speed_in_mbits": schema.Int64Attribute{Computed: true, MarkdownDescription: "Link speed in MBit/s."},
						"ipv4_addresses": schema.ListNestedAttribute{
							Computed:            true,
							MarkdownDescription: "IPv4 addresses.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"ip":      schema.StringAttribute{Computed: true, MarkdownDescription: "IPv4 address."},
									"cidr":    schema.StringAttribute{Computed: true, MarkdownDescription: "CIDR."},
									"gateway": schema.StringAttribute{Computed: true, MarkdownDescription: "Gateway."},
									"rdns":    schema.StringAttribute{Computed: true, MarkdownDescription: "Reverse DNS hostname."},
								},
							},
						},
						"ipv6_addresses": schema.ListNestedAttribute{
							Computed:            true,
							MarkdownDescription: "IPv6 addresses.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"network_prefix": schema.StringAttribute{Computed: true, MarkdownDescription: "Network prefix."},
									"cidr":           schema.StringAttribute{Computed: true, MarkdownDescription: "CIDR."},
									"gateway":        schema.StringAttribute{Computed: true, MarkdownDescription: "Gateway."},
									"link_local":     schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is a link-local address."},
									"rdns": schema.MapAttribute{
										Computed:            true,
										ElementType:         types.StringType,
										MarkdownDescription: "Reverse DNS entries, keyed by full IPv6 address.",
									},
								},
							},
						},
					},
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
		im := interfaceModel{
			Mac:          types.StringValue(ifc.Mac),
			Driver:       types.StringValue(ifc.Driver),
			SpeedInMBits: types.Int64Value(int64(ifc.SpeedInMBits)),
		}
		for _, a := range ifc.IPv4Addresses {
			im.IPv4Addresses = append(im.IPv4Addresses, ipv4Model{
				IP:      types.StringValue(a.IP),
				CIDR:    types.StringValue(a.CIDR),
				Gateway: types.StringValue(a.Gateway),
				RDNS:    types.StringValue(a.RDNS),
			})
		}
		for _, a := range ifc.IPv6Addresses {
			rdns, diags := types.MapValueFrom(ctx, types.StringType, a.RDNS)
			resp.Diagnostics.Append(diags...)
			if resp.Diagnostics.HasError() {
				return
			}
			im.IPv6Addresses = append(im.IPv6Addresses, ipv6Model{
				NetworkPrefix: types.StringValue(a.NetworkPrefix),
				CIDR:          types.StringValue(a.CIDR),
				Gateway:       types.StringValue(a.Gateway),
				LinkLocal:     types.BoolValue(a.LinkLocal),
				RDNS:          rdns,
			})
		}
		cfg.Interfaces = append(cfg.Interfaces, im)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
