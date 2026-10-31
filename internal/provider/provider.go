// SPDX-License-Identifier: MPL-2.0

// Package provider implements the netcup Terraform provider.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	fwprov "github.com/hashicorp/terraform-plugin-framework/provider"
	provschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/client"
	"github.com/hornc-greedy/terraform-provider-netcup/internal/clouddns"
	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

var _ fwprov.Provider = (*netcupProvider)(nil)

type netcupProvider struct {
	version string
}

// providerData is handed to every resource and data source via Configure.
// Each client may be nil if its credentials were not supplied; resources that need
// it must check for it (see requireCCP, requireSCP, requireCloudDNS).
type providerData struct {
	CCP      *client.Client
	SCP      *scp.Client
	CloudDNS *clouddns.Client

	// The attribute that keeps an API from being configured while planning.
	unknownCCP, unknownSCP, unknownCloudDNS string
}

// New returns the provider factory used by main and acceptance tests.
func New(version string) func() fwprov.Provider {
	return func() fwprov.Provider {
		return &netcupProvider{version: version}
	}
}

type netcupProviderModel struct {
	CustomerNumber  types.String `tfsdk:"customer_number"`
	APIKey          types.String `tfsdk:"api_key"`
	APIPassword     types.String `tfsdk:"api_password"`
	SCPRefreshTok   types.String `tfsdk:"scp_refresh_token"`
	SCPBaseURL      types.String `tfsdk:"scp_base_url"`
	CloudDNSAPIKey  types.String `tfsdk:"clouddns_api_key"`
	CloudDNSBaseURL types.String `tfsdk:"clouddns_base_url"`
}

func (p *netcupProvider) Metadata(_ context.Context, _ fwprov.MetadataRequest, resp *fwprov.MetadataResponse) {
	resp.TypeName = "netcup"
	resp.Version = p.version
}

func (p *netcupProvider) Schema(_ context.Context, _ fwprov.SchemaRequest, resp *fwprov.SchemaResponse) {
	resp.Schema = provschema.Schema{
		MarkdownDescription: "Manage netcup DNS via the CCP (DomainWebservice) API or the netcup API for " +
			"domains on CloudDNS, and reverse DNS / servers via the SCP REST API. The APIs use " +
			"independent credentials; configure whichever you need.",
		Attributes: map[string]provschema.Attribute{
			"customer_number": provschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "netcup customer number (CCP API). May also be set via `NETCUP_CUSTOMER_NUMBER`.",
			},
			"api_key": provschema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "netcup CCP API key. May also be set via `NETCUP_API_KEY`.",
			},
			"api_password": provschema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "netcup CCP API password. May also be set via `NETCUP_API_PASSWORD`.",
			},
			"scp_refresh_token": provschema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "SCP REST API offline refresh token (obtained once via the OIDC device flow). " +
					"Required only for SCP resources/data sources (reverse DNS, servers). " +
					"May also be set via `NETCUP_SCP_REFRESH_TOKEN`.",
			},
			"scp_base_url": provschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "SCP host, by default `" + scp.DefaultServer + "`. May also be set via `NETCUP_SCP_BASE_URL`.",
			},
			"clouddns_api_key": provschema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "API key of the netcup API for domains on CloudDNS, from the CCP under Master Data > API. May also be set via `NETCUP_CLOUDDNS_API_KEY`.",
			},
			"clouddns_base_url": provschema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Host of the netcup API, by default `" + clouddns.DefaultBaseURL + "`. May also be set via `NETCUP_CLOUDDNS_BASE_URL`.",
			},
		},
	}
}

func (p *netcupProvider) Configure(ctx context.Context, req fwprov.ConfigureRequest, resp *fwprov.ConfigureResponse) {
	var cfg netcupProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	customer := firstNonEmpty(cfg.CustomerNumber.ValueString(), os.Getenv("NETCUP_CUSTOMER_NUMBER"))
	apiKey := firstNonEmpty(cfg.APIKey.ValueString(), os.Getenv("NETCUP_API_KEY"))
	apiPassword := firstNonEmpty(cfg.APIPassword.ValueString(), os.Getenv("NETCUP_API_PASSWORD"))
	scpToken := firstNonEmpty(cfg.SCPRefreshTok.ValueString(), os.Getenv("NETCUP_SCP_REFRESH_TOKEN"))

	data := &providerData{
		unknownCCP: unknownAttribute(
			knownAttribute{"customer_number", cfg.CustomerNumber, "NETCUP_CUSTOMER_NUMBER"},
			knownAttribute{"api_key", cfg.APIKey, "NETCUP_API_KEY"},
			knownAttribute{"api_password", cfg.APIPassword, "NETCUP_API_PASSWORD"},
		),
		unknownSCP: unknownAttribute(
			knownAttribute{"scp_refresh_token", cfg.SCPRefreshTok, "NETCUP_SCP_REFRESH_TOKEN"},
			knownAttribute{"scp_base_url", cfg.SCPBaseURL, "NETCUP_SCP_BASE_URL"},
		),
		unknownCloudDNS: unknownAttribute(
			knownAttribute{"clouddns_api_key", cfg.CloudDNSAPIKey, "NETCUP_CLOUDDNS_API_KEY"},
			knownAttribute{"clouddns_base_url", cfg.CloudDNSBaseURL, "NETCUP_CLOUDDNS_BASE_URL"},
		),
	}
	if customer != "" && apiKey != "" && apiPassword != "" {
		data.CCP = client.New(customer, apiKey, apiPassword)
	}
	if scpToken != "" {
		data.SCP = scp.NewForServer(firstNonEmpty(cfg.SCPBaseURL.ValueString(), os.Getenv("NETCUP_SCP_BASE_URL"), scp.DefaultServer), scpToken)
	}

	if key := firstNonEmpty(cfg.CloudDNSAPIKey.ValueString(), os.Getenv("NETCUP_CLOUDDNS_API_KEY")); key != "" {
		data.CloudDNS = clouddns.New(firstNonEmpty(cfg.CloudDNSBaseURL.ValueString(), os.Getenv("NETCUP_CLOUDDNS_BASE_URL"), clouddns.DefaultBaseURL), key)
	}

	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *netcupProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewDNSRecordResource,
		NewDNSZoneResource,
		NewReverseDNSResource,
		NewSSHKeyResource,
		NewFirewallPolicyResource,
		NewServerSnapshotResource,
		NewServerFirewallResource,
		NewFailoverIPRouteResource,
		NewServerSettingsResource,
		NewCloudDNSRecordResource,
	}
}

func (p *netcupProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewDNSRecordsDataSource,
		NewServersDataSource,
		NewServerDataSource,
		NewServerInterfacesDataSource,
		NewMaintenanceDataSource,
		NewSSHKeysDataSource,
		NewFirewallPoliciesDataSource,
		NewFailoverIPsDataSource,
		NewCloudDNSDomainDataSource,
		NewCloudDNSDomainsDataSource,
		NewCloudDNSRecordDataSource,
		NewCloudDNSRecordsDataSource,
		NewCloudDNSRevisionDataSource,
		NewCloudDNSRevisionsDataSource,
		NewCloudDNSRecordTypesDataSource,
		NewCloudDNSAcmeChallengesDataSource,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
