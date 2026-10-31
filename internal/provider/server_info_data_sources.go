// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

// scpDataSource is the part the server data sources share.
type scpDataSource struct {
	client *scp.Client
}

func (d *scpDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	d.client = requireSCP(data, &resp.Diagnostics)
}

func serverIDAttribute() schema.Int64Attribute {
	return schema.Int64Attribute{Required: true, MarkdownDescription: "Id of the server."}
}

func pageAttributes(attributes map[string]schema.Attribute) map[string]schema.Attribute {
	attributes["limit"] = schema.Int64Attribute{
		Optional:            true,
		MarkdownDescription: "Maximum number of entries. Reads all entries if unset.",
		Validators:          []validator.Int64{int64validator.AtLeast(1)},
	}
	attributes["offset"] = schema.Int64Attribute{
		Optional:            true,
		MarkdownDescription: "Number of entries to skip.",
		Validators:          []validator.Int64{int64validator.AtLeast(0)},
	}
	return attributes
}

func pageOf(limit, offset types.Int64) scp.Page {
	return scp.Page{Limit: int(limit.ValueInt64()), Offset: int(offset.ValueInt64())}
}

type diskModel struct {
	Name            types.String `tfsdk:"name"`
	AllocationInMiB types.Int64  `tfsdk:"allocation_in_mib"`
	CapacityInMiB   types.Int64  `tfsdk:"capacity_in_mib"`
	StorageDriver   types.String `tfsdk:"storage_driver"`
}

func diskToModel(disk scp.Disk) diskModel {
	return diskModel{
		Name: optString(disk.Name), AllocationInMiB: types.Int64PointerValue(disk.AllocationInMiB),
		CapacityInMiB: types.Int64PointerValue(disk.CapacityInMiB), StorageDriver: optString(disk.StorageDriver),
	}
}

func diskAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name":              schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the disk."},
		"allocation_in_mib": schema.Int64Attribute{Computed: true, MarkdownDescription: "Allocated size in MiB."},
		"capacity_in_mib":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Capacity in MiB."},
		"storage_driver":    schema.StringAttribute{Computed: true, MarkdownDescription: "Storage driver of the disk."},
	}
}

// serverDisksDataSource lists the disks of a server.
type serverDisksDataSource struct{ scpDataSource }

// NewServerDisksDataSource is the data source factory.
func NewServerDisksDataSource() datasource.DataSource { return &serverDisksDataSource{} }

func (d *serverDisksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_disks"
}

func (d *serverDisksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The disks of a netcup server (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"server_id": serverIDAttribute(),
			"disks": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Disks of the server.",
				NestedObject:        schema.NestedAttributeObject{Attributes: diskAttributes()},
			},
		},
	}
}

func (d *serverDisksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		ServerID types.Int64 `tfsdk:"server_id"`
		Disks    []diskModel `tfsdk:"disks"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	disks, err := d.client.ListDisks(ctx, int(cfg.ServerID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error listing disks", err.Error())
		return
	}
	cfg.Disks = make([]diskModel, 0, len(disks))
	for _, disk := range disks {
		cfg.Disks = append(cfg.Disks, diskToModel(disk))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// serverDiskDataSource reads one disk of a server.
type serverDiskDataSource struct{ scpDataSource }

// NewServerDiskDataSource is the data source factory.
func NewServerDiskDataSource() datasource.DataSource { return &serverDiskDataSource{} }

func (d *serverDiskDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_disk"
}

func (d *serverDiskDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := diskAttributes()
	attributes["server_id"] = serverIDAttribute()
	attributes["name"] = schema.StringAttribute{Required: true, MarkdownDescription: "Name of the disk, e.g. `vda`."}
	resp.Schema = schema.Schema{
		MarkdownDescription: "One disk of a netcup server (SCP REST API).",
		Attributes:          attributes,
	}
}

func (d *serverDiskDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		diskModel
		ServerID types.Int64 `tfsdk:"server_id"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	disk, err := d.client.GetDisk(ctx, int(cfg.ServerID.ValueInt64()), cfg.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading disk", err.Error())
		return
	}
	cfg.diskModel = diskToModel(*disk)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

type userMinimalModel struct {
	ID        types.Int64  `tfsdk:"id"`
	Username  types.String `tfsdk:"username"`
	Firstname types.String `tfsdk:"firstname"`
	Lastname  types.String `tfsdk:"lastname"`
	Email     types.String `tfsdk:"email"`
	Company   types.String `tfsdk:"company"`
}

func userMinimal(user *scp.UserMinimal) *userMinimalModel {
	if user == nil {
		return nil
	}
	return &userMinimalModel{
		ID: types.Int64PointerValue(user.ID), Username: optString(user.Username), Firstname: optString(user.Firstname),
		Lastname: optString(user.Lastname), Email: optString(user.Email), Company: optString(user.Company),
	}
}

func userMinimalAttribute(description string) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Computed:            true,
		MarkdownDescription: description,
		Attributes: map[string]schema.Attribute{
			"id":        schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the user."},
			"username":  schema.StringAttribute{Computed: true, MarkdownDescription: "User name."},
			"firstname": schema.StringAttribute{Computed: true, MarkdownDescription: "First name."},
			"lastname":  schema.StringAttribute{Computed: true, MarkdownDescription: "Last name."},
			"email":     schema.StringAttribute{Computed: true, MarkdownDescription: "E-mail address."},
			"company":   schema.StringAttribute{Computed: true, MarkdownDescription: "Company."},
		},
	}
}

// serverDiskSupportedDriversDataSource lists the drivers a server supports.
type serverDiskSupportedDriversDataSource struct{ scpDataSource }

// NewServerDiskSupportedDriversDataSource is the data source factory.
func NewServerDiskSupportedDriversDataSource() datasource.DataSource {
	return &serverDiskSupportedDriversDataSource{}
}

func (d *serverDiskSupportedDriversDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_disk_supported_drivers"
}

func (d *serverDiskSupportedDriversDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The storage drivers a netcup server supports (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"server_id":         serverIDAttribute(),
			"supported_drivers": schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Supported storage drivers."},
		},
	}
}

func (d *serverDiskSupportedDriversDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		ServerID         types.Int64 `tfsdk:"server_id"`
		SupportedDrivers []string    `tfsdk:"supported_drivers"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	drivers, err := d.client.ListSupportedDrivers(ctx, int(cfg.ServerID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error listing storage drivers", err.Error())
		return
	}
	cfg.SupportedDrivers = drivers
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// serverGuestAgentDataSource reads the data of the guest agent.
type serverGuestAgentDataSource struct{ scpDataSource }

// NewServerGuestAgentDataSource is the data source factory.
func NewServerGuestAgentDataSource() datasource.DataSource { return &serverGuestAgentDataSource{} }

func (d *serverGuestAgentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_guest_agent"
}

func (d *serverGuestAgentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The data of the guest agent of a netcup server (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"server_id":             serverIDAttribute(),
			"guest_agent_available": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the guest agent is available."},
			"guest_agent_data": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Data of the guest agent as JSON, use `jsondecode` to read it. Its content changes with the agent version.",
			},
		},
	}
}

func (d *serverGuestAgentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		ServerID  types.Int64  `tfsdk:"server_id"`
		Available types.Bool   `tfsdk:"guest_agent_available"`
		Data      types.String `tfsdk:"guest_agent_data"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	agent, err := d.client.GetGuestAgent(ctx, int(cfg.ServerID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading guest agent", err.Error())
		return
	}
	cfg.Available = types.BoolPointerValue(agent.GuestAgentAvailable)
	cfg.Data = types.StringNull()
	if agent.GuestAgentData != nil {
		cfg.Data = types.StringValue(string(*agent.GuestAgentData))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// serverGuestAgentStatusDataSource reads the status of the guest agent.
type serverGuestAgentStatusDataSource struct{ scpDataSource }

// NewServerGuestAgentStatusDataSource is the data source factory.
func NewServerGuestAgentStatusDataSource() datasource.DataSource {
	return &serverGuestAgentStatusDataSource{}
}

func (d *serverGuestAgentStatusDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_guest_agent_status"
}

func (d *serverGuestAgentStatusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The status of the guest agent of a netcup server (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"server_id": serverIDAttribute(),
			"available": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the guest agent is available."},
		},
	}
}

func (d *serverGuestAgentStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		ServerID  types.Int64 `tfsdk:"server_id"`
		Available types.Bool  `tfsdk:"available"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	available, err := d.client.GetGuestAgentStatus(ctx, int(cfg.ServerID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading guest agent status", err.Error())
		return
	}
	cfg.Available = types.BoolPointerValue(available)
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// serverImageFlavoursDataSource lists the image flavours of a server.
type serverImageFlavoursDataSource struct{ scpDataSource }

// NewServerImageFlavoursDataSource is the data source factory.
func NewServerImageFlavoursDataSource() datasource.DataSource {
	return &serverImageFlavoursDataSource{}
}

func (d *serverImageFlavoursDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_image_flavours"
}

func (d *serverImageFlavoursDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The image flavours a netcup server can install (SCP REST API). `image.name` names the system and its boot mode, e.g. `Debian 13.7.0 BIOS amd64`.",
		Attributes: map[string]schema.Attribute{
			"server_id": serverIDAttribute(),
			"image_flavours": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Image flavours of the server.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":    schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the flavour."},
					"name":  schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the flavour."},
					"alias": schema.StringAttribute{Computed: true, MarkdownDescription: "Alias of the flavour."},
					"text":  schema.StringAttribute{Computed: true, MarkdownDescription: "Description of the flavour."},
					"image": schema.SingleNestedAttribute{
						Computed:            true,
						MarkdownDescription: "Image of the flavour.",
						Attributes: map[string]schema.Attribute{
							"id":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the image."},
							"name": schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the image."},
						},
					},
				}},
			},
		},
	}
}

func (d *serverImageFlavoursDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	type image struct {
		ID   types.Int64  `tfsdk:"id"`
		Name types.String `tfsdk:"name"`
	}
	type flavour struct {
		ID    types.Int64  `tfsdk:"id"`
		Name  types.String `tfsdk:"name"`
		Alias types.String `tfsdk:"alias"`
		Text  types.String `tfsdk:"text"`
		Image *image       `tfsdk:"image"`
	}
	var cfg struct {
		ServerID      types.Int64 `tfsdk:"server_id"`
		ImageFlavours []flavour   `tfsdk:"image_flavours"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	flavours, err := d.client.ListImageFlavours(ctx, int(cfg.ServerID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error listing image flavours", err.Error())
		return
	}
	cfg.ImageFlavours = make([]flavour, 0, len(flavours))
	for _, f := range flavours {
		entry := flavour{ID: types.Int64PointerValue(f.ID), Name: types.StringValue(f.Name), Alias: types.StringValue(f.Alias), Text: types.StringValue(f.Text)}
		if f.Image != nil {
			entry.Image = &image{ID: types.Int64PointerValue(f.Image.ID), Name: types.StringValue(f.Image.Name)}
		}
		cfg.ImageFlavours = append(cfg.ImageFlavours, entry)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// serverIsoImagesDataSource lists the ISO images of a server.
type serverIsoImagesDataSource struct{ scpDataSource }

// NewServerIsoImagesDataSource is the data source factory.
func NewServerIsoImagesDataSource() datasource.DataSource { return &serverIsoImagesDataSource{} }

func (d *serverIsoImagesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_iso_images"
}

func (d *serverIsoImagesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The ISO images a netcup server can attach (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"server_id": serverIDAttribute(),
			"iso_images": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "ISO images of the server.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":           schema.Int64Attribute{Computed: true, MarkdownDescription: "Id of the image."},
					"name":         schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the image."},
					"filename":     schema.StringAttribute{Computed: true, MarkdownDescription: "File name of the image."},
					"description":  schema.StringAttribute{Computed: true, MarkdownDescription: "Description of the image."},
					"architecture": schema.StringAttribute{Computed: true, MarkdownDescription: "CPU architecture of the image."},
				}},
			},
		},
	}
}

func (d *serverIsoImagesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	type image struct {
		ID           types.Int64  `tfsdk:"id"`
		Name         types.String `tfsdk:"name"`
		Filename     types.String `tfsdk:"filename"`
		Description  types.String `tfsdk:"description"`
		Architecture types.String `tfsdk:"architecture"`
	}
	var cfg struct {
		ServerID  types.Int64 `tfsdk:"server_id"`
		IsoImages []image     `tfsdk:"iso_images"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	images, err := d.client.ListIsoImages(ctx, int(cfg.ServerID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error listing ISO images", err.Error())
		return
	}
	cfg.IsoImages = make([]image, 0, len(images))
	for _, i := range images {
		cfg.IsoImages = append(cfg.IsoImages, image{
			ID: types.Int64PointerValue(i.ID), Name: types.StringValue(i.Name), Filename: types.StringValue(i.Filename),
			Description: types.StringPointerValue(i.Description), Architecture: types.StringValue(i.Architecture),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// serverLogsDataSource lists the logs of a server.
type serverLogsDataSource struct{ scpDataSource }

// NewServerLogsDataSource is the data source factory.
func NewServerLogsDataSource() datasource.DataSource { return &serverLogsDataSource{} }

func (d *serverLogsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_logs"
}

func (d *serverLogsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The logs of a netcup server (SCP REST API).",
		Attributes: pageAttributes(map[string]schema.Attribute{
			"server_id": serverIDAttribute(),
			"logs": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Log entries of the server.",
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

func (d *serverLogsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	type entry struct {
		Type          types.String      `tfsdk:"type"`
		ExecutingUser *userMinimalModel `tfsdk:"executing_user"`
		Date          types.String      `tfsdk:"date"`
		LogKey        types.String      `tfsdk:"log_key"`
		Message       types.String      `tfsdk:"message"`
	}
	var cfg struct {
		ServerID types.Int64 `tfsdk:"server_id"`
		Limit    types.Int64 `tfsdk:"limit"`
		Offset   types.Int64 `tfsdk:"offset"`
		Logs     []entry     `tfsdk:"logs"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	logs, err := d.client.ListServerLogs(ctx, int(cfg.ServerID.ValueInt64()), pageOf(cfg.Limit, cfg.Offset))
	if err != nil {
		resp.Diagnostics.AddError("Error listing server logs", err.Error())
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

// serverMetricsDataSource reads one metric of a server.
type serverMetricsDataSource struct {
	scpDataSource
	typeName    string
	description string
	metric      scp.Metric
}

// NewServerMetricsCPUDataSource is the data source factory.
func NewServerMetricsCPUDataSource() datasource.DataSource {
	return &serverMetricsDataSource{typeName: "cpu", description: "CPU", metric: scp.MetricCPU}
}

// NewServerMetricsDiskDataSource is the data source factory.
func NewServerMetricsDiskDataSource() datasource.DataSource {
	return &serverMetricsDataSource{typeName: "disk", description: "disk", metric: scp.MetricDisk}
}

// NewServerMetricsNetworkDataSource is the data source factory.
func NewServerMetricsNetworkDataSource() datasource.DataSource {
	return &serverMetricsDataSource{typeName: "network", description: "network", metric: scp.MetricNetwork}
}

// NewServerMetricsNetworkPacketDataSource is the data source factory.
func NewServerMetricsNetworkPacketDataSource() datasource.DataSource {
	return &serverMetricsDataSource{typeName: "network_packet", description: "network packet", metric: scp.MetricNetworkPacket}
}

func (d *serverMetricsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_metrics_" + d.typeName
}

func (d *serverMetricsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The " + d.description + " metrics of a netcup server (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"server_id": serverIDAttribute(),
			"hours": schema.Int64Attribute{
				Optional:            true,
				MarkdownDescription: "Number of hours to read, at most 1440.",
				Validators:          []validator.Int64{int64validator.AtMost(1440)},
			},
			"data": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Metrics as JSON, a map from an ISO-8601 timestamp to the values at that time. Use `jsondecode` to read it.",
			},
		},
	}
}

func (d *serverMetricsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg struct {
		ServerID types.Int64  `tfsdk:"server_id"`
		Hours    types.Int64  `tfsdk:"hours"`
		Data     types.String `tfsdk:"data"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data, err := d.client.GetMetrics(ctx, int(cfg.ServerID.ValueInt64()), d.metric, cfg.Hours.ValueInt64Pointer())
	if err != nil {
		resp.Diagnostics.AddError("Error reading server metrics", err.Error())
		return
	}
	cfg.Data = types.StringValue(string(data))
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// serverSnapshotsDataSource lists the snapshots of a server.
type serverSnapshotsDataSource struct{ scpDataSource }

// NewServerSnapshotsDataSource is the data source factory.
func NewServerSnapshotsDataSource() datasource.DataSource { return &serverSnapshotsDataSource{} }

func (d *serverSnapshotsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_snapshots"
}

func (d *serverSnapshotsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The snapshots of a netcup server (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"server_id": serverIDAttribute(),
			"snapshots": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Snapshots of the server.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"uuid":                 schema.StringAttribute{Computed: true, MarkdownDescription: "UUID of the snapshot."},
					"name":                 schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the snapshot."},
					"description":          schema.StringAttribute{Computed: true, MarkdownDescription: "Description of the snapshot."},
					"disks":                schema.ListAttribute{Computed: true, ElementType: types.StringType, MarkdownDescription: "Disks of the snapshot."},
					"creation_time":        schema.StringAttribute{Computed: true, MarkdownDescription: "Creation time."},
					"state":                schema.StringAttribute{Computed: true, MarkdownDescription: "State of the snapshot."},
					"online":               schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether it is an online snapshot."},
					"exported":             schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the snapshot is exported."},
					"exported_size_in_kib": schema.Int64Attribute{Computed: true, MarkdownDescription: "Size of the export in KiB."},
				}},
			},
		},
	}
}

func (d *serverSnapshotsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	type entry struct {
		UUID         types.String `tfsdk:"uuid"`
		Name         types.String `tfsdk:"name"`
		Description  types.String `tfsdk:"description"`
		Disks        []string     `tfsdk:"disks"`
		CreationTime types.String `tfsdk:"creation_time"`
		State        types.String `tfsdk:"state"`
		Online       types.Bool   `tfsdk:"online"`
		Exported     types.Bool   `tfsdk:"exported"`
		ExportedSize types.Int64  `tfsdk:"exported_size_in_kib"`
	}
	var cfg struct {
		ServerID  types.Int64 `tfsdk:"server_id"`
		Snapshots []entry     `tfsdk:"snapshots"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	snapshots, err := d.client.ListSnapshots(ctx, int(cfg.ServerID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error listing snapshots", err.Error())
		return
	}
	cfg.Snapshots = make([]entry, 0, len(snapshots))
	for _, s := range snapshots {
		cfg.Snapshots = append(cfg.Snapshots, entry{
			UUID: optString(s.UUID), Name: optString(s.Name), Description: optString(s.Description), Disks: s.Disks,
			CreationTime: optString(s.CreationTime), State: optString(s.State), Online: types.BoolPointerValue(s.Online),
			Exported: types.BoolPointerValue(s.Exported), ExportedSize: types.Int64PointerValue(s.ExportedSizeInKiB),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// serverSnapshotDryrunDataSource asks whether a snapshot is possible.
type serverSnapshotDryrunDataSource struct{ scpDataSource }

var _ datasource.DataSourceWithValidateConfig = (*serverSnapshotDryrunDataSource)(nil)

type responseErrorModel struct {
	Code    types.String `tfsdk:"code"`
	Message types.String `tfsdk:"message"`
}

type serverSnapshotDryrunModel struct {
	ServerID       types.Int64          `tfsdk:"server_id"`
	DiskName       types.String         `tfsdk:"disk_name"`
	OnlineSnapshot types.Bool           `tfsdk:"online_snapshot"`
	Errors         []responseErrorModel `tfsdk:"errors"`
}

// NewServerSnapshotDryrunDataSource is the data source factory.
func NewServerSnapshotDryrunDataSource() datasource.DataSource {
	return &serverSnapshotDryrunDataSource{}
}

func (d *serverSnapshotDryrunDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_snapshot_dryrun"
}

func (d *serverSnapshotDryrunDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Checks whether a snapshot of a netcup server is possible (SCP REST API).",
		Attributes: map[string]schema.Attribute{
			"server_id":       serverIDAttribute(),
			"disk_name":       schema.StringAttribute{Optional: true, MarkdownDescription: "Disk to snapshot. Needed unless `online_snapshot` is `true`."},
			"online_snapshot": schema.BoolAttribute{Optional: true, MarkdownDescription: "Whether to check an online snapshot."},
			"errors": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Errors netcup would give for this snapshot. Empty if the snapshot is possible.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"code":    schema.StringAttribute{Computed: true, MarkdownDescription: "Code of the error."},
					"message": schema.StringAttribute{Computed: true, MarkdownDescription: "Message of the error."},
				}},
			},
		},
	}
}

// ValidateConfig enforces the rule of the specification that an offline
// snapshot names its disk.
func (d *serverSnapshotDryrunDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var cfg serverSnapshotDryrunModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.OnlineSnapshot.IsUnknown() || cfg.DiskName.IsUnknown() {
		return
	}
	if !cfg.OnlineSnapshot.ValueBool() && cfg.DiskName.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("disk_name"), "Missing disk_name",
			`An offline snapshot needs disk_name, e.g. "vda". Set it or set online_snapshot = true.`)
	}
}

func (d *serverSnapshotDryrunDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg serverSnapshotDryrunModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	found, err := d.client.DryRunSnapshot(ctx, int(cfg.ServerID.ValueInt64()), cfg.DiskName.ValueString(), cfg.OnlineSnapshot.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError("Error checking snapshot", err.Error())
		return
	}
	cfg.Errors = make([]responseErrorModel, 0, len(found))
	for _, e := range found {
		cfg.Errors = append(cfg.Errors, responseErrorModel{Code: optString(e.Code), Message: optString(e.Message)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}

// tasksDataSource lists the tasks of the account.
type tasksDataSource struct{ scpDataSource }

// NewTasksDataSource is the data source factory.
func NewTasksDataSource() datasource.DataSource { return &tasksDataSource{} }

func (d *tasksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tasks"
}

func (d *tasksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The tasks of the netcup SCP account.",
		Attributes: pageAttributes(map[string]schema.Attribute{
			"q":         schema.StringAttribute{Optional: true, MarkdownDescription: "Search term."},
			"server_id": schema.Int64Attribute{Optional: true, MarkdownDescription: "Only tasks of this server."},
			"state": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Only tasks in this state, `PENDING`, `RUNNING`, `FINISHED`, `ERROR`, `WAITING_FOR_CANCEL` or `CANCELED`.",
				Validators:          []validator.String{stringvalidator.OneOf("PENDING", "RUNNING", "FINISHED", "ERROR", "WAITING_FOR_CANCEL", "CANCELED")},
			},
			"tasks": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Matching tasks.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"uuid":           schema.StringAttribute{Computed: true, MarkdownDescription: "UUID of the task."},
					"name":           schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the task."},
					"state":          schema.StringAttribute{Computed: true, MarkdownDescription: "State of the task."},
					"executing_user": userMinimalAttribute("User who executed it."),
					"started_at":     schema.StringAttribute{Computed: true, MarkdownDescription: "Start time."},
					"finished_at":    schema.StringAttribute{Computed: true, MarkdownDescription: "End time."},
					"message":        schema.StringAttribute{Computed: true, MarkdownDescription: "Message of the task."},
					"on_rollback":    schema.BoolAttribute{Computed: true, MarkdownDescription: "The `onRollback` value of the SCP REST API."},
					"task_progress": schema.SingleNestedAttribute{
						Computed:            true,
						MarkdownDescription: "Progress of the task.",
						Attributes: map[string]schema.Attribute{
							"expected_finished_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Expected end time."},
							"progress_in_percent":  schema.Float64Attribute{Computed: true, MarkdownDescription: "Progress in percent."},
						},
					},
				}},
			},
		}),
	}
}

func (d *tasksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	type progress struct {
		ExpectedFinishedAt types.String  `tfsdk:"expected_finished_at"`
		ProgressInPercent  types.Float64 `tfsdk:"progress_in_percent"`
	}
	type entry struct {
		UUID          types.String      `tfsdk:"uuid"`
		Name          types.String      `tfsdk:"name"`
		State         types.String      `tfsdk:"state"`
		ExecutingUser *userMinimalModel `tfsdk:"executing_user"`
		StartedAt     types.String      `tfsdk:"started_at"`
		FinishedAt    types.String      `tfsdk:"finished_at"`
		Message       types.String      `tfsdk:"message"`
		OnRollback    types.Bool        `tfsdk:"on_rollback"`
		TaskProgress  *progress         `tfsdk:"task_progress"`
	}
	var cfg struct {
		Q        types.String `tfsdk:"q"`
		ServerID types.Int64  `tfsdk:"server_id"`
		State    types.String `tfsdk:"state"`
		Limit    types.Int64  `tfsdk:"limit"`
		Offset   types.Int64  `tfsdk:"offset"`
		Tasks    []entry      `tfsdk:"tasks"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tasks, err := d.client.ListTasks(ctx, scp.TaskFilter{
		Q: cfg.Q.ValueStringPointer(), ServerID: cfg.ServerID.ValueInt64Pointer(), State: cfg.State.ValueStringPointer(),
	}, pageOf(cfg.Limit, cfg.Offset))
	if err != nil {
		resp.Diagnostics.AddError("Error listing tasks", err.Error())
		return
	}
	cfg.Tasks = make([]entry, 0, len(tasks))
	for _, t := range tasks {
		task := entry{
			UUID: optString(t.UUID), Name: optString(t.Name), State: optString(t.State), ExecutingUser: userMinimal(t.ExecutingUser),
			StartedAt: optString(t.StartedAt), FinishedAt: optString(t.FinishedAt),
			Message: optString(t.Message), OnRollback: types.BoolPointerValue(t.OnRollback),
		}
		if t.TaskProgress != nil {
			task.TaskProgress = &progress{
				ExpectedFinishedAt: optString(t.TaskProgress.ExpectedFinishedAt),
				ProgressInPercent:  types.Float64PointerValue(t.TaskProgress.ProgressInPercent),
			}
		}
		cfg.Tasks = append(cfg.Tasks, task)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, cfg)...)
}
