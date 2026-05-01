// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"
	"testing"

	fwprov "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories wires the in-process provider for acceptance
// tests. Credentials are taken from the NETCUP_* environment variables.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"netcup": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheckCCP fails fast unless the CCP (DNS) credentials and a
// disposable test domain are configured.
func testAccPreCheckCCP(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"NETCUP_CUSTOMER_NUMBER",
		"NETCUP_API_KEY",
		"NETCUP_API_PASSWORD",
		"NETCUP_TEST_DOMAIN",
	} {
		if os.Getenv(k) == "" {
			t.Fatalf("%s must be set for CCP acceptance tests", k)
		}
	}
}

// testAccPreCheckSCP fails fast unless the SCP refresh token is configured.
func testAccPreCheckSCP(t *testing.T) {
	t.Helper()
	if os.Getenv("NETCUP_SCP_REFRESH_TOKEN") == "" {
		t.Fatal("NETCUP_SCP_REFRESH_TOKEN must be set for SCP acceptance tests")
	}
}

// TestProvider is a fast smoke test (runs without TF_ACC): the provider
// instantiates and reports the expected type name.
func TestProvider(t *testing.T) {
	p := New("test")()
	resp := &fwprov.MetadataResponse{}
	p.Metadata(context.Background(), fwprov.MetadataRequest{}, resp)
	if resp.TypeName != "netcup" {
		t.Fatalf("unexpected provider type name: %q", resp.TypeName)
	}
}
