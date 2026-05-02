// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/client"
)

var (
	_ datasource.DataSource              = (*dnsRecordsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*dnsRecordsDataSource)(nil)
)

type dnsRecordsDataSource struct {
	client *client.Client
}

// NewDNSRecordsDataSource is the data source factory.
func NewDNSRecordsDataSource() datasource.DataSource {
	return &dnsRecordsDataSource{}
}

type dnsRecordsModel struct {
	Domain  types.String     `tfsdk:"domain"`
	Records []dnsRecordEntry `tfsdk:"records"`
}

type dnsRecordEntry struct {
	ID          types.String `tfsdk:"id"`
	Hostname    types.String `tfsdk:"hostname"`
	Type        types.String `tfsdk:"type"`
	Destination types.String `tfsdk:"destination"`
	Priority    types.String `tfsdk:"priority"`
}

func (d *dnsRecordsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_records"
}

func (d *dnsRecordsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "All DNS records of a netcup-hosted zone.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Zone (domain) to read, e.g. `example.com`.",
			},
			"records": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Records found in the zone.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "netcup record id."},
						"hostname":    schema.StringAttribute{Computed: true, MarkdownDescription: "Record host."},
						"type":        schema.StringAttribute{Computed: true, MarkdownDescription: "Record type."},
						"destination": schema.StringAttribute{Computed: true, MarkdownDescription: "Record value."},
						"priority":    schema.StringAttribute{Computed: true, MarkdownDescription: "Record priority."},
					},
				},
			},
		},
	}
}

func (d *dnsRecordsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	d.client = requireCCP(data, &resp.Diagnostics)
}

func (d *dnsRecordsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg dnsRecordsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	records, err := d.client.InfoDNSRecords(ctx, cfg.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading DNS records", err.Error())
		return
	}

	cfg.Records = make([]dnsRecordEntry, 0, len(records))
	for _, rec := range records {
		cfg.Records = append(cfg.Records, dnsRecordEntry{
			ID:          types.StringValue(rec.ID),
			Hostname:    types.StringValue(rec.Hostname),
			Type:        types.StringValue(rec.Type),
			Destination: types.StringValue(rec.Destination),
			Priority:    types.StringValue(rec.Priority),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
