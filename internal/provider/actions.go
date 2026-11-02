// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/action/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

// The patterns of ServerImageSetup and ServerUserImageSetup. RE2 lacks the
// lookahead of the hostname, so the length has its own validator.
var (
	hostnamePattern = regexp.MustCompile(`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`)
	usernamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)
	nonBlankPattern = regexp.MustCompile(`\S`)
)

// defaultActionTimeout bounds an action without a timeouts attribute.
const defaultActionTimeout = 60 * time.Minute

// actionTimeouts is the timeouts attribute every action config has.
type actionTimeouts struct {
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func (a actionTimeouts) invokeTimeout(ctx context.Context) (time.Duration, diag.Diagnostics) {
	return a.Timeouts.Invoke(ctx, defaultActionTimeout)
}

// actionConfig is the config of an action.
type actionConfig interface {
	invokeTimeout(context.Context) (time.Duration, diag.Diagnostics)
}

// scpAction is a Terraform action that runs one SCP operation.
type scpAction[M actionConfig] struct {
	client      *scp.Client
	typeName    string
	description string
	attributes  map[string]schema.Attribute
	run         func(context.Context, *scp.Client, M) error
	validate    func(context.Context, action.ValidateConfigRequest, *action.ValidateConfigResponse)
}

var (
	_ action.ActionWithConfigure      = (*scpAction[actionTimeouts])(nil)
	_ action.ActionWithValidateConfig = (*scpAction[actionTimeouts])(nil)
)

func (a *scpAction[M]) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	if a.validate != nil {
		a.validate(ctx, req, resp)
	}
}

func (a *scpAction[M]) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + a.typeName
}

func (a *scpAction[M]) Schema(ctx context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	attributes := maps.Clone(a.attributes)
	attributes["timeouts"] = timeouts.Attributes(ctx)
	resp.Schema = schema.Schema{MarkdownDescription: a.description, Attributes: attributes}
}

func (a *scpAction[M]) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	a.client = requireSCP(data, &resp.Diagnostics)
}

func (a *scpAction[M]) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg M
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	limit, diags := cfg.invokeTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	resp.SendProgress(action.InvokeProgressEvent{Message: "Running " + a.typeName})
	if err := a.run(ctx, a.client, cfg); err != nil {
		resp.Diagnostics.AddError("Error running "+a.typeName, err.Error())
	}
}

func actionServerID() schema.Int64Attribute {
	return schema.Int64Attribute{Required: true, MarkdownDescription: "Id of the server."}
}

func actionMac() schema.StringAttribute {
	return schema.StringAttribute{
		Required:            true,
		MarkdownDescription: "MAC address of the interface.",
		Validators:          []validator.String{stringvalidator.RegexMatches(macPattern, "must be a MAC address")},
	}
}

func actionSnapshotName() schema.StringAttribute {
	return schema.StringAttribute{Required: true, MarkdownDescription: "Name of the snapshot."}
}

func actionDiskName() schema.StringAttribute {
	return schema.StringAttribute{Required: true, MarkdownDescription: "Name of the disk, e.g. `vda`."}
}

// actionServerDisk is the config of an action on one disk.
type actionServerDisk struct {
	actionTimeouts
	ServerID types.Int64  `tfsdk:"server_id"`
	DiskName types.String `tfsdk:"disk_name"`
}

// NewServerDiskFormatAction is the action factory.
func NewServerDiskFormatAction() action.Action {
	return &scpAction[actionServerDisk]{
		typeName:    "server_disk_format",
		description: "Formats a disk of a netcup server (SCP REST API). The server must be in state `SHUTOFF` and the disk must have no snapshots. **All data on the disk is lost.**",
		attributes:  map[string]schema.Attribute{"server_id": actionServerID(), "disk_name": actionDiskName()},
		run: func(ctx context.Context, c *scp.Client, m actionServerDisk) error {
			return c.FormatDisk(ctx, int(m.ServerID.ValueInt64()), m.DiskName.ValueString())
		},
	}
}

// actionServerImageSetup is the config of netcup_server_image_setup.
type actionServerImageSetup struct {
	actionTimeouts
	ServerID                  types.Int64  `tfsdk:"server_id"`
	ImageFlavourID            types.Int64  `tfsdk:"image_flavour_id"`
	DiskName                  types.String `tfsdk:"disk_name"`
	RootPartitionFullDiskSize types.Bool   `tfsdk:"root_partition_full_disk_size"`
	Hostname                  types.String `tfsdk:"hostname"`
	Locale                    types.String `tfsdk:"locale"`
	Timezone                  types.String `tfsdk:"timezone"`
	AdditionalUserUsername    types.String `tfsdk:"additional_user_username"`
	AdditionalUserPassword    types.String `tfsdk:"additional_user_password"`
	SSHKeyIDs                 []int64      `tfsdk:"ssh_key_ids"`
	SSHPasswordAuthentication types.Bool   `tfsdk:"ssh_password_authentication"`
	CustomScript              types.String `tfsdk:"custom_script"`
	EmailToExecutingUser      types.Bool   `tfsdk:"email_to_executing_user"`
}

// NewServerImageSetupAction is the action factory.
func NewServerImageSetupAction() action.Action {
	return &scpAction[actionServerImageSetup]{
		typeName:    "server_image_setup",
		description: "Installs an image flavour on a netcup server (SCP REST API). **All data on the selected disk is lost.**",
		attributes: map[string]schema.Attribute{
			"server_id":                     actionServerID(),
			"image_flavour_id":              schema.Int64Attribute{Optional: true, MarkdownDescription: "Id of the flavour, from the `netcup_server_image_flavours` data source."},
			"disk_name":                     schema.StringAttribute{Optional: true, MarkdownDescription: "Name of the disk, e.g. `vda`."},
			"root_partition_full_disk_size": schema.BoolAttribute{Optional: true, MarkdownDescription: "Whether the root partition takes the whole disk."},
			"hostname": schema.StringAttribute{
				Optional: true, MarkdownDescription: "Hostname of the server.",
				Validators: []validator.String{
					stringvalidator.LengthAtMost(255),
					stringvalidator.RegexMatches(hostnamePattern, "must be a hostname"),
				},
			},
			"locale":   schema.StringAttribute{Optional: true, MarkdownDescription: "Locale, e.g. `en_US.UTF-8`."},
			"timezone": schema.StringAttribute{Optional: true, MarkdownDescription: "Time zone, e.g. `Europe/Berlin`."},
			"additional_user_username": schema.StringAttribute{
				Optional: true, MarkdownDescription: "Name of an additional user.",
				Validators: []validator.String{stringvalidator.RegexMatches(usernamePattern, "must start with a lowercase letter, followed by up to 30 lowercase letters, digits or underscores")},
			},
			"additional_user_password":    schema.StringAttribute{Optional: true, MarkdownDescription: "Password of the additional user. It cannot take an ephemeral value, so pass it through a `sensitive` variable. Prefer `ssh_key_ids`."},
			"ssh_key_ids":                 schema.ListAttribute{Optional: true, ElementType: types.Int64Type, MarkdownDescription: "Ids of the SSH keys to install."},
			"ssh_password_authentication": schema.BoolAttribute{Optional: true, MarkdownDescription: "Whether SSH accepts passwords."},
			"custom_script":               schema.StringAttribute{Optional: true, MarkdownDescription: "Script to run during the setup."},
			"email_to_executing_user":     schema.BoolAttribute{Optional: true, MarkdownDescription: "Whether netcup sends an e-mail to the user."},
		},
		run: func(ctx context.Context, c *scp.Client, m actionServerImageSetup) error {
			return c.SetupImage(ctx, int(m.ServerID.ValueInt64()), scp.ServerImageSetup{
				ImageFlavourID:            m.ImageFlavourID.ValueInt64Pointer(),
				DiskName:                  m.DiskName.ValueStringPointer(),
				RootPartitionFullDiskSize: m.RootPartitionFullDiskSize.ValueBoolPointer(),
				Hostname:                  m.Hostname.ValueStringPointer(),
				Locale:                    m.Locale.ValueStringPointer(),
				Timezone:                  m.Timezone.ValueStringPointer(),
				AdditionalUserUsername:    m.AdditionalUserUsername.ValueStringPointer(),
				AdditionalUserPassword:    m.AdditionalUserPassword.ValueStringPointer(),
				SSHKeyIDs:                 m.SSHKeyIDs,
				SSHPasswordAuthentication: m.SSHPasswordAuthentication.ValueBoolPointer(),
				CustomScript:              m.CustomScript.ValueStringPointer(),
				EmailToExecutingUser:      m.EmailToExecutingUser.ValueBoolPointer(),
			})
		},
	}
}

// actionServerUserImageSetup is the config of netcup_server_user_image_setup.
type actionServerUserImageSetup struct {
	actionTimeouts
	ServerID          types.Int64  `tfsdk:"server_id"`
	UserImageName     types.String `tfsdk:"user_image_name"`
	DiskName          types.String `tfsdk:"disk_name"`
	EmailNotification types.Bool   `tfsdk:"email_notification"`
}

// NewServerUserImageSetupAction is the action factory.
func NewServerUserImageSetupAction() action.Action {
	return &scpAction[actionServerUserImageSetup]{
		typeName:    "server_user_image_setup",
		description: "Installs an uploaded image on a netcup server (SCP REST API). **All data on the disk is lost.**",
		attributes: map[string]schema.Attribute{
			"server_id": actionServerID(),
			"user_image_name": schema.StringAttribute{
				Required: true, MarkdownDescription: "Name of the uploaded image.",
				Validators: []validator.String{stringvalidator.RegexMatches(nonBlankPattern, "must not be blank")},
			},
			"disk_name":          schema.StringAttribute{Optional: true, MarkdownDescription: "Name of the disk, e.g. `vda`."},
			"email_notification": schema.BoolAttribute{Optional: true, MarkdownDescription: "Whether netcup sends an e-mail when it is done."},
		},
		run: func(ctx context.Context, c *scp.Client, m actionServerUserImageSetup) error {
			return c.SetupUserImage(ctx, int(m.ServerID.ValueInt64()), scp.ServerUserImageSetup{
				UserImageName: m.UserImageName.ValueString(), DiskName: m.DiskName.ValueStringPointer(), EmailNotification: m.EmailNotification.ValueBoolPointer(),
			})
		},
	}
}

// actionStorageOptimization is the config of netcup_server_storage_optimization.
type actionStorageOptimization struct {
	actionTimeouts
	ServerID               types.Int64 `tfsdk:"server_id"`
	Disks                  []string    `tfsdk:"disks"`
	StartAfterOptimization types.Bool  `tfsdk:"start_after_optimization"`
}

// NewServerStorageOptimizationAction is the action factory.
func NewServerStorageOptimizationAction() action.Action {
	return &scpAction[actionStorageOptimization]{
		typeName:    "server_storage_optimization",
		description: "Optimizes the storage of a netcup server (SCP REST API).",
		attributes: map[string]schema.Attribute{
			"server_id":                actionServerID(),
			"disks":                    schema.ListAttribute{Optional: true, ElementType: types.StringType, MarkdownDescription: "Names of the disks."},
			"start_after_optimization": schema.BoolAttribute{Optional: true, MarkdownDescription: "Whether the server starts afterwards. netcup defaults to `false`."},
		},
		run: func(ctx context.Context, c *scp.Client, m actionStorageOptimization) error {
			return c.OptimizeStorage(ctx, int(m.ServerID.ValueInt64()), m.Disks, m.StartAfterOptimization.ValueBoolPointer())
		},
	}
}

// actionServerSnapshot is the config of an action on one snapshot.
type actionServerSnapshot struct {
	actionTimeouts
	ServerID types.Int64  `tfsdk:"server_id"`
	Name     types.String `tfsdk:"name"`
}

// NewServerSnapshotRevertAction is the action factory.
func NewServerSnapshotRevertAction() action.Action {
	return &scpAction[actionServerSnapshot]{
		typeName:    "server_snapshot_revert",
		description: "Reverts a netcup server to a snapshot (SCP REST API). **Changes since the snapshot are lost.**",
		attributes:  map[string]schema.Attribute{"server_id": actionServerID(), "name": actionSnapshotName()},
		run: func(ctx context.Context, c *scp.Client, m actionServerSnapshot) error {
			return c.RevertSnapshot(ctx, int(m.ServerID.ValueInt64()), m.Name.ValueString())
		},
	}
}

// NewServerSnapshotExportAction is the action factory.
func NewServerSnapshotExportAction() action.Action {
	return &scpAction[actionServerSnapshot]{
		typeName:    "server_snapshot_export",
		description: "Exports a snapshot of a netcup server (SCP REST API). Only offline snapshots can be exported and the server must be in state `SHUTOFF`. netcup includes one export per server, further ones are booked in the CCP.",
		attributes:  map[string]schema.Attribute{"server_id": actionServerID(), "name": actionSnapshotName()},
		run: func(ctx context.Context, c *scp.Client, m actionServerSnapshot) error {
			return c.ExportSnapshot(ctx, int(m.ServerID.ValueInt64()), m.Name.ValueString())
		},
	}
}

// actionServerInterface is the config of an action on one interface.
type actionServerInterface struct {
	actionTimeouts
	ServerID types.Int64  `tfsdk:"server_id"`
	Mac      types.String `tfsdk:"mac"`
}

// NewServerFirewallReapplyAction is the action factory.
func NewServerFirewallReapplyAction() action.Action {
	return &scpAction[actionServerInterface]{
		typeName:    "server_firewall_reapply",
		description: "Applies the firewall of an interface again (SCP REST API). netcup needs it if a policy update timed out during a long write operation, e.g. a storage optimization.",
		attributes:  map[string]schema.Attribute{"server_id": actionServerID(), "mac": actionMac()},
		run: func(ctx context.Context, c *scp.Client, m actionServerInterface) error {
			return c.ReapplyFirewall(ctx, int(m.ServerID.ValueInt64()), m.Mac.ValueString())
		},
	}
}

// NewServerFirewallRestoreCopiedPoliciesAction is the action factory.
func NewServerFirewallRestoreCopiedPoliciesAction() action.Action {
	return &scpAction[actionServerInterface]{
		typeName:    "server_firewall_restore_copied_policies",
		description: "Restores the copied firewall policies of an interface (SCP REST API).",
		attributes:  map[string]schema.Attribute{"server_id": actionServerID(), "mac": actionMac()},
		run: func(ctx context.Context, c *scp.Client, m actionServerInterface) error {
			return c.RestoreCopiedPolicies(ctx, int(m.ServerID.ValueInt64()), m.Mac.ValueString())
		},
	}
}

// actionTaskCancel is the config of netcup_task_cancel.
type actionTaskCancel struct {
	actionTimeouts
	UUID types.String `tfsdk:"uuid"`
}

// NewTaskCancelAction is the action factory.
func NewTaskCancelAction() action.Action {
	return &scpAction[actionTaskCancel]{
		typeName:    "task_cancel",
		description: "Asks netcup to cancel a running task (SCP REST API). The `netcup_tasks` data source shows the result.",
		attributes:  map[string]schema.Attribute{"uuid": schema.StringAttribute{Required: true, MarkdownDescription: "UUID of the task."}},
		run: func(ctx context.Context, c *scp.Client, m actionTaskCancel) error {
			return c.CancelTask(ctx, m.UUID.ValueString())
		},
	}
}

// actionServerState is the config of netcup_server_state.
type actionServerState struct {
	actionTimeouts
	ServerID    types.Int64  `tfsdk:"server_id"`
	State       types.String `tfsdk:"state"`
	StateOption types.String `tfsdk:"state_option"`
}

// NewServerStateAction is the action factory.
func NewServerStateAction() action.Action {
	return &scpAction[actionServerState]{
		typeName: "server_state",
		description: "Changes the power state of a netcup server once (SCP REST API), e.g. a restart with `state = \"ON\"` and `state_option = \"RESET\"`. " +
			"Use `power_state` of `netcup_server_settings` for a lasting state.",
		attributes: map[string]schema.Attribute{
			"server_id": actionServerID(),
			"state": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`ON`, `OFF` or `SUSPENDED`.",
				Validators:          []validator.String{stringvalidator.OneOf("ON", "OFF", "SUSPENDED")},
			},
			"state_option": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "`POWERCYCLE` or `RESET` for `ON`, `POWEROFF` for `OFF`.",
				Validators:          []validator.String{stringvalidator.OneOf("POWERCYCLE", "RESET", "POWEROFF")},
			},
		},
		validate: validateServerState,
		run: func(ctx context.Context, c *scp.Client, m actionServerState) error {
			return c.PatchServerState(ctx, int(m.ServerID.ValueInt64()), m.State.ValueString(), m.StateOption.ValueString())
		},
	}
}

// validateServerState enforces the rule of the specification which stateOption
// fits which state.
func validateServerState(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var state, option types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("state"), &state)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("state_option"), &option)...)
	if resp.Diagnostics.HasError() || state.IsUnknown() || option.IsNull() || option.IsUnknown() {
		return
	}
	valid := map[string][]string{"ON": {"POWERCYCLE", "RESET"}, "OFF": {"POWEROFF"}}
	if !slices.Contains(valid[state.ValueString()], option.ValueString()) {
		resp.Diagnostics.AddAttributeError(path.Root("state_option"), "Invalid state_option",
			fmt.Sprintf("%s does not fit state %s.", option.ValueString(), state.ValueString()))
	}
}
