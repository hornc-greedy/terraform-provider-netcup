// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

var (
	_ ephemeral.EphemeralResource              = (*serverGPUDriverEphemeralResource)(nil)
	_ ephemeral.EphemeralResourceWithConfigure = (*serverGPUDriverEphemeralResource)(nil)
)

// serverGPUDriverEphemeralResource reads the download of the GPU driver. Its URL
// grants access while it is valid, so it stays out of plan and state.
type serverGPUDriverEphemeralResource struct {
	client *scp.Client
}

// NewServerGPUDriverEphemeralResource is the ephemeral resource factory.
func NewServerGPUDriverEphemeralResource() ephemeral.EphemeralResource {
	return &serverGPUDriverEphemeralResource{}
}

func (r *serverGPUDriverEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server_gpu_driver"
}

func (r *serverGPUDriverEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The download of the GPU driver of a netcup server (SCP REST API). It needs Terraform 1.10 or newer. netcup answers with an error if the server has none.",
		Attributes: map[string]schema.Attribute{
			"server_id":     schema.Int64Attribute{Required: true, MarkdownDescription: "Id of the server."},
			"filename":      schema.StringAttribute{Computed: true, MarkdownDescription: "Name of the file."},
			"presigned_url": schema.StringAttribute{Computed: true, Sensitive: true, MarkdownDescription: "Download URL."},
			"presigned_url_validity_duration_in_hours": schema.Int64Attribute{Computed: true, MarkdownDescription: "Validity of the URL in hours."},
			"headers": schema.MapAttribute{
				Computed:            true,
				Sensitive:           true,
				ElementType:         types.ListType{ElemType: types.StringType},
				MarkdownDescription: "Headers to send with the download.",
			},
		},
	}
}

func (r *serverGPUDriverEphemeralResource) Configure(_ context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	r.client = requireSCP(data, &resp.Diagnostics)
}

func (r *serverGPUDriverEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var cfg struct {
		ServerID types.Int64         `tfsdk:"server_id"`
		Filename types.String        `tfsdk:"filename"`
		URL      types.String        `tfsdk:"presigned_url"`
		Validity types.Int64         `tfsdk:"presigned_url_validity_duration_in_hours"`
		Headers  map[string][]string `tfsdk:"headers"`
	}
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	info, err := r.client.GetGPUDriver(ctx, int(cfg.ServerID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Error reading GPU driver", err.Error())
		return
	}
	cfg.Filename = optString(info.Filename)
	cfg.URL = types.StringValue(info.PresignedURL)
	cfg.Validity = types.Int64PointerValue(info.PresignedURLValidityDurationInHours)
	cfg.Headers = info.Headers
	resp.Diagnostics.Append(resp.Result.Set(ctx, cfg)...)
}
