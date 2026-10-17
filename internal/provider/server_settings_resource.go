// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

var (
	_ resource.Resource                = (*serverSettingsResource)(nil)
	_ resource.ResourceWithConfigure   = (*serverSettingsResource)(nil)
	_ resource.ResourceWithImportState = (*serverSettingsResource)(nil)
)

type serverSettingsResource struct {
	client *scp.Client
}

// NewServerSettingsResource is the resource factory.
func NewServerSettingsResource() resource.Resource {
	return &serverSettingsResource{}
}

type serverSettingsModel struct {
	ID                types.String   `tfsdk:"id"`
	ServerID          types.Int64    `tfsdk:"server_id"`
	Hostname          types.String   `tfsdk:"hostname"`
	Nickname          types.String   `tfsdk:"nickname"`
	Autostart         types.Bool     `tfsdk:"autostart"`
	UEFI              types.Bool     `tfsdk:"uefi"`
	Bootorder         types.List     `tfsdk:"bootorder"`
	OSOptimization    types.String   `tfsdk:"os_optimization"`
	CPUSockets        types.Int64    `tfsdk:"cpu_sockets"`
	CPUCoresPerSocket types.Int64    `tfsdk:"cpu_cores_per_socket"`
	KeyboardLayout    types.String   `tfsdk:"keyboard_layout"`
	BackupTime        types.String   `tfsdk:"backup_time"`
	PowerState        types.String   `tfsdk:"power_state"`
	Timeouts          timeouts.Value `tfsdk:"timeouts"`
}

func (r *serverSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_settings"
}

func (r *serverSettingsResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Settings of an existing netcup server (SCP REST API). Only attributes set in the " +
			"configuration are managed. netcup cannot create or delete servers, so `destroy` only drops the " +
			"resource from state.",
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
			"hostname": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Hostname of the server.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"nickname": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Nickname shown in the SCP.",
				Validators:          []validator.String{stringvalidator.LengthAtMost(255)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"autostart": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the server starts when its host starts.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"uefi": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the server boots with UEFI.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"bootorder": schema.ListAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Boot devices in order, each one of `HDD`, `CDROM`, `NETWORK`.",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
					listvalidator.ValueStringsAre(stringvalidator.OneOf("HDD", "CDROM", "NETWORK")),
				},
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			},
			"os_optimization": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Operating system the virtual hardware is tuned for: `LINUX`, `WINDOWS`, `BSD`, `LINUX_LEGACY` or `UNKNOWN`.",
				Validators:          []validator.String{stringvalidator.OneOf("LINUX", "WINDOWS", "BSD", "LINUX_LEGACY", "UNKNOWN")},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cpu_sockets": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Number of CPU sockets. Set together with `cpu_cores_per_socket`.",
				Validators:          []validator.Int64{int64validator.AlsoRequires(path.MatchRoot("cpu_cores_per_socket"))},
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"cpu_cores_per_socket": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Number of CPU cores per socket. Set together with `cpu_sockets`.",
				Validators:          []validator.Int64{int64validator.AlsoRequires(path.MatchRoot("cpu_sockets"))},
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"keyboard_layout": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Keyboard layout of the server console.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"backup_time": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Daily backup start as local time with UTC offset, e.g. `01:30:00+02:00`. Negative offsets are not supported.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"power_state": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "`ON`, `OFF` or `SUSPENDED`. Changing it starts, shuts down or suspends the server.",
				Validators:          []validator.String{stringvalidator.OneOf("ON", "OFF", "SUSPENDED")},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
			}),
		},
	}
}

func (r *serverSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

// powerState maps the libvirt state of serverLiveInfo to the values the PATCH
// endpoint accepts.
func powerState(state string) string {
	switch state {
	case "RUNNING", "BLOCKED":
		return "ON"
	case "PAUSED", "PMSUSPENDED":
		return "SUSPENDED"
	}
	return "OFF"
}

// apply writes the netcup server into the model.
func (m *serverSettingsModel) apply(ctx context.Context, s *scp.Server, da *diagAppender) {
	live := s.LiveInfo
	m.ID = types.StringValue(strconv.Itoa(s.ID))
	m.Hostname = types.StringValue(s.Hostname)
	m.Nickname = types.StringValue(s.Nickname)
	m.Autostart = types.BoolValue(live.Autostart)
	m.UEFI = types.BoolValue(live.UEFI)
	bootorder, d := types.ListValueFrom(ctx, types.StringType, live.Bootorder)
	da.diags.Append(d...)
	m.Bootorder = bootorder
	m.OSOptimization = types.StringValue(live.OSOptimization)
	m.CPUSockets = types.Int64Value(int64(live.Sockets))
	m.CPUCoresPerSocket = types.Int64Value(int64(live.CoresPerSocket))
	m.KeyboardLayout = types.StringValue(live.KeyboardLayout)
	m.BackupTime = types.StringValue(live.BackupTime)
	m.PowerState = types.StringValue(powerState(live.State))
}

// configured reports whether the plan holds a value for the attribute.
func configured(v attr.Value) bool {
	return !v.IsNull() && !v.IsUnknown()
}

// patches returns the PATCH bodies that move cur to the planned settings. netcup
// takes one attribute per request. Shutting down comes first and starting last,
// so settings that need a stopped server apply in between.
func (m serverSettingsModel) patches(ctx context.Context, cur *scp.Server, da *diagAppender) []map[string]any {
	live := cur.LiveInfo
	var out []map[string]any
	add := func(key string, value any) {
		out = append(out, map[string]any{key: value})
	}
	curPower := powerState(live.State)
	if configured(m.PowerState) && m.PowerState.ValueString() == "OFF" && curPower != "OFF" {
		add("state", "OFF")
	}
	if configured(m.Hostname) && m.Hostname.ValueString() != cur.Hostname {
		add("hostname", m.Hostname.ValueString())
	}
	if configured(m.Nickname) && m.Nickname.ValueString() != cur.Nickname {
		add("nickname", m.Nickname.ValueString())
	}
	if configured(m.Autostart) && m.Autostart.ValueBool() != live.Autostart {
		add("autostart", m.Autostart.ValueBool())
	}
	if configured(m.UEFI) && m.UEFI.ValueBool() != live.UEFI {
		add("uefi", m.UEFI.ValueBool())
	}
	if configured(m.Bootorder) {
		var order []string
		da.diags.Append(m.Bootorder.ElementsAs(ctx, &order, false)...)
		if !slices.Equal(order, live.Bootorder) {
			add("bootorder", order)
		}
	}
	if configured(m.OSOptimization) && m.OSOptimization.ValueString() != live.OSOptimization {
		add("os_optimization", m.OSOptimization.ValueString())
	}
	if configured(m.CPUSockets) && configured(m.CPUCoresPerSocket) &&
		(int(m.CPUSockets.ValueInt64()) != live.Sockets || int(m.CPUCoresPerSocket.ValueInt64()) != live.CoresPerSocket) {
		add("cpuTopology", map[string]int64{
			"socketCount":         m.CPUSockets.ValueInt64(),
			"coresPerSocketCount": m.CPUCoresPerSocket.ValueInt64(),
		})
	}
	if configured(m.KeyboardLayout) && m.KeyboardLayout.ValueString() != live.KeyboardLayout {
		add("keyboardLayout", m.KeyboardLayout.ValueString())
	}
	if configured(m.BackupTime) && m.BackupTime.ValueString() != live.BackupTime {
		add("backupTime", m.BackupTime.ValueString())
	}
	if configured(m.PowerState) && m.PowerState.ValueString() != "OFF" && m.PowerState.ValueString() != curPower {
		add("state", m.PowerState.ValueString())
	}
	return out
}

// write applies the planned settings and reads the result back.
func (r *serverSettingsResource) write(ctx context.Context, plan *serverSettingsModel, da *diagAppender) error {
	id := int(plan.ServerID.ValueInt64())
	cur, err := r.client.GetServerLive(ctx, id)
	if err != nil {
		return err
	}
	for _, p := range plan.patches(ctx, cur, da) {
		if err := r.client.PatchServer(ctx, id, p); err != nil {
			return err
		}
	}
	updated, err := r.client.GetServerLive(ctx, id)
	if err != nil {
		return err
	}
	plan.apply(ctx, updated, da)
	return nil
}

func (r *serverSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	createTimeout, diags := plan.Timeouts.Create(ctx, 20*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()
	da := &diagAppender{diags: &resp.Diagnostics}
	if err := r.write(ctx, &plan, da); err != nil {
		resp.Diagnostics.AddError("Error applying server settings", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverSettingsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client.GetServerLive(ctx, int(state.ServerID.ValueInt64()))
	if err != nil {
		if scp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading server settings", err.Error())
		return
	}
	state.apply(ctx, s, &diagAppender{diags: &resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *serverSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverSettingsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updateTimeout, diags := plan.Timeouts.Update(ctx, 20*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()
	da := &diagAppender{diags: &resp.Diagnostics}
	if err := r.write(ctx, &plan, da); err != nil {
		resp.Diagnostics.AddError("Error updating server settings", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverSettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Server left untouched",
		"netcup cannot delete servers through the API. The server keeps its current settings and the resource was removed from state only.",
	)
}

func (r *serverSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "Expected the numeric server id, e.g. 296155")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
