// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/clouddns"
)

// cloudDNSDataSource is the part all CloudDNS data sources share.
type cloudDNSDataSource struct {
	client *clouddns.Client
}

func (d *cloudDNSDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	d.client = requireCloudDNS(data, &resp.Diagnostics)
}

// revisionOrActive returns the given revision, or the active one if it is null.
func (d *cloudDNSDataSource) revisionOrActive(ctx context.Context, domainID int64, identifier types.String) (string, error) {
	if !identifier.IsNull() {
		return identifier.ValueString(), nil
	}
	active, err := d.client.ActiveRevision(ctx, domainID)
	if err != nil {
		return "", err
	}
	return active.Identifier, nil
}

type cloudDNSDomainModel struct {
	ID           types.Int64  `tfsdk:"id"`
	FQDN         types.String `tfsdk:"fqdn"`
	IsDNSManaged types.Bool   `tfsdk:"is_dns_managed"`
}

func domainModel(d clouddns.Domain) cloudDNSDomainModel {
	return cloudDNSDomainModel{ID: types.Int64Value(d.ID), FQDN: types.StringValue(d.FQDN), IsDNSManaged: types.BoolValue(d.IsDNSManaged)}
}

// cloudDNSDomainDataSource reads one domain by name.
type cloudDNSDomainDataSource struct{ cloudDNSDataSource }

// NewCloudDNSDomainDataSource is the data source factory.
func NewCloudDNSDomainDataSource() datasource.DataSource { return &cloudDNSDomainDataSource{} }

func (d *cloudDNSDomainDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clouddns_domain"
}

func (d *cloudDNSDomainDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One domain of the netcup API key, looked up by name.",
		Attributes: map[string]schema.Attribute{
			"fqdn":           schema.StringAttribute{Required: true, MarkdownDescription: "Domain name, e.g. `example.com`."},
			"id":             schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the domain."},
			"is_dns_managed": schema.BoolAttribute{Computed: true, MarkdownDescription: "The `isDnsManaged` value of the netcup API."},
		},
	}
}

func (d *cloudDNSDomainDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg cloudDNSDomainModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	domain, err := d.client.DomainByFQDN(ctx, cfg.FQDN.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading domain", err.Error())
		return
	}
	if domain == nil {
		resp.Diagnostics.AddError("Domain not found", fmt.Sprintf("The API key has no domain %q.", cfg.FQDN.ValueString()))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, domainModel(*domain))...)
}

// cloudDNSDomainsDataSource lists all domains.
type cloudDNSDomainsDataSource struct{ cloudDNSDataSource }

// NewCloudDNSDomainsDataSource is the data source factory.
func NewCloudDNSDomainsDataSource() datasource.DataSource { return &cloudDNSDomainsDataSource{} }

func (d *cloudDNSDomainsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clouddns_domains"
}

func (d *cloudDNSDomainsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "All domains of the netcup API key.",
		Attributes: map[string]schema.Attribute{
			"domains": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Domains of the API key.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":             schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the domain."},
					"fqdn":           schema.StringAttribute{Computed: true, MarkdownDescription: "Domain name."},
					"is_dns_managed": schema.BoolAttribute{Computed: true, MarkdownDescription: "The `isDnsManaged` value of the netcup API."},
				}},
			},
		},
	}
}

func (d *cloudDNSDomainsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	domains, err := d.client.Domains(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing domains", err.Error())
		return
	}
	state := struct {
		Domains []cloudDNSDomainModel `tfsdk:"domains"`
	}{Domains: make([]cloudDNSDomainModel, 0, len(domains))}
	for _, domain := range domains {
		state.Domains = append(state.Domains, domainModel(domain))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

type cloudDNSRecordEntry struct {
	Identifier types.String `tfsdk:"identifier"`
	Name       types.String `tfsdk:"name"`
	Type       types.String `tfsdk:"type"`
	RData      types.String `tfsdk:"rdata"`
	TTL        types.Int64  `tfsdk:"ttl"`
	Immutable  types.Bool   `tfsdk:"immutable"`
}

func recordEntry(r clouddns.Record) cloudDNSRecordEntry {
	return cloudDNSRecordEntry{
		Identifier: types.StringValue(r.Identifier), Name: types.StringValue(r.Name), Type: types.StringValue(r.Type),
		RData: types.StringValue(r.RData), TTL: types.Int64PointerValue(r.TTL), Immutable: types.BoolValue(r.Immutable),
	}
}

func recordAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"identifier": schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier of the record."},
		"name":       schema.StringAttribute{Computed: true, MarkdownDescription: "Record name relative to the zone."},
		"type":       schema.StringAttribute{Computed: true, MarkdownDescription: "Record type."},
		"rdata":      schema.StringAttribute{Computed: true, MarkdownDescription: "Record data in zone file format."},
		"ttl":        schema.Int64Attribute{Computed: true, MarkdownDescription: "TTL in seconds, if the record has one."},
		"immutable":  schema.BoolAttribute{Computed: true, MarkdownDescription: "The `immutable` value of the netcup API."},
	}
}

// cloudDNSRecordsDataSource lists the records of a revision.
type cloudDNSRecordsDataSource struct{ cloudDNSDataSource }

// NewCloudDNSRecordsDataSource is the data source factory.
func NewCloudDNSRecordsDataSource() datasource.DataSource { return &cloudDNSRecordsDataSource{} }

func (d *cloudDNSRecordsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clouddns_records"
}

func (d *cloudDNSRecordsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The records of a revision of a domain on CloudDNS.",
		Attributes: map[string]schema.Attribute{
			"domain_id":   schema.Int64Attribute{Required: true, MarkdownDescription: "Id of the domain."},
			"revision_id": schema.StringAttribute{Optional: true, MarkdownDescription: "Identifier of the revision. Defaults to the active one."},
			"records": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Records of the revision.",
				NestedObject:        schema.NestedAttributeObject{Attributes: recordAttributes()},
			},
		},
	}
}

func (d *cloudDNSRecordsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		DomainID   types.Int64           `tfsdk:"domain_id"`
		RevisionID types.String          `tfsdk:"revision_id"`
		Records    []cloudDNSRecordEntry `tfsdk:"records"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	revision, err := d.revisionOrActive(ctx, cfg.DomainID.ValueInt64(), cfg.RevisionID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading revision", err.Error())
		return
	}
	records, err := d.client.Records(ctx, cfg.DomainID.ValueInt64(), revision)
	if err != nil {
		resp.Diagnostics.AddError("Error listing records", err.Error())
		return
	}
	cfg.Records = make([]cloudDNSRecordEntry, 0, len(records))
	for _, r := range records {
		cfg.Records = append(cfg.Records, recordEntry(r))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// cloudDNSRecordDataSource reads one record by identifier.
type cloudDNSRecordDataSource struct{ cloudDNSDataSource }

// NewCloudDNSRecordDataSource is the data source factory.
func NewCloudDNSRecordDataSource() datasource.DataSource { return &cloudDNSRecordDataSource{} }

func (d *cloudDNSRecordDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clouddns_record"
}

func (d *cloudDNSRecordDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := recordAttributes()
	attributes["identifier"] = schema.StringAttribute{Required: true, MarkdownDescription: "Identifier of the record."}
	attributes["domain_id"] = schema.Int64Attribute{Required: true, MarkdownDescription: "Id of the domain."}
	attributes["revision_id"] = schema.StringAttribute{Optional: true, MarkdownDescription: "Identifier of the revision. Defaults to the active one."}
	resp.Schema = schema.Schema{
		MarkdownDescription: "One record of a revision of a domain on CloudDNS.",
		Attributes:          attributes,
	}
}

func (d *cloudDNSRecordDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	type model struct {
		cloudDNSRecordEntry
		DomainID   types.Int64  `tfsdk:"domain_id"`
		RevisionID types.String `tfsdk:"revision_id"`
	}
	var cfg model
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	revision, err := d.revisionOrActive(ctx, cfg.DomainID.ValueInt64(), cfg.RevisionID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading revision", err.Error())
		return
	}
	record, err := d.client.RecordByIdentifier(ctx, cfg.DomainID.ValueInt64(), revision, cfg.Identifier.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading record", err.Error())
		return
	}
	cfg.cloudDNSRecordEntry = recordEntry(*record)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

type cloudDNSRevisionEntry struct {
	Identifier types.String `tfsdk:"identifier"`
	CreatedAt  types.String `tfsdk:"created_at"`
	ModifiedAt types.String `tfsdk:"modified_at"`
	State      types.String `tfsdk:"state"`
	Serial     types.Int64  `tfsdk:"serial"`
}

func revisionEntry(r clouddns.Revision) cloudDNSRevisionEntry {
	return cloudDNSRevisionEntry{
		Identifier: types.StringValue(r.Identifier), CreatedAt: types.StringValue(r.CreatedAt), ModifiedAt: types.StringValue(r.ModifiedAt),
		State: types.StringValue(r.State), Serial: types.Int64PointerValue(r.Serial),
	}
}

func revisionAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"identifier":  schema.StringAttribute{Computed: true, MarkdownDescription: "Identifier of the revision."},
		"created_at":  schema.StringAttribute{Computed: true, MarkdownDescription: "Creation time."},
		"modified_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Time of the last change."},
		"state":       schema.StringAttribute{Computed: true, MarkdownDescription: "State of the revision."},
		"serial":      schema.Int64Attribute{Computed: true, MarkdownDescription: "Serial of the revision, if it has one."},
	}
}

// cloudDNSRevisionsDataSource lists the revisions of a domain.
type cloudDNSRevisionsDataSource struct{ cloudDNSDataSource }

// NewCloudDNSRevisionsDataSource is the data source factory.
func NewCloudDNSRevisionsDataSource() datasource.DataSource { return &cloudDNSRevisionsDataSource{} }

func (d *cloudDNSRevisionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clouddns_revisions"
}

func (d *cloudDNSRevisionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The revisions of a domain on CloudDNS.",
		Attributes: map[string]schema.Attribute{
			"domain_id": schema.Int64Attribute{Required: true, MarkdownDescription: "Id of the domain."},
			"revisions": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Revisions of the domain.",
				NestedObject:        schema.NestedAttributeObject{Attributes: revisionAttributes()},
			},
		},
	}
}

func (d *cloudDNSRevisionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		DomainID  types.Int64             `tfsdk:"domain_id"`
		Revisions []cloudDNSRevisionEntry `tfsdk:"revisions"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	revisions, err := d.client.Revisions(ctx, cfg.DomainID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error listing revisions", err.Error())
		return
	}
	cfg.Revisions = make([]cloudDNSRevisionEntry, 0, len(revisions))
	for _, r := range revisions {
		cfg.Revisions = append(cfg.Revisions, revisionEntry(r))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// cloudDNSRevisionDataSource reads one revision.
type cloudDNSRevisionDataSource struct{ cloudDNSDataSource }

// NewCloudDNSRevisionDataSource is the data source factory.
func NewCloudDNSRevisionDataSource() datasource.DataSource { return &cloudDNSRevisionDataSource{} }

func (d *cloudDNSRevisionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clouddns_revision"
}

func (d *cloudDNSRevisionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := revisionAttributes()
	attributes["identifier"] = schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Identifier of the revision. Defaults to the active one."}
	attributes["domain_id"] = schema.Int64Attribute{Required: true, MarkdownDescription: "Id of the domain."}
	resp.Schema = schema.Schema{
		MarkdownDescription: "One revision of a domain on CloudDNS.",
		Attributes:          attributes,
	}
}

func (d *cloudDNSRevisionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	type model struct {
		cloudDNSRevisionEntry
		DomainID types.Int64 `tfsdk:"domain_id"`
	}
	var cfg model
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var revision *clouddns.Revision
	var err error
	if cfg.Identifier.IsNull() {
		revision, err = d.client.ActiveRevision(ctx, cfg.DomainID.ValueInt64())
	} else {
		revision, err = d.client.Revision(ctx, cfg.DomainID.ValueInt64(), cfg.Identifier.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading revision", err.Error())
		return
	}
	cfg.cloudDNSRevisionEntry = revisionEntry(*revision)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// cloudDNSRecordTypesDataSource lists the supported record types.
type cloudDNSRecordTypesDataSource struct{ cloudDNSDataSource }

// NewCloudDNSRecordTypesDataSource is the data source factory.
func NewCloudDNSRecordTypesDataSource() datasource.DataSource {
	return &cloudDNSRecordTypesDataSource{}
}

func (d *cloudDNSRecordTypesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clouddns_record_types"
}

func (d *cloudDNSRecordTypesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The record types the netcup API supports.",
		Attributes: map[string]schema.Attribute{
			"types": schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Supported record types."},
		},
	}
}

func (d *cloudDNSRecordTypesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	recordTypes, err := d.client.RecordTypes(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing record types", err.Error())
		return
	}
	state := struct {
		Types []string `tfsdk:"types"`
	}{Types: recordTypes}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// cloudDNSAcmeChallengesDataSource lists the ACME challenges of a domain.
type cloudDNSAcmeChallengesDataSource struct{ cloudDNSDataSource }

// NewCloudDNSAcmeChallengesDataSource is the data source factory.
func NewCloudDNSAcmeChallengesDataSource() datasource.DataSource {
	return &cloudDNSAcmeChallengesDataSource{}
}

func (d *cloudDNSAcmeChallengesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_clouddns_acme_challenges"
}

func (d *cloudDNSAcmeChallengesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The ACME challenges of a domain on CloudDNS.",
		Attributes: map[string]schema.Attribute{
			"domain_id": schema.Int64Attribute{Required: true, MarkdownDescription: "Id of the domain."},
			"challenges": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "ACME challenges of the domain.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"scope":  schema.StringAttribute{Computed: true, MarkdownDescription: "`@` for the zone apex or a hostname label."},
					"fqdn":   schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the challenge record."},
					"value":  schema.StringAttribute{Computed: true, MarkdownDescription: "Challenge value."},
					"status": schema.StringAttribute{Computed: true, MarkdownDescription: "Status of the challenge."},
				}},
			},
		},
	}
}

func (d *cloudDNSAcmeChallengesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	type entry struct {
		Scope  types.String `tfsdk:"scope"`
		FQDN   types.String `tfsdk:"fqdn"`
		Value  types.String `tfsdk:"value"`
		Status types.String `tfsdk:"status"`
	}
	var cfg struct {
		DomainID   types.Int64 `tfsdk:"domain_id"`
		Challenges []entry     `tfsdk:"challenges"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	challenges, err := d.client.Challenges(ctx, cfg.DomainID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error listing ACME challenges", err.Error())
		return
	}
	cfg.Challenges = make([]entry, 0, len(challenges))
	for _, c := range challenges {
		cfg.Challenges = append(cfg.Challenges, entry{
			Scope: types.StringValue(c.Scope), FQDN: types.StringValue(c.FQDN), Value: types.StringValue(c.Value), Status: types.StringValue(c.Status),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
