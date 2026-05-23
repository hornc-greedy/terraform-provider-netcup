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
	_ datasource.DataSource              = (*maintenanceDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*maintenanceDataSource)(nil)
)

type maintenanceDataSource struct {
	client *scp.Client
}

// NewMaintenanceDataSource is the data source factory.
func NewMaintenanceDataSource() datasource.DataSource {
	return &maintenanceDataSource{}
}

type maintenanceModel struct {
	StartAt  types.String `tfsdk:"start_at"`
	FinishAt types.String `tfsdk:"finish_at"`
}

func (d *maintenanceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance"
}

func (d *maintenanceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Scheduled SCP system maintenance window, if any (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"start_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Maintenance start (empty if none scheduled)."},
			"finish_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Maintenance finish (empty if none scheduled)."},
		},
	}
}

func (d *maintenanceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	d.client = requireSCP(data, &resp.Diagnostics)
}

func (d *maintenanceDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	m, err := d.client.GetMaintenance(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading maintenance", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, maintenanceModel{
		StartAt:  types.StringValue(m.StartAt),
		FinishAt: types.StringValue(m.FinishAt),
	})...)
}
