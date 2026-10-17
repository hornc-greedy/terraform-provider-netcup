// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strconv"
	"strings"

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
	_ resource.Resource                = (*failoverIPRouteResource)(nil)
	_ resource.ResourceWithConfigure   = (*failoverIPRouteResource)(nil)
	_ resource.ResourceWithImportState = (*failoverIPRouteResource)(nil)
)

type failoverIPRouteResource struct {
	client *scp.Client
}

// NewFailoverIPRouteResource is the resource factory.
func NewFailoverIPRouteResource() resource.Resource {
	return &failoverIPRouteResource{}
}

type failoverIPRouteModel struct {
	ID           types.String `tfsdk:"id"`
	FailoverIPID types.Int64  `tfsdk:"failover_ip_id"`
	ServerID     types.Int64  `tfsdk:"server_id"`
	IPv6         types.Bool   `tfsdk:"ipv6"`
}

func (r *failoverIPRouteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_failover_ip_route"
}

func (r *failoverIPRouteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Routing of a failover IP (v4 or v6) to a server (SCP REST API). " +
			"Routing is asynchronous; the provider waits for completion.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite id `v4|v6/failover_ip_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"failover_ip_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Id of the failover IP (from the `netcup_failover_ips` data source).",
				PlanModifiers:       []planmodifier.Int64{int64RequiresReplace()},
			},
			"server_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Server id to route the failover IP to.",
			},
			"ipv6": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Set to true for an IPv6 failover. Defaults to `false`.",
				PlanModifiers:       []planmodifier.Bool{boolRequiresReplace()},
			},
		},
	}
}

func (r *failoverIPRouteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

func (r *failoverIPRouteResource) route(ctx context.Context, plan failoverIPRouteModel) error {
	return r.client.RouteFailover(ctx, int(plan.FailoverIPID.ValueInt64()), int(plan.ServerID.ValueInt64()), plan.IPv6.ValueBool())
}

func famPrefix(v6 bool) string {
	if v6 {
		return "v6"
	}
	return "v4"
}

func (r *failoverIPRouteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan failoverIPRouteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.route(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Error routing failover IP", err.Error())
		return
	}
	plan.ID = types.StringValue(famPrefix(plan.IPv6.ValueBool()) + "/" + strconv.FormatInt(plan.FailoverIPID.ValueInt64(), 10))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *failoverIPRouteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state failoverIPRouteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id := int(state.FailoverIPID.ValueInt64())
	// A null ipv6 in state means IPv4 and must match the default.
	state.IPv6 = types.BoolValue(state.IPv6.ValueBool())
	if state.IPv6.ValueBool() {
		ips, err := r.client.ListFailoverIPv6(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Error reading failover IPv6", err.Error())
			return
		}
		for _, ip := range ips {
			if ip.ID == id && ip.Server != nil {
				state.ServerID = types.Int64Value(int64(ip.Server.ID))
			}
		}
	} else {
		ips, err := r.client.ListFailoverIPv4(ctx)
		if err != nil {
			resp.Diagnostics.AddError("Error reading failover IPv4", err.Error())
			return
		}
		for _, ip := range ips {
			if ip.ID == id && ip.Server != nil {
				state.ServerID = types.Int64Value(int64(ip.Server.ID))
			}
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *failoverIPRouteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan failoverIPRouteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.route(ctx, plan); err != nil {
		resp.Diagnostics.AddError("Error re-routing failover IP", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete leaves the routing as-is: a failover IP is always routed somewhere and
// there is no "unroute" operation. Removing the resource just drops it from
// state.
func (r *failoverIPRouteResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Failover routing left in place",
		"netcup has no unroute operation; the failover IP keeps its current routing. The resource was removed from state only.",
	)
}

func (r *failoverIPRouteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || (parts[0] != "v4" && parts[0] != "v6") || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format v4/<id> or v6/<id>, e.g. v4/1234")
		return
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid failover_ip_id in import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ipv6"), parts[0] == "v6")...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("failover_ip_id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
