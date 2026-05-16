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
	_ datasource.DataSource              = (*serversDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*serversDataSource)(nil)
)

type serversDataSource struct {
	client *scp.Client
}

// NewServersDataSource is the data source factory.
func NewServersDataSource() datasource.DataSource {
	return &serversDataSource{}
}

type serversModel struct {
	Servers []serverListEntryModel `tfsdk:"servers"`
}

type serverListEntryModel struct {
	ID       types.Int64  `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Hostname types.String `tfsdk:"hostname"`
	Nickname types.String `tfsdk:"nickname"`
	Disabled types.Bool   `tfsdk:"disabled"`
}

func (d *serversDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_servers"
}

func (d *serversDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "All servers of the netcup account (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"servers": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Servers in the account.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Server id."},
						"name":     schema.StringAttribute{Computed: true, MarkdownDescription: "Server name."},
						"hostname": schema.StringAttribute{Computed: true, MarkdownDescription: "Hostname."},
						"nickname": schema.StringAttribute{Computed: true, MarkdownDescription: "Nickname."},
						"disabled": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the server is disabled."},
					},
				},
			},
		},
	}
}

func (d *serversDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	d.client = requireSCP(data, &resp.Diagnostics)
}

func (d *serversDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	servers, err := d.client.ListServers(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing servers", err.Error())
		return
	}
	var state serversModel
	state.Servers = make([]serverListEntryModel, 0, len(servers))
	for _, s := range servers {
		state.Servers = append(state.Servers, serverListEntryModel{
			ID:       types.Int64Value(int64(s.ID)),
			Name:     types.StringValue(s.Name),
			Hostname: types.StringValue(s.Hostname),
			Nickname: types.StringValue(s.Nickname),
			Disabled: types.BoolValue(s.Disabled),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
