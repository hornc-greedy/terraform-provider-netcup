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
	_ datasource.DataSource              = (*sshKeysDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*sshKeysDataSource)(nil)
)

type sshKeysDataSource struct {
	client *scp.Client
}

// NewSSHKeysDataSource is the data source factory.
func NewSSHKeysDataSource() datasource.DataSource {
	return &sshKeysDataSource{}
}

type sshKeysModel struct {
	Keys []sshKeyEntryModel `tfsdk:"keys"`
}

type sshKeyEntryModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Key       types.String `tfsdk:"key"`
	CreatedAt types.String `tfsdk:"created_at"`
}

func (d *sshKeysDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_keys"
}

func (d *sshKeysDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "All SSH keys of the netcup SCP account.",
		Attributes: map[string]schema.Attribute{
			"keys": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "SSH keys.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":         schema.StringAttribute{Computed: true, MarkdownDescription: "Key id."},
						"name":       schema.StringAttribute{Computed: true, MarkdownDescription: "Key name."},
						"key":        schema.StringAttribute{Computed: true, MarkdownDescription: "Public key."},
						"created_at": schema.StringAttribute{Computed: true, MarkdownDescription: "Creation timestamp."},
					},
				},
			},
		},
	}
}

func (d *sshKeysDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	data := providerDataFrom(req.ProviderData, &resp.Diagnostics)
	if data == nil {
		return
	}
	d.client = requireSCP(data, &resp.Diagnostics)
}

func (d *sshKeysDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	keys, err := d.client.ListSSHKeys(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error listing SSH keys", err.Error())
		return
	}
	var state sshKeysModel
	state.Keys = make([]sshKeyEntryModel, 0, len(keys))
	for _, k := range keys {
		state.Keys = append(state.Keys, sshKeyEntryModel{
			ID:        types.StringValue(strconv.Itoa(k.ID)),
			Name:      types.StringValue(k.Name),
			Key:       types.StringValue(k.Key),
			CreatedAt: types.StringValue(k.CreatedAt),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}
