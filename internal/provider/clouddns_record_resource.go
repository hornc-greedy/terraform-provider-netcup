// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/clouddns"
)

var (
	_ resource.Resource                = (*cloudDNSRecordResource)(nil)
	_ resource.ResourceWithConfigure   = (*cloudDNSRecordResource)(nil)
	_ resource.ResourceWithImportState = (*cloudDNSRecordResource)(nil)
)

type cloudDNSRecordResource struct {
	client *clouddns.Client
}

// NewCloudDNSRecordResource is the resource factory.
func NewCloudDNSRecordResource() resource.Resource {
	return &cloudDNSRecordResource{}
}

type cloudDNSRecordModel struct {
	ID       types.String `tfsdk:"id"`
	DomainID types.Int64  `tfsdk:"domain_id"`
	Name     types.String `tfsdk:"name"`
	Type     types.String `tfsdk:"type"`
	RData    types.String `tfsdk:"rdata"`
	TTL      types.Int64  `tfsdk:"ttl"`
}

func (r *cloudDNSRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clouddns_record"
}

func (r *cloudDNSRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A DNS record of a domain on CloudDNS (netcup API). Each change is one changeset.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite id `domain_id/name/type/rdata`.",
			},
			"domain_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Id of the domain, from the `netcup_clouddns_domain` data source.",
				PlanModifiers:       []planmodifier.Int64{int64RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record name relative to the zone, e.g. `www`, `@` for the apex or `*` as wildcard label.",
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 255)},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record type, e.g. `A`, `AAAA`, `CNAME`, `MX`, `TXT`.",
				Validators: []validator.String{stringvalidator.OneOf(
					"A", "AAAA", "CAA", "CNAME", "DS", "MX", "NS", "OPENPGPKEY", "SRV", "SMIMEA", "SSHFP", "TLSA", "TXT",
				)},
			},
			"rdata": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Record data in zone file format, e.g. an IP address, `priority hostname` for `MX` or " +
					"`priority weight port target` for `SRV`.",
				Validators: []validator.String{stringvalidator.LengthBetween(1, 65535)},
			},
			"ttl": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "TTL in seconds.",
				Validators:          []validator.Int64{int64validator.AtLeast(1)},
			},
		},
	}
}

func (r *cloudDNSRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireCloudDNS(data, &resp.Diagnostics)
}

func (m cloudDNSRecordModel) input() clouddns.RecordInput {
	return clouddns.RecordInput{
		Name: m.Name.ValueString(), Type: m.Type.ValueString(), RData: m.RData.ValueString(), TTL: m.TTL.ValueInt64(),
	}
}

func (m cloudDNSRecordModel) compositeID() types.String {
	return types.StringValue(fmt.Sprintf("%d/%s/%s/%s", m.DomainID.ValueInt64(), m.Name.ValueString(), m.Type.ValueString(), m.RData.ValueString()))
}

// find returns the record of the active revision with the name, type and rdata of
// the model, or nil if there is none.
func (r *cloudDNSRecordResource) find(ctx context.Context, m cloudDNSRecordModel) (*clouddns.Record, error) {
	domainID := m.DomainID.ValueInt64()
	revision, err := r.client.ActiveRevision(ctx, domainID)
	if err != nil {
		return nil, err
	}
	records, err := r.client.Records(ctx, domainID, revision.Identifier)
	if err != nil {
		return nil, err
	}
	for i, rec := range records {
		if rec.Name == m.Name.ValueString() && rec.Type == m.Type.ValueString() && rec.RData == m.RData.ValueString() {
			return &records[i], nil
		}
	}
	return nil, nil
}

// readBack stores the record netcup holds after a change. It fails if netcup
// stores other values than planned, because the next refresh would not find the
// record and the next apply could not create it again.
func (r *cloudDNSRecordResource) readBack(ctx context.Context, plan *cloudDNSRecordModel) error {
	record, err := r.find(ctx, *plan)
	if err != nil {
		return err
	}
	if record == nil {
		return fmt.Errorf("netcup does not hold %s %s with the rdata %q after the change. It may store the value in another form, write it exactly as netcup_clouddns_records shows it",
			plan.Name.ValueString(), plan.Type.ValueString(), plan.RData.ValueString())
	}
	plan.TTL = types.Int64PointerValue(record.TTL)
	plan.ID = plan.compositeID()
	return nil
}

func (r *cloudDNSRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cloudDNSRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.ChangeRecords(ctx, plan.DomainID.ValueInt64(), []clouddns.RecordInput{plan.input()}, nil); err != nil {
		resp.Diagnostics.AddError("Error creating CloudDNS record", err.Error())
		return
	}
	if err := r.readBack(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error creating CloudDNS record", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *cloudDNSRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cloudDNSRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	record, err := r.find(ctx, state)
	if err != nil {
		resp.Diagnostics.AddError("Error reading CloudDNS record", err.Error())
		return
	}
	if record == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.TTL = types.Int64PointerValue(record.TTL)
	state.ID = state.compositeID()
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *cloudDNSRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state cloudDNSRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.ChangeRecords(ctx, plan.DomainID.ValueInt64(), []clouddns.RecordInput{plan.input()}, []clouddns.RecordInput{state.input()})
	if err != nil {
		resp.Diagnostics.AddError("Error updating CloudDNS record", err.Error())
		return
	}
	if err := r.readBack(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error updating CloudDNS record", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *cloudDNSRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cloudDNSRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.ChangeRecords(ctx, state.DomainID.ValueInt64(), nil, []clouddns.RecordInput{state.input()}); err != nil {
		resp.Diagnostics.AddError("Error deleting CloudDNS record", err.Error())
	}
}

func (r *cloudDNSRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 4)
	if len(parts) != 4 || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format <domain_id>/<name>/<type>/<rdata>, e.g. 12345/www/A/192.0.2.1")
		return
	}
	domainID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid domain_id in import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain_id"), domainID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("type"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("rdata"), parts[3])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
