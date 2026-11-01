// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

var (
	_ resource.Resource                = (*serverIsoResource)(nil)
	_ resource.ResourceWithConfigure   = (*serverIsoResource)(nil)
	_ resource.ResourceWithImportState = (*serverIsoResource)(nil)
)

type serverIsoResource struct {
	client *scp.Client
}

// NewServerIsoResource is the resource factory.
func NewServerIsoResource() resource.Resource {
	return &serverIsoResource{}
}

type serverIsoModel struct {
	ID                      types.String   `tfsdk:"id"`
	ServerID                types.Int64    `tfsdk:"server_id"`
	IsoID                   types.Int64    `tfsdk:"iso_id"`
	UserIsoName             types.String   `tfsdk:"user_iso_name"`
	ChangeBootDeviceToCdrom types.Bool     `tfsdk:"change_boot_device_to_cdrom"`
	Iso                     types.String   `tfsdk:"iso"`
	Timeouts                timeouts.Value `tfsdk:"timeouts"`
}

func (r *serverIsoResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_iso"
}

func (r *serverIsoResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An ISO attached to a netcup server (SCP REST API), one per server. Attaching is asynchronous; the provider waits for completion. " +
			"netcup does not return `iso_id`, `user_iso_name` or `change_boot_device_to_cdrom`, so an import leaves them empty and the next plan replaces the resource. " +
			"`lifecycle { ignore_changes = [iso_id, user_iso_name, change_boot_device_to_cdrom] }` avoids that.",
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
			"iso_id": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Id of the ISO, from the `netcup_server_iso_images` data source. Exactly one of `iso_id` and `user_iso_name` is needed.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
				Validators: []validator.Int64{int64validator.ExactlyOneOf(
					path.MatchRoot("iso_id"), path.MatchRoot("user_iso_name"),
				)},
			},
			"user_iso_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Name of an uploaded ISO.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"change_boot_device_to_cdrom": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether attaching changes the boot device to CD-ROM.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.RequiresReplace()},
			},
			"iso": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The attached ISO as netcup names it.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{Create: true}),
		},
	}
}

func (r *serverIsoResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

func (r *serverIsoResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverIsoModel
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
	attached, err := r.client.GetIso(ctx, serverID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading ISO", err.Error())
		return
	}
	if attached.IsoAttached {
		resp.Diagnostics.AddError("ISO already attached",
			fmt.Sprintf("Server %d has %q attached, and netcup would replace it without a message. Detach it or use terraform import.", serverID, attached.Iso))
		return
	}
	started, err := r.client.AttachIso(ctx, serverID, plan.IsoID.ValueInt64Pointer(), plan.UserIsoName.ValueStringPointer(), plan.ChangeBootDeviceToCdrom.ValueBoolPointer())
	plan.ID = types.StringValue(strconv.Itoa(serverID))
	plan.Iso = types.StringNull()
	if err != nil {
		resp.Diagnostics.AddError("Error attaching ISO", err.Error())
		if started {
			// Terraform taints the saved resource and replaces it at the next apply.
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		}
		return
	}
	iso, err := r.client.GetIso(ctx, serverID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading ISO", err.Error())
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		return
	}
	plan.Iso = optString(iso.Iso)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverIsoResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverIsoModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	iso, err := r.client.GetIso(ctx, int(state.ServerID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading ISO", err.Error())
		return
	}
	// Another ISO is drift the writable attributes cannot show, so the next plan
	// attaches the configured one again. An import has no known ISO yet.
	if !iso.IsoAttached || !state.Iso.IsNull() && state.Iso.ValueString() != iso.Iso {
		resp.State.RemoveResource(ctx)
		return
	}
	state.Iso = optString(iso.Iso)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update only stores a changed timeouts value.
func (r *serverIsoResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverIsoModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverIsoResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serverIsoModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DetachIso(ctx, int(state.ServerID.ValueInt64())); err != nil {
		resp.Diagnostics.AddError("Error detaching ISO", err.Error())
	}
}

func (r *serverIsoResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "Expected the numeric server id, e.g. 12345")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
