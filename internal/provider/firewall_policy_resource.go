// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

var (
	_ resource.Resource                = (*firewallPolicyResource)(nil)
	_ resource.ResourceWithConfigure   = (*firewallPolicyResource)(nil)
	_ resource.ResourceWithImportState = (*firewallPolicyResource)(nil)
)

type firewallPolicyResource struct {
	client *scp.Client
}

// NewFirewallPolicyResource is the resource factory.
func NewFirewallPolicyResource() resource.Resource {
	return &firewallPolicyResource{}
}

type firewallPolicyModel struct {
	ID          types.String        `tfsdk:"id"`
	Name        types.String        `tfsdk:"name"`
	Description types.String        `tfsdk:"description"`
	Rules       []firewallRuleModel `tfsdk:"rule"`
}

type firewallRuleModel struct {
	Action           types.String `tfsdk:"action"`
	Protocol         types.String `tfsdk:"protocol"`
	Direction        types.String `tfsdk:"direction"`
	Sources          types.List   `tfsdk:"sources"`
	Destinations     types.List   `tfsdk:"destinations"`
	SourcePorts      types.String `tfsdk:"source_ports"`
	DestinationPorts types.String `tfsdk:"destination_ports"`
	Description      types.String `tfsdk:"description"`
}

func (r *firewallPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_policy"
}

func (r *firewallPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A firewall policy in the netcup SCP account, assignable to server interfaces.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SCP policy id.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Policy name.",
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "Policy description.",
			},
		},
		Blocks: map[string]schema.Block{
			"rule": schema.ListNestedBlock{
				MarkdownDescription: "Firewall rules, evaluated in order.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"action": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "`ACCEPT` or `DROP`.",
							Validators: []validator.String{
								stringvalidator.OneOf("ACCEPT", "DROP"),
							},
						},
						"protocol": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "`TCP`, `UDP`, `ICMP` or `ICMPv6`.",
							Validators: []validator.String{
								stringvalidator.OneOf("TCP", "UDP", "ICMP", "ICMPv6"),
							},
						},
						"direction": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "`INGRESS` or `EGRESS`.",
							Validators: []validator.String{
								stringvalidator.OneOf("INGRESS", "EGRESS"),
							},
						},
						"sources": schema.ListAttribute{
							Optional:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Source CIDRs/addresses.",
						},
						"destinations": schema.ListAttribute{
							Optional:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Destination CIDRs/addresses.",
						},
						"source_ports": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Source port or range.",
						},
						"destination_ports": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Destination port or range.",
						},
						"description": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Free-text rule description.",
						},
					},
				},
			},
		},
	}
}

func (r *firewallPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

// toAPI converts the plan model into the client policy.
func (m firewallPolicyModel) toAPI(ctx context.Context, da *diagAppender) scp.FirewallPolicy {
	p := scp.FirewallPolicy{
		Name:        m.Name.ValueString(),
		Description: m.Description.ValueString(),
	}
	for _, rule := range m.Rules {
		p.Rules = append(p.Rules, scp.FirewallRule{
			Action:           rule.Action.ValueString(),
			Protocol:         rule.Protocol.ValueString(),
			Direction:        rule.Direction.ValueString(),
			Sources:          da.stringList(ctx, rule.Sources),
			Destinations:     da.stringList(ctx, rule.Destinations),
			SourcePorts:      rule.SourcePorts.ValueString(),
			DestinationPorts: rule.DestinationPorts.ValueString(),
			Description:      rule.Description.ValueString(),
		})
	}
	return p
}

func (r *firewallPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	da := &diagAppender{diags: &resp.Diagnostics}
	api := plan.toAPI(ctx, da)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.client.CreateFirewallPolicy(ctx, api)
	if err != nil {
		resp.Diagnostics.AddError("Error creating firewall policy", err.Error())
		return
	}
	plan.ID = types.StringValue(strconv.Itoa(created.ID))
	plan.Description = types.StringValue(created.Description)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *firewallPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid policy id", err.Error())
		return
	}
	pol, err := r.client.GetFirewallPolicy(ctx, id)
	if err != nil {
		if scp.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading firewall policy", err.Error())
		return
	}
	state.Name = types.StringValue(pol.Name)
	state.Description = types.StringValue(pol.Description)
	state.Rules = rulesToModel(ctx, pol.Rules, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *firewallPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan firewallPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid policy id", err.Error())
		return
	}
	da := &diagAppender{diags: &resp.Diagnostics}
	api := plan.toAPI(ctx, da)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdateFirewallPolicy(ctx, id, api)
	if err != nil {
		resp.Diagnostics.AddError("Error updating firewall policy", err.Error())
		return
	}
	plan.Description = types.StringValue(updated.Description)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *firewallPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firewallPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := strconv.Atoi(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid policy id", err.Error())
		return
	}
	if err := r.client.DeleteFirewallPolicy(ctx, id); err != nil {
		resp.Diagnostics.AddError("Error deleting firewall policy", err.Error())
	}
}

func (r *firewallPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
