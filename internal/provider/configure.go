// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/client"
	"github.com/hornc-greedy/terraform-provider-netcup/internal/clouddns"
	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

// providerDataFrom converts a raw ConfigureRequest.ProviderData into
// *providerData, adding a diagnostic on type mismatch. providerData is nil
// during the early-validation Configure call (ProviderData == nil), which
// callers must treat as "skip".
func providerDataFrom(raw any, diags *diag.Diagnostics) *providerData {
	if raw == nil {
		return nil
	}
	data, ok := raw.(*providerData)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", raw))
		return nil
	}
	return data
}

// knownAttribute is a provider attribute with its environment variable.
type knownAttribute struct {
	name  string
	value types.String
	env   string
}

// unknownAttribute returns the first attribute that has no value yet and no
// environment variable to fall back to.
func unknownAttribute(attributes ...knownAttribute) string {
	for _, a := range attributes {
		if a.value.IsUnknown() && os.Getenv(a.env) == "" {
			return a.name
		}
	}
	return ""
}

// addUnknownAttribute reports an API whose attribute is not known while planning.
func addUnknownAttribute(name string, diags *diag.Diagnostics) bool {
	if name == "" {
		return false
	}
	diags.AddError("Unknown provider attribute",
		"This object needs "+name+" while planning. Give it a value known before apply, or remove it and set its environment variable.")
	return true
}

// requireCCP returns the CCP client or adds a diagnostic if it is not
// configured.
func requireCCP(data *providerData, diags *diag.Diagnostics) *client.Client {
	if addUnknownAttribute(data.unknownCCP, diags) {
		return nil
	}
	if data.CCP == nil {
		diags.AddError(
			"CCP credentials not configured",
			"This object needs the CCP API. Set customer_number, api_key and api_password "+
				"on the provider (or the NETCUP_* environment variables).",
		)
		return nil
	}
	return data.CCP
}

// requireSCP returns the SCP client or adds a diagnostic if it is not
// configured.
func requireSCP(data *providerData, diags *diag.Diagnostics) *scp.Client {
	if addUnknownAttribute(data.unknownSCP, diags) {
		return nil
	}
	if data.SCP == nil {
		diags.AddError(
			"SCP credentials not configured",
			"This object needs the SCP REST API. Set scp_refresh_token on the provider "+
				"(or the NETCUP_SCP_REFRESH_TOKEN environment variable).",
		)
		return nil
	}
	return data.SCP
}

// requireCloudDNS returns the CloudDNS client or adds a diagnostic if it is not
// configured.
func requireCloudDNS(data *providerData, diags *diag.Diagnostics) *clouddns.Client {
	if addUnknownAttribute(data.unknownCloudDNS, diags) {
		return nil
	}
	if data.CloudDNS == nil {
		diags.AddError(
			"CloudDNS credentials not configured",
			"This object needs the netcup API. Set clouddns_api_key on the provider "+
				"(or the NETCUP_CLOUDDNS_API_KEY environment variable).",
		)
		return nil
	}
	return data.CloudDNS
}
