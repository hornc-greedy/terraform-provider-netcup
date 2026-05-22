// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

var (
	_ datasource.DataSource              = (*firewallPoliciesDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*firewallPoliciesDataSource)(nil)
)

type firewallPoliciesDataSource struct {
	client *scp.Client
}

// NewFirewallPoliciesDataSource is the data source factory.
func NewFirewallPoliciesDataSource() datasource.DataSource {
	return &firewallPoliciesDataSource{}
}

type firewallPoliciesModel struct {
	Policies []firewallPolicyEntryModel `tfsdk:"policies"`
}

type firewallPolicyEntryModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	RuleCount   types.Int64  `tfsdk:"rule_count"`
}

func (d *firewallPoliciesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_policies"
}

func (d *firewallPoliciesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "All firewall policies of the netcup SCP account.",
		Attributes: map[string]schema.Attribute{
			"policies": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Firewall policies.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{Computed: true, MarkdownDescription: "Policy id."},
						"name":        schema.StringAttribute{Computed: true, MarkdownDescription: "Policy name."},
						"description": schema.StringAttribute{Computed: true, MarkdownDescription: "Policy description."},
						"rule_count":  schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of rules in the policy."},
					},
				},
			},
		},
	}
}

func (d *firewallPoliciesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	d.client = requireSCP(data, &resp.Diagnostics)
}

func (d *firewallPoliciesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	policies, err := d.client.ListFirewallPolicies(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing firewall policies", err.Error())
		return
	}
	var state firewallPoliciesModel
	state.Policies = make([]firewallPolicyEntryModel, 0, len(policies))
	for _, p := range policies {
		state.Policies = append(state.Policies, firewallPolicyEntryModel{
			ID:          types.StringValue(strconv.Itoa(p.ID)),
			Name:        types.StringValue(p.Name),
			Description: types.StringValue(p.Description),
			RuleCount:   types.Int64Value(int64(len(p.Rules))),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
