// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

type vlanSiteModel struct {
	ID   types.Int64  `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	City types.String `tfsdk:"city"`
}

type vlanBandwidthClassModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	SpeedInMBit types.Int64  `tfsdk:"speed_in_mbit"`
}

type vlanModel struct {
	VlanID         types.Int64              `tfsdk:"vlan_id"`
	Name           types.String             `tfsdk:"name"`
	User           *userMinimalModel        `tfsdk:"user"`
	Site           *vlanSiteModel           `tfsdk:"site"`
	BandwidthClass *vlanBandwidthClassModel `tfsdk:"bandwidth_class"`
}

func vlanToModel(v *scp.VLan) vlanModel {
	m := vlanModel{VlanID: types.Int64PointerValue(v.VlanID), Name: optString(v.Name), User: userMinimal(v.User)}
	if v.Site != nil {
		m.Site = &vlanSiteModel{ID: types.Int64PointerValue(v.Site.ID), Name: optString(v.Site.Name), City: optString(v.Site.City)}
	}
	if v.BandwidthClass != nil {
		m.BandwidthClass = &vlanBandwidthClassModel{
			ID: types.Int64PointerValue(v.BandwidthClass.ID), Name: optString(v.BandwidthClass.Name), SpeedInMBit: types.Int64PointerValue(v.BandwidthClass.SpeedInMBit),
		}
	}
	return m
}

func vlanAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"vlan_id": schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the VLAN."},
		"name":    schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the VLAN."},
		"user":    userMinimalAttribute("User the VLAN belongs to."),
		"site": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: "Site of the VLAN.",
			Attributes: map[string]schema.Attribute{
				"id":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the site."},
				"name": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the site."},
				"city": schema.StringAttribute{Computed: true, MarkdownDescription: "City of the site."},
			},
		},
		"bandwidth_class": schema.SingleNestedAttribute{
			Computed:            true,
			MarkdownDescription: "Bandwidth class of the VLAN.",
			Attributes: map[string]schema.Attribute{
				"id":            schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the class."},
				"name":          schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the class."},
				"speed_in_mbit": schema.Int64Attribute{Computed: true, MarkdownDescription: "Speed in MBit/s."},
			},
		},
	}
}

func vlanLookupAttributes() map[string]schema.Attribute {
	attributes := vlanAttributes()
	attributes["vlan_id"] = schema.Int64Attribute{Required: true, MarkdownDescription: "Id of the VLAN."}
	return attributes
}

// vlanDataSource reads one VLAN by its id.
type vlanDataSource struct{ scpDataSource }

// NewVlanDataSource is the data source factory.
func NewVlanDataSource() datasource.DataSource { return &vlanDataSource{} }

func (d *vlanDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vlan"
}

func (d *vlanDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One netcup VLAN by its id alone (SCP REST API). Use `netcup_user_vlan` for a VLAN of the user.",
		Attributes:          vlanLookupAttributes(),
	}
}

func (d *vlanDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg vlanModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vlan, err := d.client.GetVlan(ctx, cfg.VlanID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading VLAN", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, vlanToModel(vlan))...)
}

// userVlanDataSource reads one VLAN of the user.
type userVlanDataSource struct{ scpDataSource }

// NewUserVlanDataSource is the data source factory.
func NewUserVlanDataSource() datasource.DataSource { return &userVlanDataSource{} }

func (d *userVlanDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_vlan"
}

func (d *userVlanDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One VLAN of the netcup SCP user. netcup answers 404 for a VLAN of another user.",
		Attributes:          vlanLookupAttributes(),
	}
}

func (d *userVlanDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg vlanModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vlan, err := d.client.GetUserVlan(ctx, cfg.VlanID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading VLAN", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, vlanToModel(vlan))...)
}

// userVlansDataSource lists the VLANs of the user.
type userVlansDataSource struct{ scpDataSource }

// NewUserVlansDataSource is the data source factory.
func NewUserVlansDataSource() datasource.DataSource { return &userVlansDataSource{} }

func (d *userVlansDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user_vlans"
}

func (d *userVlansDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The VLANs of the netcup SCP user.",
		Attributes: map[string]schema.Attribute{
			"server_id": schema.Int64Attribute{Optional: true, MarkdownDescription: "Only VLANs of this server."},
			"vlans": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "VLANs of the user.",
				NestedObject:        schema.NestedAttributeObject{Attributes: vlanAttributes()},
			},
		},
	}
}

func (d *userVlansDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		ServerID types.Int64 `tfsdk:"server_id"`
		Vlans    []vlanModel `tfsdk:"vlans"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vlans, err := d.client.ListUserVlans(ctx, cfg.ServerID.ValueInt64Pointer())
	if err != nil {
		resp.Diagnostics.AddError("Error listing VLANs", err.Error())
		return
	}
	cfg.Vlans = make([]vlanModel, 0, len(vlans))
	for i := range vlans {
		cfg.Vlans = append(cfg.Vlans, vlanToModel(&vlans[i]))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
