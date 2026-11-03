// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// userDataSource reads the user of the refresh token.
type userDataSource struct{ scpDataSource }

// NewUserDataSource is the data source factory.
func NewUserDataSource() datasource.DataSource { return &userDataSource{} }

func (d *userDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (d *userDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	computed := func(description string) schema.StringAttribute {
		return schema.StringAttribute{Computed: true, MarkdownDescription: description}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "The netcup SCP user of the refresh token (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"id":                        schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the user."},
			"username":                  computed("User name."),
			"firstname":                 computed("First name."),
			"lastname":                  computed("Last name."),
			"email":                     computed("E-mail address."),
			"company":                   computed("Company."),
			"language":                  computed("Language."),
			"time_zone":                 computed("Time zone."),
			"show_nickname":             schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the SCP shows nicknames in preference to server names."},
			"passwordless_mode":         schema.BoolAttribute{Computed: true, MarkdownDescription: "The `passwordlessMode` setting of the user. netcup does not document its effect."},
			"api_ip_login_restrictions": computed("The `apiIpLoginRestrictions` setting of the user."),
		},
	}
}

func (d *userDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	user, err := d.client.GetUser(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading user", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, struct {
		ID                     types.Int64  `tfsdk:"id"`
		Username               types.String `tfsdk:"username"`
		Firstname              types.String `tfsdk:"firstname"`
		Lastname               types.String `tfsdk:"lastname"`
		Email                  types.String `tfsdk:"email"`
		Company                types.String `tfsdk:"company"`
		Language               types.String `tfsdk:"language"`
		TimeZone               types.String `tfsdk:"time_zone"`
		ShowNickname           types.Bool   `tfsdk:"show_nickname"`
		PasswordlessMode       types.Bool   `tfsdk:"passwordless_mode"`
		APIIPLoginRestrictions types.String `tfsdk:"api_ip_login_restrictions"`
	}{
		types.Int64Value(user.ID), optString(user.Username), optString(user.Firstname),
		optString(user.Lastname), optString(user.Email), types.StringPointerValue(user.Company),
		optString(user.Language), types.StringPointerValue(user.TimeZone), types.BoolPointerValue(user.ShowNickname),
		types.BoolPointerValue(user.PasswordlessMode), types.StringPointerValue(user.APIIPLoginRestrictions),
	})...)
}

// userLogsDataSource lists the logs of the user of the refresh token.
type userLogsDataSource struct{ scpDataSource }

// NewUserLogsDataSource is the data source factory.
func NewUserLogsDataSource() datasource.DataSource { return &userLogsDataSource{} }

func (d *userLogsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_logs"
}

func (d *userLogsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The logs of the netcup SCP user of the refresh token (SCP REST API).",
		Attributes: pageAttributes(map[string]schema.Attribute{
			"logs": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Log entries of the user.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"type":           schema.StringAttribute{Computed: true, MarkdownDescription: "`ERROR`, `WARNING` or `INFO`."},
					"executing_user": userMinimalAttribute("User who executed it."),
					"date":           schema.StringAttribute{Computed: true, MarkdownDescription: "Time of the entry."},
					"log_key":        schema.StringAttribute{Computed: true, MarkdownDescription: "Key of the entry."},
					"message":        schema.StringAttribute{Computed: true, MarkdownDescription: "Message of the entry."},
				}},
			},
		}),
	}
}

func (d *userLogsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	type entry struct {
		Type          types.String      `tfsdk:"type"`
		ExecutingUser *userMinimalModel `tfsdk:"executing_user"`
		Date          types.String      `tfsdk:"date"`
		LogKey        types.String      `tfsdk:"log_key"`
		Message       types.String      `tfsdk:"message"`
	}
	var cfg struct {
		Limit  types.Int64 `tfsdk:"limit"`
		Offset types.Int64 `tfsdk:"offset"`
		Logs   []entry     `tfsdk:"logs"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	logs, err := d.client.ListUserLogs(ctx, pageOf(cfg.Limit, cfg.Offset))
	if err != nil {
		resp.Diagnostics.AddError("Error listing user logs", err.Error())
		return
	}
	cfg.Logs = make([]entry, 0, len(logs))
	for _, l := range logs {
		cfg.Logs = append(cfg.Logs, entry{
			Type: optString(l.Type), ExecutingUser: userMinimal(l.ExecutingUser), Date: optString(l.Date), LogKey: optString(l.LogKey), Message: optString(l.Message),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
