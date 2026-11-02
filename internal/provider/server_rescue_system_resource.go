// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	ephemeralschema "github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

var (
	_ resource.Resource                        = (*serverRescueSystemResource)(nil)
	_ resource.ResourceWithConfigure           = (*serverRescueSystemResource)(nil)
	_ resource.ResourceWithImportState         = (*serverRescueSystemResource)(nil)
	_ ephemeral.EphemeralResource              = (*serverRescueSystemEphemeralResource)(nil)
	_ ephemeral.EphemeralResourceWithConfigure = (*serverRescueSystemEphemeralResource)(nil)
)

type serverRescueSystemResource struct {
	client *scp.Client
}

// NewServerRescueSystemResource is the resource factory.
func NewServerRescueSystemResource() resource.Resource {
	return &serverRescueSystemResource{}
}

type serverRescueSystemModel struct {
	ID       types.String   `tfsdk:"id"`
	ServerID types.Int64    `tfsdk:"server_id"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func (r *serverRescueSystemResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_rescue_system"
}

func (r *serverRescueSystemResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The active rescue system of a netcup server (SCP REST API), one per server. Changes are asynchronous; the provider waits for completion. " +
			"netcup activates it only on a shut down server and shuts the server down to deactivate it, so creating and destroying the resource " +
			"shuts down a running server and starts it again, into the rescue system or back into its own system. " +
			"The password is in the ephemeral `netcup_server_rescue_system`, so it stays out of state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Equals `server_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"server_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Id of the server.",
				PlanModifiers:       []planmodifier.Int64{int64RequiresReplace()},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{Create: true, Delete: true}),
		},
	}
}

func (r *serverRescueSystemResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

func (r *serverRescueSystemResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverRescueSystemModel
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
	serverID := int(plan.ServerID.ValueInt64())
	status, err := r.client.GetRescueSystem(ctx, serverID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading rescue system", err.Error())
		return
	}
	if status.Active != nil && *status.Active {
		resp.Diagnostics.AddError("Rescue system already active",
			fmt.Sprintf("Server %d has an active rescue system. Deactivate it or use terraform import to manage it.", serverID))
		return
	}
	server, err := r.client.GetServerLive(ctx, serverID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading server", err.Error())
		return
	}
	if server.LiveInfo.State != "SHUTOFF" {
		if err := r.client.PatchServerState(ctx, serverID, "OFF", ""); err != nil {
			resp.Diagnostics.AddError("Error shutting down server", err.Error())
			return
		}
	}
	started, err := r.client.ActivateRescueSystem(ctx, serverID)
	plan.ID = types.StringValue(strconv.Itoa(serverID))
	if err != nil {
		resp.Diagnostics.AddError("Error activating rescue system", err.Error())
	}
	if err == nil || started {
		// After an error Terraform taints the saved resource and replaces it at the next apply.
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	}
	if server.LiveInfo.State == "RUNNING" {
		if err := r.client.PatchServerState(ctx, serverID, "ON", ""); err != nil {
			resp.Diagnostics.AddError("Error starting server", err.Error())
		}
	}
}

func (r *serverRescueSystemResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverRescueSystemModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	status, err := r.client.GetRescueSystem(ctx, int(state.ServerID.ValueInt64()))
	if err != nil {
		if scp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading rescue system", err.Error())
		return
	}
	if status.Active == nil || !*status.Active {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update only stores a changed timeouts value.
func (r *serverRescueSystemResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverRescueSystemModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverRescueSystemResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serverRescueSystemModel
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
	serverID := int(state.ServerID.ValueInt64())
	server, err := r.client.GetServerLive(ctx, serverID)
	if scp.IsNotFound(err) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading server", err.Error())
		return
	}
	err = r.client.DeactivateRescueSystem(ctx, serverID)
	if scp.HasCode(err, scp.RescueSystemNotActive) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error deactivating rescue system", err.Error())
		return
	}
	if server.LiveInfo.State == "RUNNING" {
		if err := r.client.PatchServerState(ctx, serverID, "ON", ""); err != nil {
			resp.Diagnostics.AddError("Error starting server", err.Error())
		}
	}
}

func (r *serverRescueSystemResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "Expected the numeric server id, e.g. 12345")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// serverRescueSystemEphemeralResource reads the status with the password of the
// rescue system, which stays out of plan and state.
type serverRescueSystemEphemeralResource struct {
	client *scp.Client
}

// NewServerRescueSystemEphemeralResource is the ephemeral resource factory.
func NewServerRescueSystemEphemeralResource() ephemeral.EphemeralResource {
	return &serverRescueSystemEphemeralResource{}
}

func (r *serverRescueSystemEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_rescue_system"
}

func (r *serverRescueSystemEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = ephemeralschema.Schema{
		MarkdownDescription: "The status of the rescue system of a netcup server with its password (SCP REST API). It needs Terraform 1.10 or newer.",
		Attributes: map[string]ephemeralschema.Attribute{
			"server_id": ephemeralschema.Int64Attribute{Required: true, MarkdownDescription: "Id of the server."},
			"active":    ephemeralschema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the rescue system is active."},
			"password":  ephemeralschema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Password of the rescue system, null while it is inactive."},
		},
	}
}

func (r *serverRescueSystemEphemeralResource) Configure(_ context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

func (r *serverRescueSystemEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var cfg struct {
		ServerID types.Int64  `tfsdk:"server_id"`
		Active   types.Bool   `tfsdk:"active"`
		Password types.String `tfsdk:"password"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	status, err := r.client.GetRescueSystem(ctx, int(cfg.ServerID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading rescue system", err.Error())
		return
	}
	cfg.Active = types.BoolPointerValue(status.Active)
	cfg.Password = optString(status.Password)
	resp.Diagnostics.Append(resp.Result.Set(ctx, cfg)...)
}
