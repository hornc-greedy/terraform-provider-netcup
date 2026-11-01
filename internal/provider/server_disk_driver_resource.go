// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

var (
	_ resource.Resource                = (*serverDiskDriverResource)(nil)
	_ resource.ResourceWithConfigure   = (*serverDiskDriverResource)(nil)
	_ resource.ResourceWithImportState = (*serverDiskDriverResource)(nil)
)

type serverDiskDriverResource struct {
	client *scp.Client
}

// NewServerDiskDriverResource is the resource factory.
func NewServerDiskDriverResource() resource.Resource {
	return &serverDiskDriverResource{}
}

type serverDiskDriverModel struct {
	ID       types.String   `tfsdk:"id"`
	ServerID types.Int64    `tfsdk:"server_id"`
	Driver   types.String   `tfsdk:"driver"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func (r *serverDiskDriverResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_disk_driver"
}

func (r *serverDiskDriverResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The storage driver of the disks of an existing netcup server (SCP REST API), one per server. netcup sets it for all disks at once. " +
			"Changes are asynchronous; the provider waits for completion. `destroy` only drops the resource from state and the disks keep their driver.",
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
			"driver": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Storage driver, `VIRTIO`, `VIRTIO_SCSI`, `IDE` or `SATA`. `netcup_server_disk_supported_drivers` lists those of the server.",
				Validators:          []validator.String{stringvalidator.OneOf("VIRTIO", "VIRTIO_SCSI", "IDE", "SATA")},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{Create: true, Update: true}),
		},
	}
}

func (r *serverDiskDriverResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

// write sets the driver if a disk has another one.
func (r *serverDiskDriverResource) write(ctx context.Context, plan *serverDiskDriverModel) error {
	id := int(plan.ServerID.ValueInt64())
	disks, err := r.client.ListDisks(ctx, id)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(disks, func(d scp.Disk) bool { return d.StorageDriver != "" && d.StorageDriver != plan.Driver.ValueString() }) {
		if err := r.client.PatchDiskDriver(ctx, id, plan.Driver.ValueString()); err != nil {
			return err
		}
	}
	plan.ID = types.StringValue(strconv.Itoa(id))
	return nil
}

func (r *serverDiskDriverResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverDiskDriverModel
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
	if err := r.write(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error setting disk driver", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read reports the first disk with another driver than the state, so that the
// plan shows the difference.
func (r *serverDiskDriverResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverDiskDriverModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	disks, err := r.client.ListDisks(ctx, int(state.ServerID.ValueInt64()))
	if err != nil {
		// The specification lists no 404 here, GET of the server has one.
		if scp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading disks", err.Error())
		return
	}
	for _, d := range disks {
		if d.StorageDriver != "" && d.StorageDriver != state.Driver.ValueString() {
			state.Driver = types.StringValue(d.StorageDriver)
			break
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *serverDiskDriverResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverDiskDriverModel
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
	if err := r.write(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error setting disk driver", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverDiskDriverResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Disk driver left untouched",
		"netcup has no default to return to. The disks keep their driver and the resource was removed from state only.",
	)
}

func (r *serverDiskDriverResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "Expected the numeric server id, e.g. 12345")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
