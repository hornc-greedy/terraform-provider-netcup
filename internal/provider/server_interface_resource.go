// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
	_ resource.Resource                = (*serverInterfaceResource)(nil)
	_ resource.ResourceWithConfigure   = (*serverInterfaceResource)(nil)
	_ resource.ResourceWithImportState = (*serverInterfaceResource)(nil)
)

type serverInterfaceResource struct {
	client *scp.Client
}

// NewServerInterfaceResource is the resource factory.
func NewServerInterfaceResource() resource.Resource {
	return &serverInterfaceResource{}
}

type serverInterfaceResourceModel struct {
	ID           types.String   `tfsdk:"id"`
	ServerID     types.Int64    `tfsdk:"server_id"`
	VlanID       types.Int64    `tfsdk:"vlan_id"`
	Driver       types.String   `tfsdk:"driver"`
	Mac          types.String   `tfsdk:"mac"`
	SpeedInMBits types.Int64    `tfsdk:"speed_in_mbits"`
	Timeouts     timeouts.Value `tfsdk:"timeouts"`
}

func (r *serverInterfaceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_interface"
}

func (r *serverInterfaceResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A network interface of a netcup server in a VLAN (SCP REST API). Changes are asynchronous; the provider waits for completion. " +
			"Its addresses are in the `netcup_server_interface` data source, because other resources change their reverse DNS. Timeouts include waiting for other new interfaces of the server.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite id `server_id/mac`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"server_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Id of the server.",
				PlanModifiers:       []planmodifier.Int64{int64RequiresReplace()},
			},
			"vlan_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Id of the VLAN.",
				PlanModifiers:       []planmodifier.Int64{int64RequiresReplace()},
			},
			"driver": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Network driver, `VIRTIO`, `E1000`, `E1000E`, `RTL8139` or `VMXNET3`.",
				Validators:          []validator.String{stringvalidator.OneOf("VIRTIO", "E1000", "E1000E", "RTL8139", "VMXNET3")},
			},
			"mac": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "MAC address netcup assigned to the interface.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"speed_in_mbits": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Link speed in MBit/s.",
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{Create: true, Update: true, Delete: true}),
		},
	}
}

func (r *serverInterfaceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

// read stores the interface in the model, or reports false if it is gone. The
// interface operations do not return the VLAN, the server does. netcup can report
// no live info, then the known VLAN stays.
func (r *serverInterfaceResource) read(ctx context.Context, m *serverInterfaceResourceModel) (bool, error) {
	serverID := int(m.ServerID.ValueInt64())
	ifc, err := r.client.GetInterface(ctx, serverID, m.Mac.ValueString())
	if err != nil {
		if scp.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	server, err := r.client.GetServer(ctx, serverID)
	if err != nil {
		return false, err
	}
	if vlan, known := server.VlanOf(ifc.Mac); known && vlan != nil {
		m.VlanID = types.Int64Value(int64(*vlan))
	}
	m.ID = types.StringValue(fmt.Sprintf("%d/%s", m.ServerID.ValueInt64(), ifc.Mac))
	m.Mac = types.StringValue(ifc.Mac)
	// GET types the driver as a free string, POST and PUT as the upper case enum.
	m.Driver = types.StringValue(strings.ToUpper(ifc.Driver))
	m.SpeedInMBits = types.Int64Value(int64(ifc.SpeedInMBits))
	return true, nil
}

func (r *serverInterfaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serverInterfaceResourceModel
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
	mac, err := r.client.CreateInterface(ctx, int(plan.ServerID.ValueInt64()), int(plan.VlanID.ValueInt64()), plan.Driver.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating server interface", err.Error())
		if mac != "" {
			// Terraform taints the saved interface and replaces it at the next apply.
			plan.ID = types.StringValue(fmt.Sprintf("%d/%s", plan.ServerID.ValueInt64(), mac))
			plan.Mac = types.StringValue(mac)
			plan.SpeedInMBits = types.Int64Null()
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		}
		return
	}
	plan.Mac = types.StringValue(mac)
	found, err := r.read(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error reading server interface", err.Error())
		return
	}
	if !found {
		resp.Diagnostics.AddError("Error reading server interface", "netcup does not find the new interface "+mac)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverInterfaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serverInterfaceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := r.read(ctx, &state)
	if err != nil {
		resp.Diagnostics.AddError("Error reading server interface", err.Error())
		return
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *serverInterfaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serverInterfaceResourceModel
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
	if err := r.client.UpdateInterface(ctx, int(plan.ServerID.ValueInt64()), plan.Mac.ValueString(), plan.Driver.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating server interface", err.Error())
		return
	}
	found, err := r.read(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("Error reading server interface", err.Error())
		return
	}
	if !found {
		resp.Diagnostics.AddError("Error reading server interface", "netcup does not find the interface "+plan.Mac.ValueString())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serverInterfaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serverInterfaceResourceModel
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
	if err := r.client.DeleteInterface(ctx, int(state.ServerID.ValueInt64()), state.Mac.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting server interface", err.Error())
	}
}

// ImportState accepts only VLAN interfaces, because this resource would replace
// the public interface otherwise.
func (r *serverInterfaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || !macPattern.MatchString(parts[1]) {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format <server_id>/<mac>, e.g. 12345/00:00:5e:00:53:01")
		return
	}
	serverID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid server_id in import ID", err.Error())
		return
	}
	server, err := r.client.GetServer(ctx, int(serverID))
	if err != nil {
		resp.Diagnostics.AddError("Error reading server", err.Error())
		return
	}
	vlan, known := server.VlanOf(parts[1])
	if !known || vlan == nil {
		resp.Diagnostics.AddError("Not a VLAN interface", fmt.Sprintf("netcup reports no VLAN for the interface %s of server %d.", parts[1], serverID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), serverID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vlan_id"), int64(*vlan))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
