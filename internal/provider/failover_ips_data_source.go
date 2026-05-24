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
	_ datasource.DataSource              = (*failoverIPsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*failoverIPsDataSource)(nil)
)

type failoverIPsDataSource struct {
	client *scp.Client
}

// NewFailoverIPsDataSource is the data source factory.
func NewFailoverIPsDataSource() datasource.DataSource {
	return &failoverIPsDataSource{}
}

type failoverIPsModel struct {
	IPv4 []failoverV4Model `tfsdk:"ipv4"`
	IPv6 []failoverV6Model `tfsdk:"ipv6"`
}

type failoverV4Model struct {
	ID       types.Int64  `tfsdk:"id"`
	IP       types.String `tfsdk:"ip"`
	ServerID types.Int64  `tfsdk:"server_id"`
}

type failoverV6Model struct {
	ID            types.Int64  `tfsdk:"id"`
	NetworkPrefix types.String `tfsdk:"network_prefix"`
	ServerID      types.Int64  `tfsdk:"server_id"`
}

func (d *failoverIPsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_failover_ips"
}

func (d *failoverIPsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Failover IPs (v4 and v6) of the netcup account (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"ipv4": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Failover IPv4s.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":        schema.Int64Attribute{Computed: true, MarkdownDescription: "Failover IP id."},
						"ip":        schema.StringAttribute{Computed: true, MarkdownDescription: "IPv4 address."},
						"server_id": schema.Int64Attribute{Computed: true, MarkdownDescription: "Currently routed server id (0 if none)."},
					},
				},
			},
			"ipv6": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Failover IPv6s.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.Int64Attribute{Computed: true, MarkdownDescription: "Failover IP id."},
						"network_prefix": schema.StringAttribute{Computed: true, MarkdownDescription: "IPv6 network prefix."},
						"server_id":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Currently routed server id (0 if none)."},
					},
				},
			},
		},
	}
}

func (d *failoverIPsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	d.client = requireSCP(data, &resp.Diagnostics)
}

func (d *failoverIPsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	v4, err := d.client.ListFailoverIPv4(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing failover IPv4", err.Error())
		return
	}
	v6, err := d.client.ListFailoverIPv6(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing failover IPv6", err.Error())
		return
	}
	var state failoverIPsModel
	for _, ip := range v4 {
		sid := 0
		if ip.Server != nil {
			sid = ip.Server.ID
		}
		state.IPv4 = append(state.IPv4, failoverV4Model{
			ID:       types.Int64Value(int64(ip.ID)),
			IP:       types.StringValue(ip.IP),
			ServerID: types.Int64Value(int64(sid)),
		})
	}
	for _, ip := range v6 {
		sid := 0
		if ip.Server != nil {
			sid = ip.Server.ID
		}
		state.IPv6 = append(state.IPv6, failoverV6Model{
			ID:            types.Int64Value(int64(ip.ID)),
			NetworkPrefix: types.StringValue(ip.NetworkPrefix),
			ServerID:      types.Int64Value(int64(sid)),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
