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
	_ resource.Resource                = (*serverSnapshotResource)(nil)
	_ resource.ResourceWithConfigure   = (*serverSnapshotResource)(nil)
	_ resource.ResourceWithImportState = (*serverSnapshotResource)(nil)
)

type serverSnapshotResource struct {
	client *scp.Client
}

// NewServerSnapshotResource is the resource factory.
func NewServerSnapshotResource() resource.Resource {
	return &serverSnapshotResource{}
}

type serverSnapshotModel struct {
	ID          types.String   `tfsdk:"id"`
	ServerID    types.Int64    `tfsdk:"server_id"`
	Name        types.String   `tfsdk:"name"`
	Description types.String   `tfsdk:"description"`
	Online      types.Bool     `tfsdk:"online"`
	State       types.String   `tfsdk:"state"`
	Timeouts    timeouts.Value `tfsdk:"timeouts"`
}

func (r *serverSnapshotResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_snapshot"
}

func (r *serverSnapshotResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A snapshot of a netcup server (SCP REST API). Create and delete are asynchronous; the provider waits for completion.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite id `server_id/name`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"server_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Server id the snapshot belongs to.",
				PlanModifiers:       []planmodifier.Int64{int64RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Snapshot name.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Snapshot description.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"online": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Whether to take an online snapshot. Defaults to `false`.",
				PlanModifiers:       []planmodifier.Bool{boolRequiresReplace()},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Snapshot state.",
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Delete: true,
			}),
		},
	}
}

func (r *serverSnapshotResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

func (r *serverSnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverSnapshotModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createTimeout, diags := plan.Timeouts.Create(ctx, 30*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()
	snap, err := r.client.CreateSnapshot(ctx, int(plan.ServerID.ValueInt64()),
		plan.Name.ValueString(), plan.Description.ValueString(), "", plan.Online.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Error creating snapshot", err.Error())
		return
	}
	plan.ID = types.StringValue(strconv.FormatInt(plan.ServerID.ValueInt64(), 10) + "/" + plan.Name.ValueString())
	if snap != nil {
		plan.State = types.StringValue(snap.State)
	} else {
		plan.State = types.StringValue("")
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverSnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverSnapshotModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	snap, err := r.client.GetSnapshot(ctx, int(state.ServerID.ValueInt64()), state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading snapshot", err.Error())
		return
	}
	if snap == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Description = optString(snap.Description)
	state.Online = types.BoolValue(snap.Online)
	state.State = types.StringValue(snap.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is unreachable (all writable attributes force replacement) but required.
func (r *serverSnapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverSnapshotModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverSnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serverSnapshotModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	deleteTimeout, diags := state.Timeouts.Delete(ctx, 15*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	if err := r.client.DeleteSnapshot(ctx, int(state.ServerID.ValueInt64()), state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting snapshot", err.Error())
	}
}

func (r *serverSnapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format <server_id>/<name>, e.g. 296155/daily")
		return
	}
	sid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid server_id in import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), sid)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
