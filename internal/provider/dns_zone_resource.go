// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/client"
)

var (
	_ resource.Resource                = (*dnsZoneResource)(nil)
	_ resource.ResourceWithConfigure   = (*dnsZoneResource)(nil)
	_ resource.ResourceWithImportState = (*dnsZoneResource)(nil)
)

type dnsZoneResource struct {
	client *client.Client
}

// NewDNSZoneResource is the resource factory.
func NewDNSZoneResource() resource.Resource {
	return &dnsZoneResource{}
}

type dnsZoneModel struct {
	ID           types.String `tfsdk:"id"`
	Domain       types.String `tfsdk:"domain"`
	TTL          types.Int64  `tfsdk:"ttl"`
	Refresh      types.Int64  `tfsdk:"refresh"`
	Retry        types.Int64  `tfsdk:"retry"`
	Expire       types.Int64  `tfsdk:"expire"`
	Serial       types.String `tfsdk:"serial"`
	DNSSECStatus types.Bool   `tfsdk:"dnssec_status"`
}

func (r *dnsZoneResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_zone"
}

func (r *dnsZoneResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Zone-level DNS settings (TTL / SOA) of an existing netcup zone. " +
			"The zone itself is never created or deleted by this resource — `destroy` only drops it from state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Equals the domain.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"domain": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Zone (domain), e.g. `example.com`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"ttl": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Zone TTL in seconds.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"refresh": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "SOA refresh in seconds.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"retry": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "SOA retry in seconds.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"expire": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "SOA expire in seconds.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"serial": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SOA serial (managed by netcup).",
			},
			"dnssec_status": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether DNSSEC is enabled for the zone.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *dnsZoneResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireCCP(data, &resp.Diagnostics)
}

// apply writes the netcup zone into the model, keeping the configured domain.
func (m *dnsZoneModel) apply(z *client.DNSZone) {
	m.ID = m.Domain
	m.TTL = types.Int64Value(parseInt(z.TTL))
	m.Refresh = types.Int64Value(parseInt(z.Refresh))
	m.Retry = types.Int64Value(parseInt(z.Retry))
	m.Expire = types.Int64Value(parseInt(z.Expire))
	m.Serial = types.StringValue(z.Serial)
	m.DNSSECStatus = types.BoolValue(z.DNSSECStatus)
}

func parseInt(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

// desiredZone reads the current zone and overlays the values set in the plan.
func (r *dnsZoneResource) desiredZone(ctx context.Context, plan dnsZoneModel) (client.DNSZone, error) {
	cur, err := r.client.InfoDNSZone(ctx, plan.Domain.ValueString())
	if err != nil {
		return client.DNSZone{}, err
	}
	z := *cur
	if !plan.TTL.IsNull() && !plan.TTL.IsUnknown() {
		z.TTL = strconv.FormatInt(plan.TTL.ValueInt64(), 10)
	}
	if !plan.Refresh.IsNull() && !plan.Refresh.IsUnknown() {
		z.Refresh = strconv.FormatInt(plan.Refresh.ValueInt64(), 10)
	}
	if !plan.Retry.IsNull() && !plan.Retry.IsUnknown() {
		z.Retry = strconv.FormatInt(plan.Retry.ValueInt64(), 10)
	}
	if !plan.Expire.IsNull() && !plan.Expire.IsUnknown() {
		z.Expire = strconv.FormatInt(plan.Expire.ValueInt64(), 10)
	}
	return z, nil
}

func (r *dnsZoneResource) write(ctx context.Context, plan dnsZoneModel) (*dnsZoneModel, error) {
	desired, err := r.desiredZone(ctx, plan)
	if err != nil {
		return nil, err
	}
	updated, err := r.client.UpdateDNSZone(ctx, plan.Domain.ValueString(), desired)
	if err != nil {
		return nil, err
	}
	plan.apply(updated)
	return &plan, nil
}

func (r *dnsZoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsZoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.write(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error configuring DNS zone", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *dnsZoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.client.InfoDNSZone(ctx, state.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading DNS zone", err.Error())
		return
	}
	state.apply(cur)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *dnsZoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dnsZoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state, err := r.write(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating DNS zone", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *dnsZoneResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	// netcup has no API to delete a zone; it lives as long as the domain does.
	resp.Diagnostics.AddWarning(
		"Zone left untouched",
		"netcup_dns_zone only manages settings of an existing zone — the zone itself was not deleted.",
	)
}

func (r *dnsZoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
