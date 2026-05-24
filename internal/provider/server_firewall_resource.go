// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

var (
	_ resource.Resource                = (*serverFirewallResource)(nil)
	_ resource.ResourceWithConfigure   = (*serverFirewallResource)(nil)
	_ resource.ResourceWithImportState = (*serverFirewallResource)(nil)
)

type serverFirewallResource struct {
	client *scp.Client
}

// NewServerFirewallResource is the resource factory.
func NewServerFirewallResource() resource.Resource {
	return &serverFirewallResource{}
}

type serverFirewallModel struct {
	ID        types.String   `tfsdk:"id"`
	ServerID  types.Int64    `tfsdk:"server_id"`
	Mac       types.String   `tfsdk:"mac"`
	PolicyIDs types.List     `tfsdk:"policy_ids"`
	Active    types.Bool     `tfsdk:"active"`
	Timeouts  timeouts.Value `tfsdk:"timeouts"`
}

func (r *serverFirewallResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_firewall"
}

func (r *serverFirewallResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Assignment of firewall policies to a server interface (SCP REST API). " +
			"Assigning is asynchronous; the provider waits for completion. Deleting this resource clears the assignment.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite id `server_id/mac`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"server_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Server id.",
				PlanModifiers:       []planmodifier.Int64{int64RequiresReplace()},
			},
			"mac": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "MAC address of the interface.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"policy_ids": schema.ListAttribute{
				Required:            true,
				ElementType:         types.Int64Type,
				MarkdownDescription: "Firewall policy ids to assign to the interface.",
			},
			"active": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the firewall is active. Defaults to `true`.",
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *serverFirewallResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

func (m serverFirewallModel) policyIDs(ctx context.Context, da *diagAppender) []int {
	if m.PolicyIDs.IsNull() || m.PolicyIDs.IsUnknown() {
		return nil
	}
	var ids []int64
	da.diags.Append(m.PolicyIDs.ElementsAs(ctx, &ids, false)...)
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		out = append(out, int(id))
	}
	return out
}

func (r *serverFirewallResource) set(ctx context.Context, plan serverFirewallModel, diags *diagAppender) error {
	return r.client.SetServerFirewall(ctx,
		int(plan.ServerID.ValueInt64()), plan.Mac.ValueString(),
		plan.policyIDs(ctx, diags), plan.Active.ValueBool())
}

func (r *serverFirewallResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverFirewallModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createTimeout, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()
	da := &diagAppender{diags: &resp.Diagnostics}
	if err := r.set(ctx, plan, da); err != nil {
		resp.Diagnostics.AddError("Error assigning firewall", err.Error())
		return
	}
	plan.ID = types.StringValue(strconv.FormatInt(plan.ServerID.ValueInt64(), 10) + "/" + plan.Mac.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverFirewallResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverFirewallModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	fw, err := r.client.GetServerFirewall(ctx, int(state.ServerID.ValueInt64()), state.Mac.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading server firewall", err.Error())
		return
	}
	ids := make([]int64, 0, len(fw.UserPolicies))
	for _, p := range fw.UserPolicies {
		ids = append(ids, int64(p.ID))
	}
	list, d := types.ListValueFrom(ctx, types.Int64Type, ids)
	resp.Diagnostics.Append(d...)
	state.PolicyIDs = list
	state.Active = types.BoolValue(fw.Active)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *serverFirewallResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverFirewallModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateTimeout, diags := plan.Timeouts.Update(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()
	da := &diagAppender{diags: &resp.Diagnostics}
	if err := r.set(ctx, plan, da); err != nil {
		resp.Diagnostics.AddError("Error updating firewall assignment", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverFirewallResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serverFirewallModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteTimeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	// Clearing the assignment: no policies, firewall inactive.
	if err := r.client.SetServerFirewall(ctx, int(state.ServerID.ValueInt64()), state.Mac.ValueString(), nil, false); err != nil {
		resp.Diagnostics.AddError("Error clearing firewall assignment", err.Error())
	}
}

func (r *serverFirewallResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format <server_id>/<mac>, e.g. 296155/ba:ad:7a:1f:d3:68")
		return
	}
	sid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid server_id in import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), sid)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
