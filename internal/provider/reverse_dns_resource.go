// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

var (
	_ resource.Resource                = (*reverseDNSResource)(nil)
	_ resource.ResourceWithConfigure   = (*reverseDNSResource)(nil)
	_ resource.ResourceWithImportState = (*reverseDNSResource)(nil)
)

type reverseDNSResource struct {
	client *scp.Client
}

// NewReverseDNSResource is the resource factory.
func NewReverseDNSResource() resource.Resource {
	return &reverseDNSResource{}
}

type reverseDNSModel struct {
	ID       types.String `tfsdk:"id"`
	IP       types.String `tfsdk:"ip"`
	Hostname types.String `tfsdk:"hostname"`
}

func (r *reverseDNSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_reverse_dns"
}

func (r *reverseDNSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reverse DNS (PTR) entry for an IPv4 or IPv6 address of a netcup server, " +
			"managed via the SCP REST API. The IP must already belong to one of your servers.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Equals the IP address.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"ip": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "IPv4 or IPv6 address to set the PTR for.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"hostname": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "PTR target hostname, e.g. `mail.example.com`.",
			},
		},
	}
}

func (r *reverseDNSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

func (r *reverseDNSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan reverseDNSModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.SetReverseDNS(ctx, plan.IP.ValueString(), plan.Hostname.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error setting reverse DNS", err.Error())
		return
	}
	plan.ID = plan.IP
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *reverseDNSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state reverseDNSModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hostname, err := r.client.GetReverseDNS(ctx, state.IP.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading reverse DNS", err.Error())
		return
	}
	if hostname == "" {
		// PTR cleared upstream -> drop from state.
		resp.State.RemoveResource(ctx)
		return
	}
	state.ID = state.IP
	state.Hostname = types.StringValue(hostname)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *reverseDNSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan reverseDNSModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.SetReverseDNS(ctx, plan.IP.ValueString(), plan.Hostname.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating reverse DNS", err.Error())
		return
	}
	plan.ID = plan.IP
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *reverseDNSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state reverseDNSModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteReverseDNS(ctx, state.IP.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting reverse DNS", err.Error())
	}
}

func (r *reverseDNSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
