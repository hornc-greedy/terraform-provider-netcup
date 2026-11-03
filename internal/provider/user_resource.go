// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	_ resource.Resource                = (*userResource)(nil)
	_ resource.ResourceWithConfigure   = (*userResource)(nil)
	_ resource.ResourceWithImportState = (*userResource)(nil)
)

type userResource struct {
	client *scp.Client
}

// NewUserResource is the resource factory.
func NewUserResource() resource.Resource {
	return &userResource{}
}

type userModel struct {
	ID                              types.Int64    `tfsdk:"id"`
	Username                        types.String   `tfsdk:"username"`
	Firstname                       types.String   `tfsdk:"firstname"`
	Lastname                        types.String   `tfsdk:"lastname"`
	Email                           types.String   `tfsdk:"email"`
	Company                         types.String   `tfsdk:"company"`
	Language                        types.String   `tfsdk:"language"`
	TimeZone                        types.String   `tfsdk:"time_zone"`
	ShowNickname                    types.Bool     `tfsdk:"show_nickname"`
	PasswordlessMode                types.Bool     `tfsdk:"passwordless_mode"`
	APIIPLoginRestrictions          types.String   `tfsdk:"api_ip_login_restrictions"`
	PasswordWO                      types.String   `tfsdk:"password_wo"`
	OldPasswordWO                   types.String   `tfsdk:"old_password_wo"`
	PasswordWOVersion               types.Int64    `tfsdk:"password_wo_version"`
	SoapWebservicePasswordWO        types.String   `tfsdk:"soap_webservice_password_wo"`
	SoapWebservicePasswordWOVersion types.Int64    `tfsdk:"soap_webservice_password_wo_version"`
	Timeouts                        timeouts.Value `tfsdk:"timeouts"`
}

func (r *userResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

// fixedString is a computed attribute netcup does not change through this API.
func fixedString(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:            true,
		MarkdownDescription: description,
		PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

func writeOnlyPassword(description, required, version string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional: true, Sensitive: true, WriteOnly: true,
		MarkdownDescription: description + " It is sent on create, on the first apply after an import and whenever `" + version + "` changes. It stays out of plan and state and needs Terraform 1.11 or newer.",
		Validators:          []validator.String{stringvalidator.UTF8LengthAtMost(255), stringvalidator.AlsoRequires(path.MatchRoot(required))},
	}
}

func passwordVersion(password string) schema.Int64Attribute {
	return schema.Int64Attribute{
		Optional:            true,
		MarkdownDescription: "Change it to send `" + password + "` again.",
		Validators:          []validator.Int64{int64validator.AlsoRequires(path.MatchRoot(password))},
	}
}

func (r *userResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The settings of the netcup SCP user of the refresh token (SCP REST API). netcup cannot create or delete users through the API, so `destroy` only drops the resource from the state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Id of the user.",
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"username":  fixedString("User name."),
			"firstname": fixedString("First name."),
			"lastname":  fixedString("Last name."),
			"email":     fixedString("E-mail address."),
			"company":   fixedString("Company."),
			"language": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Language, `en` or `de`.",
				Validators:          []validator.String{stringvalidator.OneOf("en", "de")},
			},
			"time_zone": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Time zone, e.g. `Europe/Berlin`.",
				Validators:          []validator.String{stringvalidator.RegexMatches(nonBlankPattern, "must not be blank")},
			},
			"show_nickname": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Whether the SCP shows nicknames in preference to server names.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"passwordless_mode": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "The `passwordlessMode` setting of the user. netcup does not document its effect.",
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"api_ip_login_restrictions": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "The `apiIpLoginRestrictions` setting of the user. netcup does not document its format, and a wrong value can lock Terraform out of the API. An empty string should remove it, which is an assumption.",
				Validators:          []validator.String{stringvalidator.UTF8LengthAtMost(255)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"password_wo":                         writeOnlyPassword("New password of the user.", "password_wo_version", "password_wo_version"),
			"old_password_wo":                     writeOnlyPassword("Current password of the user.", "password_wo", "password_wo_version"),
			"password_wo_version":                 passwordVersion("password_wo"),
			"soap_webservice_password_wo":         writeOnlyPassword("New password of the SOAP web service.", "soap_webservice_password_wo_version", "soap_webservice_password_wo_version"),
			"soap_webservice_password_wo_version": passwordVersion("soap_webservice_password_wo"),
			"timeouts":                            timeouts.Attributes(ctx, timeouts.Opts{Create: true, Update: true}),
		},
	}
}

func (r *userResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

// store sets everything netcup returns. An empty string in the model stays while
// netcup reads back no restrictions.
func (m *userModel) store(user *scp.User) {
	m.ID = types.Int64Value(user.ID)
	m.Username = optString(user.Username)
	m.Firstname = optString(user.Firstname)
	m.Lastname = optString(user.Lastname)
	m.Email = optString(user.Email)
	m.Company = types.StringPointerValue(user.Company)
	m.Language = optString(user.Language)
	m.TimeZone = types.StringPointerValue(user.TimeZone)
	m.ShowNickname = types.BoolPointerValue(user.ShowNickname)
	m.PasswordlessMode = types.BoolPointerValue(user.PasswordlessMode)
	if user.APIIPLoginRestrictions != nil || m.APIIPLoginRestrictions.ValueString() != "" {
		m.APIIPLoginRestrictions = types.StringPointerValue(user.APIIPLoginRestrictions)
	}
}

// put sends the planned settings with the passwords.
func (r *userResource) put(ctx context.Context, plan *userModel, password, oldPassword, soapPassword *string) error {
	return r.client.UpdateUser(ctx, scp.UserSave{
		ID:                     plan.ID.ValueInt64(),
		Language:               plan.Language.ValueString(),
		TimeZone:               plan.TimeZone.ValueString(),
		APIIPLoginRestrictions: plan.APIIPLoginRestrictions.ValueStringPointer(),
		Password:               password,
		OldPassword:            oldPassword,
		SoapWebservicePassword: soapPassword,
		ShowNickname:           plan.ShowNickname.ValueBoolPointer(),
		PasswordlessMode:       plan.PasswordlessMode.ValueBoolPointer(),
	})
}

func (r *userResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	limit, diags := plan.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	current, err := r.client.GetUser(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}
	plan.ID = types.Int64Value(current.ID)
	if plan.ShowNickname.IsUnknown() {
		plan.ShowNickname = types.BoolPointerValue(current.ShowNickname)
	}
	if plan.PasswordlessMode.IsUnknown() {
		plan.PasswordlessMode = types.BoolPointerValue(current.PasswordlessMode)
	}
	if plan.APIIPLoginRestrictions.IsUnknown() {
		plan.APIIPLoginRestrictions = types.StringPointerValue(current.APIIPLoginRestrictions)
	}
	if err := r.put(ctx, &plan, cfg.PasswordWO.ValueStringPointer(), cfg.OldPasswordWO.ValueStringPointer(), cfg.SoapWebservicePasswordWO.ValueStringPointer()); err != nil {
		resp.Diagnostics.AddError("Error updating user", err.Error())
		return
	}
	user, err := r.client.GetUser(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}
	plan.store(user)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *userResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	user, err := r.client.GetUser(ctx)
	if err != nil {
		if scp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}
	// Another refresh token means another user, not a change of this one.
	if state.ID.ValueInt64() != user.ID {
		resp.State.RemoveResource(ctx)
		return
	}
	state.store(user)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *userResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg userModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	limit, diags := plan.Timeouts.Update(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	var password, oldPassword, soapPassword *string
	if !plan.PasswordWOVersion.Equal(state.PasswordWOVersion) {
		password, oldPassword = cfg.PasswordWO.ValueStringPointer(), cfg.OldPasswordWO.ValueStringPointer()
	}
	if !plan.SoapWebservicePasswordWOVersion.Equal(state.SoapWebservicePasswordWOVersion) {
		soapPassword = cfg.SoapWebservicePasswordWO.ValueStringPointer()
	}
	if err := r.put(ctx, &plan, password, oldPassword, soapPassword); err != nil {
		resp.Diagnostics.AddError("Error updating user", err.Error())
		return
	}
	// netcup has the passwords now, even if the read below fails.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("password_wo_version"), plan.PasswordWOVersion)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("soap_webservice_password_wo_version"), plan.SoapWebservicePasswordWOVersion)...)
	user, err := r.client.GetUser(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}
	plan.store(user)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete only drops the resource from the state, netcup has no such operation.
func (r *userResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning("User left untouched",
		"netcup_user only manages settings of the existing user, so destroy only drops it from the state.")
}

func (r *userResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", "Expected the id of the user, got "+req.ID+".")
		return
	}
	own, err := r.client.UserID(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}
	if id != int64(own) {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("%d is not the user of the refresh token, that is %d.", id, own))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
