// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/clouddns"
)

var (
	_ resource.Resource                = (*cloudDNSAcmeChallengeResource)(nil)
	_ resource.ResourceWithConfigure   = (*cloudDNSAcmeChallengeResource)(nil)
	_ resource.ResourceWithImportState = (*cloudDNSAcmeChallengeResource)(nil)
)

type cloudDNSAcmeChallengeResource struct {
	client *clouddns.Client
}

// NewCloudDNSAcmeChallengeResource is the resource factory.
func NewCloudDNSAcmeChallengeResource() resource.Resource {
	return &cloudDNSAcmeChallengeResource{}
}

type cloudDNSAcmeChallengeModel struct {
	ID       types.String `tfsdk:"id"`
	DomainID types.Int64  `tfsdk:"domain_id"`
	Scope    types.String `tfsdk:"scope"`
	Value    types.String `tfsdk:"value"`
	FQDN     types.String `tfsdk:"fqdn"`
	Status   types.String `tfsdk:"status"`
}

func (r *cloudDNSAcmeChallengeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clouddns_acme_challenge"
}

func (r *cloudDNSAcmeChallengeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		MarkdownDescription: "An ACME challenge of a domain on CloudDNS (netcup API).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite id `domain_id/scope/value`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"domain_id": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Id of the domain, from the `netcup_clouddns_domain` data source.",
				PlanModifiers:       []planmodifier.Int64{int64RequiresReplace()},
			},
			"scope": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`@` for the zone apex or a hostname label.",
				PlanModifiers:       replace,
			},
			"value": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Challenge value, 43 characters of base64url.",
				PlanModifiers:       replace,
				Validators: []validator.String{stringvalidator.RegexMatches(
					regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`), "must be 43 characters of base64url",
				)},
			},
			"fqdn": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Name of the challenge record.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Status of the challenge.",
			},
		},
	}
}

func (r *cloudDNSAcmeChallengeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireCloudDNS(data, &resp.Diagnostics)
}

func (m *cloudDNSAcmeChallengeModel) apply(c *clouddns.Challenge) {
	m.ID = types.StringValue(fmt.Sprintf("%d/%s/%s", m.DomainID.ValueInt64(), m.Scope.ValueString(), m.Value.ValueString()))
	m.FQDN = types.StringValue(c.FQDN)
	m.Status = types.StringValue(c.Status)
}

func (r *cloudDNSAcmeChallengeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan cloudDNSAcmeChallengeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	challenge, err := r.client.CreateChallenge(ctx, plan.DomainID.ValueInt64(), plan.Scope.ValueString(), plan.Value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating ACME challenge", err.Error())
		return
	}
	plan.apply(challenge)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *cloudDNSAcmeChallengeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state cloudDNSAcmeChallengeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	challenge, err := r.client.Challenge(ctx, state.DomainID.ValueInt64(), state.Scope.ValueString(), state.Value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading ACME challenge", err.Error())
		return
	}
	if challenge == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.apply(challenge)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is unreachable (all writable attributes force replacement) but required.
func (r *cloudDNSAcmeChallengeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan cloudDNSAcmeChallengeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *cloudDNSAcmeChallengeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state cloudDNSAcmeChallengeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.DeleteChallenge(ctx, state.DomainID.ValueInt64(), state.Scope.ValueString(), state.Value.ValueString())
	if err != nil && !clouddns.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting ACME challenge", err.Error())
	}
}

func (r *cloudDNSAcmeChallengeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 3)
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Expected format <domain_id>/<scope>/<value>")
		return
	}
	domainID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid domain_id in import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain_id"), domainID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("scope"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("value"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
