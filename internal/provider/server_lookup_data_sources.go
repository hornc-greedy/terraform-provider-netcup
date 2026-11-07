// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	ephemeralschema "github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

func computedAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{Computed: true, MarkdownDescription: description}
}

// serverFirewallDataSource reads the firewall of an interface.
type serverFirewallDataSource struct{ scpDataSource }

// NewServerFirewallDataSource is the data source factory.
func NewServerFirewallDataSource() datasource.DataSource { return &serverFirewallDataSource{} }

func (d *serverFirewallDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_firewall"
}

func (d *serverFirewallDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	policies := func(description string) schema.ListNestedAttribute {
		return schema.ListNestedAttribute{
			Computed:            true,
			MarkdownDescription: description,
			NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"id":          schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the policy."},
				"name":        computedAttribute("Name of the policy."),
				"description": computedAttribute("Description of the policy."),
				"rules": schema.ListNestedAttribute{
					Computed:            true,
					MarkdownDescription: "Rules of the policy.",
					NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
						"number_of_effective_rules": schema.Int64Attribute{Computed: true, MarkdownDescription: "The `numberOfEffectiveRules` value of netcup."},
						"description":               computedAttribute("Description of the rule."),
						"direction":                 computedAttribute("`INGRESS` or `EGRESS`."),
						"protocol":                  computedAttribute("`TCP`, `UDP`, `ICMP` or `ICMPv6`."),
						"action":                    computedAttribute("`ACCEPT` or `DROP`."),
						"sources":                   schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Sources of the rule."},
						"source_ports":              computedAttribute("Source ports of the rule."),
						"destinations":              schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Destinations of the rule."},
						"destination_ports":         computedAttribute("Destination ports of the rule."),
					}},
				},
			}},
		}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "The firewall of an interface of a netcup server (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"server_id": serverIDAttribute(),
			"mac":       schema.StringAttribute{Required: true, MarkdownDescription: "MAC address of the interface."},
			"consistency_check": schema.BoolAttribute{
				Optional:            true,
				MarkdownDescription: "Whether netcup checks that the rules have been applied, which sets `consistent`.",
			},
			"copied_policies":       policies("Copied policies."),
			"user_policies":         policies("Policies of the user."),
			"ingress_implicit_rule": computedAttribute("`ACCEPT_ALL` or `DROP_ALL`."),
			"egress_implicit_rule":  computedAttribute("`ACCEPT_ALL` or `DROP_ALL`."),
			"consistent":            schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the rules have been applied. Null without `consistency_check`. If false, `netcup_server_firewall_reapply` applies them again."},
			"active":                schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the firewall is active."},
		},
	}
}

// firewallRuleData is a rule of the firewall data source.
type firewallRuleData struct {
	NumberOfEffectiveRules types.Int64  `tfsdk:"number_of_effective_rules"`
	Description            types.String `tfsdk:"description"`
	Direction              types.String `tfsdk:"direction"`
	Protocol               types.String `tfsdk:"protocol"`
	Action                 types.String `tfsdk:"action"`
	Sources                []string     `tfsdk:"sources"`
	SourcePorts            types.String `tfsdk:"source_ports"`
	Destinations           []string     `tfsdk:"destinations"`
	DestinationPorts       types.String `tfsdk:"destination_ports"`
}

// firewallPolicyData is a policy of the firewall data source.
type firewallPolicyData struct {
	ID          types.Int64        `tfsdk:"id"`
	Name        types.String       `tfsdk:"name"`
	Description types.String       `tfsdk:"description"`
	Rules       []firewallRuleData `tfsdk:"rules"`
}

func firewallPoliciesData(policies []scp.FirewallPolicy) []firewallPolicyData {
	out := make([]firewallPolicyData, 0, len(policies))
	for _, p := range policies {
		rules := make([]firewallRuleData, 0, len(p.Rules))
		for _, r := range p.Rules {
			rules = append(rules, firewallRuleData{
				NumberOfEffectiveRules: types.Int64PointerValue(r.NumberOfEffectiveRules),
				Description:            optString(r.Description), Direction: optString(r.Direction),
				Protocol: optString(r.Protocol), Action: optString(r.Action),
				Sources: append([]string{}, r.Sources...), SourcePorts: optString(r.SourcePorts),
				Destinations: append([]string{}, r.Destinations...), DestinationPorts: optString(r.DestinationPorts),
			})
		}
		out = append(out, firewallPolicyData{
			ID: types.Int64Value(int64(p.ID)), Name: optString(p.Name), Description: optString(p.Description), Rules: rules,
		})
	}
	return out
}

func (d *serverFirewallDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		ServerID         types.Int64          `tfsdk:"server_id"`
		Mac              types.String         `tfsdk:"mac"`
		ConsistencyCheck types.Bool           `tfsdk:"consistency_check"`
		CopiedPolicies   []firewallPolicyData `tfsdk:"copied_policies"`
		UserPolicies     []firewallPolicyData `tfsdk:"user_policies"`
		IngressRule      types.String         `tfsdk:"ingress_implicit_rule"`
		EgressRule       types.String         `tfsdk:"egress_implicit_rule"`
		Consistent       types.Bool           `tfsdk:"consistent"`
		Active           types.Bool           `tfsdk:"active"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	read := d.client.GetServerFirewall
	if cfg.ConsistencyCheck.ValueBool() {
		read = d.client.CheckServerFirewall
	}
	firewall, err := read(ctx, int(cfg.ServerID.ValueInt64()), cfg.Mac.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading server firewall", err.Error())
		return
	}
	cfg.CopiedPolicies, cfg.UserPolicies = firewallPoliciesData(firewall.CopiedPolicies), firewallPoliciesData(firewall.UserPolicies)
	cfg.IngressRule, cfg.EgressRule = optString(firewall.IngressImplicitRule), optString(firewall.EgressImplicitRule)
	cfg.Consistent, cfg.Active = types.BoolPointerValue(firewall.Consistent), types.BoolValue(firewall.Active)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// serverSnapshotDataSource reads one snapshot.
type serverSnapshotDataSource struct{ scpDataSource }

// NewServerSnapshotDataSource is the data source factory.
func NewServerSnapshotDataSource() datasource.DataSource { return &serverSnapshotDataSource{} }

func (d *serverSnapshotDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_snapshot"
}

func (d *serverSnapshotDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One snapshot of a netcup server (SCP REST API). `netcup_server_snapshot_download` has the download of an exported one.",
		Attributes: map[string]schema.Attribute{
			"server_id":            serverIDAttribute(),
			"name":                 schema.StringAttribute{Required: true, MarkdownDescription: "Name of the snapshot."},
			"uuid":                 computedAttribute("UUID of the snapshot."),
			"description":          computedAttribute("Description."),
			"disks":                schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Disks of the snapshot."},
			"creation_time":        computedAttribute("Time of the snapshot."),
			"state":                computedAttribute("Power state in the snapshot."),
			"online":               schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether it is an online snapshot."},
			"exported":             schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether it was exported."},
			"exported_size_in_kib": schema.Int64Attribute{Computed: true, MarkdownDescription: "Size of the export in KiB."},
		},
	}
}

func (d *serverSnapshotDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		ServerID     types.Int64  `tfsdk:"server_id"`
		Name         types.String `tfsdk:"name"`
		UUID         types.String `tfsdk:"uuid"`
		Description  types.String `tfsdk:"description"`
		Disks        []string     `tfsdk:"disks"`
		CreationTime types.String `tfsdk:"creation_time"`
		State        types.String `tfsdk:"state"`
		Online       types.Bool   `tfsdk:"online"`
		Exported     types.Bool   `tfsdk:"exported"`
		ExportedSize types.Int64  `tfsdk:"exported_size_in_kib"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	snapshot, err := d.client.GetSnapshot(ctx, int(cfg.ServerID.ValueInt64()), cfg.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading snapshot", err.Error())
		return
	}
	if snapshot == nil {
		resp.Diagnostics.AddError("Snapshot not found", "The server has no snapshot "+cfg.Name.ValueString()+".")
		return
	}
	cfg.UUID, cfg.Description = optString(snapshot.UUID), optString(snapshot.Description)
	cfg.Disks = append([]string{}, snapshot.Disks...)
	cfg.CreationTime, cfg.State = optString(snapshot.CreationTime), optString(snapshot.State)
	cfg.Online, cfg.Exported = types.BoolValue(snapshot.Online), types.BoolPointerValue(snapshot.Exported)
	cfg.ExportedSize = types.Int64PointerValue(snapshot.ExportedSizeInKiB)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// serverSnapshotDownloadEphemeralResource reads the download of an export. Its
// URL grants access while valid, so it stays out of plan and state.
type serverSnapshotDownloadEphemeralResource struct {
	client *scp.Client
}

var _ ephemeral.EphemeralResourceWithConfigure = (*serverSnapshotDownloadEphemeralResource)(nil)

// NewServerSnapshotDownloadEphemeralResource is the ephemeral resource factory.
func NewServerSnapshotDownloadEphemeralResource() ephemeral.EphemeralResource {
	return &serverSnapshotDownloadEphemeralResource{}
}

func (r *serverSnapshotDownloadEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_snapshot_download"
}

func (r *serverSnapshotDownloadEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = ephemeralschema.Schema{
		MarkdownDescription: "The download of a snapshot that `netcup_server_snapshot_export` exported (SCP REST API). It needs Terraform 1.10 or newer.",
		Attributes: map[string]ephemeralschema.Attribute{
			"server_id":     ephemeralschema.Int64Attribute{Required: true, MarkdownDescription: "Id of the server."},
			"name":          ephemeralschema.StringAttribute{Required: true, MarkdownDescription: "Name of the snapshot."},
			"filename":      ephemeralschema.StringAttribute{Computed: true, MarkdownDescription: "Name of the file."},
			"presigned_url": ephemeralschema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Download URL."},
			"presigned_url_validity_duration_in_hours": ephemeralschema.Int64Attribute{Computed: true, MarkdownDescription: "Validity of the URL in hours."},
			"headers": ephemeralschema.MapAttribute{
				Computed:            true,
				Sensitive:           true,
				ElementType:         types.ListType{ElemType: types.StringType},
				MarkdownDescription: "Headers to send with the download.",
			},
		},
	}
}

func (r *serverSnapshotDownloadEphemeralResource) Configure(_ context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

func (r *serverSnapshotDownloadEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var cfg struct {
		ServerID types.Int64         `tfsdk:"server_id"`
		Name     types.String        `tfsdk:"name"`
		Filename types.String        `tfsdk:"filename"`
		URL      types.String        `tfsdk:"presigned_url"`
		Validity types.Int64         `tfsdk:"presigned_url_validity_duration_in_hours"`
		Headers  map[string][]string `tfsdk:"headers"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	snapshot, err := r.client.GetSnapshot(ctx, int(cfg.ServerID.ValueInt64()), cfg.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading snapshot", err.Error())
		return
	}
	if snapshot == nil || snapshot.DownloadInfos == nil {
		resp.Diagnostics.AddError("Snapshot has no download", "The server has no exported snapshot "+cfg.Name.ValueString()+".")
		return
	}
	cfg.Filename = optString(snapshot.DownloadInfos.Filename)
	cfg.URL = types.StringValue(snapshot.DownloadInfos.PresignedURL)
	cfg.Validity = types.Int64PointerValue(snapshot.DownloadInfos.PresignedURLValidityDurationInHours)
	cfg.Headers = snapshot.DownloadInfos.Headers
	resp.Diagnostics.Append(resp.Result.Set(ctx, cfg)...)
}

// serverIsoDataSource reads the ISO of a server without managing it.
type serverIsoDataSource struct{ scpDataSource }

// NewServerIsoDataSource is the data source factory.
func NewServerIsoDataSource() datasource.DataSource { return &serverIsoDataSource{} }

func (d *serverIsoDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_iso"
}

func (d *serverIsoDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The ISO attached to a netcup server (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"server_id":    serverIDAttribute(),
			"iso_attached": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether an ISO is attached."},
			"iso":          computedAttribute("Name of the ISO, null without one."),
		},
	}
}

func (d *serverIsoDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		ServerID    types.Int64  `tfsdk:"server_id"`
		IsoAttached types.Bool   `tfsdk:"iso_attached"`
		Iso         types.String `tfsdk:"iso"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	iso, err := d.client.GetIso(ctx, int(cfg.ServerID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading ISO", err.Error())
		return
	}
	cfg.IsoAttached, cfg.Iso = types.BoolValue(iso.IsoAttached), optString(iso.Iso)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// reverseDNSDataSource reads a PTR record without managing it.
type reverseDNSDataSource struct{ scpDataSource }

// NewReverseDNSDataSource is the data source factory.
func NewReverseDNSDataSource() datasource.DataSource { return &reverseDNSDataSource{} }

func (d *reverseDNSDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_reverse_dns"
}

func (d *reverseDNSDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The reverse DNS entry of an IP address (SCP REST API). `rdns` is null without an entry.",
		Attributes: map[string]schema.Attribute{
			"ip":   schema.StringAttribute{Required: true, MarkdownDescription: "IPv4 or IPv6 address."},
			"rdns": computedAttribute("Hostname of the entry."),
		},
	}
}

func (d *reverseDNSDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		IP   types.String `tfsdk:"ip"`
		RDNS types.String `tfsdk:"rdns"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rdns, err := d.client.GetReverseDNS(ctx, cfg.IP.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading reverse DNS", err.Error())
		return
	}
	cfg.RDNS = optString(rdns)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// taskDataSource reads one task.
type taskDataSource struct{ scpDataSource }

// NewTaskDataSource is the data source factory.
func NewTaskDataSource() datasource.DataSource { return &taskDataSource{} }

func (d *taskDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_task"
}

func (d *taskDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One task of the netcup SCP (SCP REST API). `netcup_tasks` lists them.",
		Attributes: map[string]schema.Attribute{
			"uuid":           schema.StringAttribute{Required: true, MarkdownDescription: "UUID of the task."},
			"name":           computedAttribute("Name of the task."),
			"state":          computedAttribute("`PENDING`, `RUNNING`, `FINISHED`, `ERROR`, `WAITING_FOR_CANCEL`, `CANCELED` or `ROLLBACK`."),
			"started_at":     computedAttribute("Start of the task."),
			"finished_at":    computedAttribute("End of the task."),
			"executing_user": userMinimalAttribute("User who started it."),
			"task_progress": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Progress of the task.",
				Attributes: map[string]schema.Attribute{
					"expected_finished_at": computedAttribute("Expected end of the task."),
					"progress_in_percent":  schema.Float64Attribute{Computed: true, MarkdownDescription: "Progress in percent."},
				},
			},
			"result":      computedAttribute("Result of the task as JSON."),
			"message":     computedAttribute("Message of the task."),
			"on_rollback": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the task is rolling back."},
			"steps": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Steps of the task.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"uuid":        computedAttribute("UUID of the step."),
					"name":        computedAttribute("Name of the step."),
					"state":       computedAttribute("State of the step."),
					"started_at":  computedAttribute("Start of the step."),
					"finished_at": computedAttribute("End of the step."),
				}},
			},
			"response_error": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Error of a failed task.",
				Attributes: map[string]schema.Attribute{
					"code":    computedAttribute("Code of the error."),
					"message": computedAttribute("Message of the error."),
				},
			},
		},
	}
}

func (d *taskDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	type step struct {
		UUID       types.String `tfsdk:"uuid"`
		Name       types.String `tfsdk:"name"`
		State      types.String `tfsdk:"state"`
		StartedAt  types.String `tfsdk:"started_at"`
		FinishedAt types.String `tfsdk:"finished_at"`
	}
	type progress struct {
		ExpectedFinishedAt types.String  `tfsdk:"expected_finished_at"`
		ProgressInPercent  types.Float64 `tfsdk:"progress_in_percent"`
	}
	var cfg struct {
		UUID          types.String        `tfsdk:"uuid"`
		Name          types.String        `tfsdk:"name"`
		State         types.String        `tfsdk:"state"`
		StartedAt     types.String        `tfsdk:"started_at"`
		FinishedAt    types.String        `tfsdk:"finished_at"`
		ExecutingUser *userMinimalModel   `tfsdk:"executing_user"`
		TaskProgress  *progress           `tfsdk:"task_progress"`
		Result        types.String        `tfsdk:"result"`
		Message       types.String        `tfsdk:"message"`
		OnRollback    types.Bool          `tfsdk:"on_rollback"`
		Steps         []step              `tfsdk:"steps"`
		ResponseError *responseErrorModel `tfsdk:"response_error"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	task, err := d.client.GetTask(ctx, cfg.UUID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading task", err.Error())
		return
	}
	cfg.Name, cfg.State = optString(task.Name), optString(task.State)
	cfg.StartedAt, cfg.FinishedAt = optString(task.StartedAt), optString(task.FinishedAt)
	cfg.ExecutingUser = userMinimal(task.ExecutingUser)
	cfg.Message, cfg.OnRollback = optString(task.Message), types.BoolPointerValue(task.OnRollback)
	if task.TaskProgress != nil {
		cfg.TaskProgress = &progress{
			ExpectedFinishedAt: optString(task.TaskProgress.ExpectedFinishedAt),
			ProgressInPercent:  types.Float64PointerValue(task.TaskProgress.ProgressInPercent),
		}
	}
	cfg.Result = optString(string(task.Result))
	cfg.Steps = make([]step, 0, len(task.Steps))
	for _, s := range task.Steps {
		cfg.Steps = append(cfg.Steps, step{
			UUID: optString(s.UUID), Name: optString(s.Name), State: optString(s.State),
			StartedAt: optString(s.StartedAt), FinishedAt: optString(s.FinishedAt),
		})
	}
	if task.ResponseError != nil {
		cfg.ResponseError = &responseErrorModel{Code: optString(task.ResponseError.Code), Message: optString(task.ResponseError.Message)}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
