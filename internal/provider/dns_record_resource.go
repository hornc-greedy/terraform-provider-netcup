// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/client"
)

var (
	_ resource.Resource                = (*dnsRecordResource)(nil)
	_ resource.ResourceWithConfigure   = (*dnsRecordResource)(nil)
	_ resource.ResourceWithImportState = (*dnsRecordResource)(nil)
)

type dnsRecordResource struct {
	client *client.Client
}

// NewDNSRecordResource is the resource factory.
func NewDNSRecordResource() resource.Resource {
	return &dnsRecordResource{}
}

type dnsRecordModel struct {
	ID          types.String `tfsdk:"id"`
	Domain      types.String `tfsdk:"domain"`
	Hostname    types.String `tfsdk:"hostname"`
	Type        types.String `tfsdk:"type"`
	Destination types.String `tfsdk:"destination"`
	Priority    types.String `tfsdk:"priority"`
}

func (r *dnsRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *dnsRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A single DNS record within a netcup-hosted zone.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "netcup record id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Zone (domain) the record belongs to, e.g. `example.com`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"hostname": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record host, e.g. `@`, `www` or `_dmarc`.",
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record type, e.g. `A`, `AAAA`, `CNAME`, `MX`, `TXT`.",
				Validators: []validator.String{
					stringvalidator.OneOf(
						"A", "AAAA", "MX", "CNAME", "CAA", "SRV",
						"TXT", "TLSA", "NS", "DS", "OPENPGPKEY", "SMIMEA", "SSHFP",
					),
				},
			},
			"destination": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Record value (target / content).",
			},
			"priority": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("0"),
				MarkdownDescription: "Priority, relevant for `MX` / `SRV`. Defaults to `0`.",
			},
		},
	}
}

func (r *dnsRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireCCP(data, &resp.Diagnostics)
}

func (r *dnsRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.CreateDNSRecord(ctx, plan.Domain.ValueString(), client.DNSRecord{
		Hostname:    plan.Hostname.ValueString(),
		Type:        plan.Type.ValueString(),
		Destination: plan.Destination.ValueString(),
		Priority:    plan.Priority.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating DNS record", err.Error())
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Priority = types.StringValue(created.Priority)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dnsRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	records, err := r.client.InfoDNSRecords(ctx, state.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading DNS records", err.Error())
		return
	}

	for _, rec := range records {
		if rec.ID == state.ID.ValueString() {
			state.Hostname = types.StringValue(rec.Hostname)
			state.Type = types.StringValue(rec.Type)
			state.Destination = types.StringValue(rec.Destination)
			state.Priority = types.StringValue(rec.Priority)
			resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
			return
		}
	}
	// Record gone upstream -> drop it so Terraform plans a recreate.
	resp.State.RemoveResource(ctx)
}

func (r *dnsRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateDNSRecord(ctx, plan.Domain.ValueString(), client.DNSRecord{
		ID:          plan.ID.ValueString(),
		Hostname:    plan.Hostname.ValueString(),
		Type:        plan.Type.ValueString(),
		Destination: plan.Destination.ValueString(),
		Priority:    plan.Priority.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error updating DNS record", err.Error())
		return
	}

	plan.Priority = types.StringValue(updated.Priority)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *dnsRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteDNSRecord(ctx, state.Domain.ValueString(), client.DNSRecord{
		ID:          state.ID.ValueString(),
		Hostname:    state.Hostname.ValueString(),
		Type:        state.Type.ValueString(),
		Destination: state.Destination.ValueString(),
		Priority:    state.Priority.ValueString(),
	}); err != nil {
		resp.Diagnostics.AddError("Error deleting DNS record", err.Error())
	}
}

func (r *dnsRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected format <domain>:<record_id>, e.g. example.com:123456789",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
