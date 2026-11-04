// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	ephemeralschema "github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

// uploadTimeout bounds an upload unless the timeouts attribute says otherwise.
const uploadTimeout = 6 * time.Hour

// userObjectKind names one storage of a user.
type userObjectKind struct {
	kind scp.ObjectKind
	// singular and plural complete the type names, noun the descriptions.
	singular, plural, noun string
}

var (
	userImageKind = userObjectKind{kind: scp.ObjectKindImages, singular: "user_image", plural: "user_images", noun: "image"}
	userIsoKind   = userObjectKind{kind: scp.ObjectKindIsos, singular: "user_iso", plural: "user_isos", noun: "ISO"}
)

var (
	_ resource.Resource                = (*userObjectResource)(nil)
	_ resource.ResourceWithConfigure   = (*userObjectResource)(nil)
	_ resource.ResourceWithImportState = (*userObjectResource)(nil)

	_ ephemeral.EphemeralResourceWithConfigure = (*userObjectDownloadEphemeralResource)(nil)
)

// userObjectResource is an uploaded file of a storage of the user.
type userObjectResource struct {
	userObjectKind
	client *scp.Client
}

// NewUserImageResource is the resource factory.
func NewUserImageResource() resource.Resource {
	return &userObjectResource{userObjectKind: userImageKind}
}

// NewUserIsoResource is the resource factory.
func NewUserIsoResource() resource.Resource {
	return &userObjectResource{userObjectKind: userIsoKind}
}

type userObjectModel struct {
	ID           types.String   `tfsdk:"id"`
	Key          types.String   `tfsdk:"key"`
	Source       types.String   `tfsdk:"source"`
	SourceHash   types.String   `tfsdk:"source_hash"`
	LastModified types.String   `tfsdk:"last_modified"`
	SizeInB      types.Int64    `tfsdk:"size_in_b"`
	Type         types.String   `tfsdk:"type"`
	Timeouts     timeouts.Value `tfsdk:"timeouts"`
}

func (r *userObjectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.singular
}

func (r *userObjectResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: fmt.Sprintf("An uploaded %s of the netcup SCP user (SCP REST API). The provider uploads `source` on create, and `destroy` deletes it. netcup refuses an upload to an existing `key`, so a new value of `source_hash` replaces the %s, and without `source_hash` the provider cannot see changes of the file. The API cannot abort an upload, so a failed one leaves its parts at netcup.", r.noun, r.noun),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Equals `key`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: fmt.Sprintf("Name of the %s at netcup.", r.noun),
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"source": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Path of the local file. Changing only the path uploads nothing.",
			},
			"source_hash": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Any value that changes with the file, e.g. `filesha256(\"disk.img\")`. A new value replaces the object, also the first one after an import. Removing it changes nothing at netcup.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIf(
					func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
						resp.RequiresReplace = !req.PlanValue.IsNull()
					},
					"A new value replaces the object.",
					"A new value replaces the object.",
				)},
			},
			"last_modified": computedString("Time of the last change."),
			"size_in_b":     computedInt64("Size in bytes."),
			"type":          computedString("`DISK`, `SNAPSHOT`, `BACKUP` or `MANUAL`."),
			"timeouts":      timeouts.Attributes(ctx, timeouts.Opts{Create: true}),
		},
	}
}

func (r *userObjectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

// read returns the object with key, or nil without one.
func (r *userObjectResource) read(ctx context.Context, key string) (*scp.S3Object, error) {
	objects, err := r.client.ListObjects(ctx, r.kind)
	if err != nil {
		return nil, err
	}
	for i := range objects {
		if objects[i].Key == key {
			return &objects[i], nil
		}
	}
	return nil, nil
}

func (m *userObjectModel) store(object *scp.S3Object) {
	m.ID = types.StringValue(object.Key)
	m.Key = types.StringValue(object.Key)
	m.LastModified = optString(object.LastModified)
	m.SizeInB = types.Int64PointerValue(object.SizeInB)
	m.Type = optString(object.Type)
}

// refresh stores the object an apply just wrote.
func (r *userObjectResource) refresh(ctx context.Context, plan *userObjectModel) diag.Diagnostics {
	var diags diag.Diagnostics
	object, err := r.read(ctx, plan.Key.ValueString())
	switch {
	case err != nil:
		diags.AddError("Error reading "+r.noun, err.Error())
	case object == nil:
		diags.AddError("Error reading "+r.noun, plan.Key.ValueString()+" is missing after the upload.")
	default:
		plan.store(object)
	}
	return diags
}

// upload sends the file. The timeout spares the read that follows a finished
// upload, which would otherwise lose it.
func (r *userObjectResource) upload(ctx context.Context, limit time.Duration, plan *userObjectModel) error {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	return r.client.UploadObject(ctx, r.kind, plan.Key.ValueString(), plan.Source.ValueString())
}

func (r *userObjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan userObjectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	limit, diags := plan.Timeouts.Create(ctx, uploadTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.upload(ctx, limit, &plan); err != nil {
		resp.Diagnostics.AddError("Error uploading "+r.noun, err.Error())
		return
	}
	resp.Diagnostics.Append(r.refresh(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *userObjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state userObjectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	object, err := r.read(ctx, state.Key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading "+r.noun, err.Error())
		return
	}
	if object == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.store(object)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update stores source, the removal of source_hash or timeouts, which change
// nothing at netcup.
func (r *userObjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan userObjectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.refresh(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *userObjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state userObjectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteObject(ctx, r.kind, state.Key.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting "+r.noun, err.Error())
	}
}

func (r *userObjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("key"), req.ID)...)
}

// userObjectsDataSource lists a storage of the user.
type userObjectsDataSource struct {
	scpDataSource
	userObjectKind
}

// NewUserImagesDataSource is the data source factory.
func NewUserImagesDataSource() datasource.DataSource {
	return &userObjectsDataSource{userObjectKind: userImageKind}
}

// NewUserIsosDataSource is the data source factory.
func NewUserIsosDataSource() datasource.DataSource {
	return &userObjectsDataSource{userObjectKind: userIsoKind}
}

func (d *userObjectsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.plural
}

func (d *userObjectsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dsschema.Schema{
		MarkdownDescription: fmt.Sprintf("The uploaded %ss of the netcup SCP user (SCP REST API).", d.noun),
		Attributes: map[string]dsschema.Attribute{
			"objects": dsschema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: fmt.Sprintf("The %ss.", d.noun),
				NestedObject: dsschema.NestedAttributeObject{Attributes: map[string]dsschema.Attribute{
					"key":           dsschema.StringAttribute{Computed: true, MarkdownDescription: "Name of the object."},
					"last_modified": dsschema.StringAttribute{Computed: true, MarkdownDescription: "Time of the last change."},
					"size_in_b":     dsschema.Int64Attribute{Computed: true, MarkdownDescription: "Size in bytes."},
					"type":          dsschema.StringAttribute{Computed: true, MarkdownDescription: "`DISK`, `SNAPSHOT`, `BACKUP` or `MANUAL`."},
				}},
			},
		},
	}
}

func (d *userObjectsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	type entry struct {
		Key          types.String `tfsdk:"key"`
		LastModified types.String `tfsdk:"last_modified"`
		SizeInB      types.Int64  `tfsdk:"size_in_b"`
		Type         types.String `tfsdk:"type"`
	}
	objects, err := d.client.ListObjects(ctx, d.kind)
	if err != nil {
		resp.Diagnostics.AddError("Error listing "+d.noun+"s", err.Error())
		return
	}
	out := struct {
		Objects []entry `tfsdk:"objects"`
	}{Objects: make([]entry, 0, len(objects))}
	for _, o := range objects {
		out.Objects = append(out.Objects, entry{
			Key: types.StringValue(o.Key), LastModified: optString(o.LastModified), SizeInB: types.Int64PointerValue(o.SizeInB), Type: optString(o.Type),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, out)...)
}

// userObjectDownloadEphemeralResource reads the download of an uploaded file.
// Its URL grants access while valid, so it stays out of plan and state.
type userObjectDownloadEphemeralResource struct {
	userObjectKind
	client *scp.Client
}

// NewUserImageDownloadEphemeralResource is the ephemeral resource factory.
func NewUserImageDownloadEphemeralResource() ephemeral.EphemeralResource {
	return &userObjectDownloadEphemeralResource{userObjectKind: userImageKind}
}

// NewUserIsoDownloadEphemeralResource is the ephemeral resource factory.
func NewUserIsoDownloadEphemeralResource() ephemeral.EphemeralResource {
	return &userObjectDownloadEphemeralResource{userObjectKind: userIsoKind}
}

func (r *userObjectDownloadEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.singular + "_download"
}

func (r *userObjectDownloadEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = ephemeralschema.Schema{
		MarkdownDescription: fmt.Sprintf("The download of an uploaded %s of the netcup SCP user (SCP REST API). It needs Terraform 1.10 or newer.", r.noun),
		Attributes: map[string]ephemeralschema.Attribute{
			"key":           ephemeralschema.StringAttribute{Required: true, MarkdownDescription: fmt.Sprintf("Name of the %s at netcup.", r.noun)},
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

func (r *userObjectDownloadEphemeralResource) Configure(_ context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

func (r *userObjectDownloadEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var cfg struct {
		Key      types.String        `tfsdk:"key"`
		Filename types.String        `tfsdk:"filename"`
		URL      types.String        `tfsdk:"presigned_url"`
		Validity types.Int64         `tfsdk:"presigned_url_validity_duration_in_hours"`
		Headers  map[string][]string `tfsdk:"headers"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	info, err := r.client.GetObjectDownload(ctx, r.kind, cfg.Key.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading the download of "+r.noun, err.Error())
		return
	}
	cfg.Filename = optString(info.Filename)
	cfg.URL = types.StringValue(info.PresignedURL)
	cfg.Validity = types.Int64PointerValue(info.PresignedURLValidityDurationInHours)
	cfg.Headers = info.Headers
	resp.Diagnostics.Append(resp.Result.Set(ctx, cfg)...)
}
