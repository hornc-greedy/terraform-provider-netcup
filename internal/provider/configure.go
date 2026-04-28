// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/client"
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

// requireCCP returns the CCP client or adds a diagnostic if it is not
// configured.
func requireCCP(data *providerData, diags *diag.Diagnostics) *client.Client {
	if data.CCP == nil {
		diags.AddError(
			"CCP credentials not configured",
			"This resource needs the CCP API. Set customer_number, api_key and api_password "+
				"on the provider (or the NETCUP_* environment variables).",
		)
		return nil
	}
	return data.CCP
}

// requireSCP returns the SCP client or adds a diagnostic if it is not
// configured.
func requireSCP(data *providerData, diags *diag.Diagnostics) *scp.Client {
	if data.SCP == nil {
		diags.AddError(
			"SCP credentials not configured",
			"This resource needs the SCP REST API. Set scp_refresh_token on the provider "+
				"(or the NETCUP_SCP_REFRESH_TOKEN environment variable).",
		)
		return nil
	}
	return data.SCP
}
