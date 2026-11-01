// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

var (
	_ resource.Resource                = (*userVlanResource)(nil)
	_ resource.ResourceWithConfigure   = (*userVlanResource)(nil)
	_ resource.ResourceWithImportState = (*userVlanResource)(nil)
)

type userVlanResource struct {
	client *scp.Client
}

// NewUserVlanResource is the resource factory.
func NewUserVlanResource() resource.Resource {
	return &userVlanResource{}
}

type userVlanModel struct {
	ID             types.String   `tfsdk:"id"`
	VlanID         types.Int64    `tfsdk:"vlan_id"`
	Name           types.String   `tfsdk:"name"`
	User           types.Object   `tfsdk:"user"`
	Site           types.Object   `tfsdk:"site"`
	BandwidthClass types.Object   `tfsdk:"bandwidth_class"`
	Timeouts       timeouts.Value `tfsdk:"timeouts"`
}

var (
	userMinimalTypes = map[string]attr.Type{
		"id": types.Int64Type, "username": types.StringType, "firstname": types.StringType,
		"lastname": types.StringType, "email": types.StringType, "company": types.StringType,
	}
	vlanSiteTypes           = map[string]attr.Type{"id": types.Int64Type, "name": types.StringType, "city": types.StringType}
	vlanBandwidthClassTypes = map[string]attr.Type{"id": types.Int64Type, "name": types.StringType, "speed_in_mbit": types.Int64Type}
)

// computedObject is the schema of a nested object netcup returns, which PUT does
// not change.
func computedObject(description string, attributes map[string]schema.Attribute) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: description,
		Attributes:          attributes,
		PlanModifiers:       []planmodifier.Object{objectplanmodifier.UseStateForUnknown()},
	}
}

func computedInt64(description string) schema.Int64Attribute {
	return schema.Int64Attribute{Computed: true, MarkdownDescription: description}
}

func computedString(description string) schema.StringAttribute {
	return schema.StringAttribute{Computed: true, MarkdownDescription: description}
}

func (r *userVlanResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_vlan"
}

func (r *userVlanResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The name of an existing VLAN of the netcup SCP user. netcup cannot create or delete VLANs through the API, so `destroy` only drops the resource from state. netcup answers with an error unless the server is in state SHUTOFF, so the provider sends no change if the name is already right.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Equals `vlan_id`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"vlan_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Id of the VLAN.",
				PlanModifiers:       []planmodifier.Int64{int64RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name of the VLAN.",
				Validators:          []validator.String{stringvalidator.LengthBetween(1, 255)},
			},
			"user": computedObject("User the VLAN belongs to.", map[string]schema.Attribute{
				"id":        computedInt64("Id of the user."),
				"username":  computedString("User name."),
				"firstname": computedString("First name."),
				"lastname":  computedString("Last name."),
				"email":     computedString("E-mail address."),
				"company":   computedString("Company."),
			}),
			"site": computedObject("Site of the VLAN.", map[string]schema.Attribute{
				"id":   computedInt64("Id of the site."),
				"name": computedString("Name of the site."),
				"city": computedString("City of the site."),
			}),
			"bandwidth_class": computedObject("Bandwidth class of the VLAN.", map[string]schema.Attribute{
				"id":            computedInt64("Id of the class."),
				"name":          computedString("Name of the class."),
				"speed_in_mbit": computedInt64("Speed in MBit/s."),
			}),
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{Create: true, Update: true}),
		},
	}
}

func (r *userVlanResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

// write gives the VLAN the planned name, if it has another one, and stores what
// netcup returns.
func (r *userVlanResource) write(ctx context.Context, plan *userVlanModel) diag.Diagnostics {
	var diags diag.Diagnostics
	vlan, err := r.client.GetUserVlan(ctx, plan.VlanID.ValueInt64())
	if err == nil && vlan.Name != plan.Name.ValueString() {
		vlan, err = r.client.UpdateUserVlan(ctx, plan.VlanID.ValueInt64(), plan.Name.ValueString())
	}
	if err != nil {
		diags.AddError("Error naming VLAN", err.Error())
		return diags
	}
	plan.ID = types.StringValue(strconv.FormatInt(plan.VlanID.ValueInt64(), 10))
	plan.Name = types.StringValue(vlan.Name)
	diags.Append(plan.setObjects(ctx, vlan)...)
	return diags
}

// setObjects stores the nested objects of the VLAN.
func (m *userVlanModel) setObjects(ctx context.Context, vlan *scp.VLan) diag.Diagnostics {
	var diags diag.Diagnostics
	converted := vlanToModel(vlan)
	object := func(types_ map[string]attr.Type, value any) types.Object {
		out, d := types.ObjectValueFrom(ctx, types_, value)
		diags.Append(d...)
		return out
	}
	m.User = object(userMinimalTypes, converted.User)
	m.Site = object(vlanSiteTypes, converted.Site)
	m.BandwidthClass = object(vlanBandwidthClassTypes, converted.BandwidthClass)
	return diags
}

func (r *userVlanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userVlanModel
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
	resp.Diagnostics.Append(r.write(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *userVlanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userVlanModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vlan, err := r.client.GetUserVlan(ctx, state.VlanID.ValueInt64())
	if err != nil {
		if scp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading VLAN", err.Error())
		return
	}
	state.Name = optString(vlan.Name)
	resp.Diagnostics.Append(state.setObjects(ctx, vlan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *userVlanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userVlanModel
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
	resp.Diagnostics.Append(r.write(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *userVlanResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"VLAN left untouched",
		"netcup cannot delete VLANs through the API. The VLAN keeps its name and the resource was removed from state only.",
	)
}

func (r *userVlanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "Expected the numeric VLAN id, e.g. 100")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vlan_id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
