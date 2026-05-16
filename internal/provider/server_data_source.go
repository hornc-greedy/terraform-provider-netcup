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
			"name":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Server name."},
			"hostname":             schema.StringAttribute{Computed: true, MarkdownDescription: "Hostname."},
			"nickname":             schema.StringAttribute{Computed: true, MarkdownDescription: "Nickname."},
			"disabled":             schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the server is disabled."},
			"architecture":         schema.StringAttribute{Computed: true, MarkdownDescription: "CPU architecture, e.g. `AMD64`."},
			"max_cpu_count":        schema.Int64Attribute{Computed: true, MarkdownDescription: "Maximum CPU count."},
			"snapshot_count":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of snapshots."},
			"rescue_system_active": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the rescue system is active."},
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
	s, err := d.client.GetServer(ctx, int(cfg.ID.ValueInt64()))
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
	cfg.SnapshotCount = types.Int64Value(int64(s.SnapshotCount))
	cfg.RescueSystemActive = types.BoolValue(s.RescueSystemActive)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
