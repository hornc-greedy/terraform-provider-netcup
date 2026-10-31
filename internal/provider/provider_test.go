// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	fwprov "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

// testAccProtoV6ProviderFactories provides the in-process provider to a test.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"netcup": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheckCCP skips the test unless credentials and a domain explicitly
// meant for live tests are set, and returns the domain.
func testAccPreCheckCCP(t *testing.T) string {
	t.Helper()
	for live, normal := range map[string]string{
		"NETCUP_TEST_CUSTOMER_NUMBER": "NETCUP_CUSTOMER_NUMBER",
		"NETCUP_TEST_API_KEY":         "NETCUP_API_KEY",
		"NETCUP_TEST_API_PASSWORD":    "NETCUP_API_PASSWORD",
	} {
		value := os.Getenv(live)
		if value == "" {
			t.Skipf("%s not set", live)
		}
		t.Setenv(normal, value)
	}
	domain := os.Getenv("NETCUP_TEST_DOMAIN")
	if domain == "" {
		t.Skip("NETCUP_TEST_DOMAIN not set")
	}
	return domain
}

// liveServerNickname is the prefix a server's nickname must have before a live
// test may change it.
const liveServerNickname = "netcup-provider-live-test"

// testAccPreCheckSCP skips the test unless a token explicitly meant for live tests
// is set, so that a token of the normal environment is never used.
func testAccPreCheckSCP(t *testing.T) {
	t.Helper()
	token := os.Getenv("NETCUP_TEST_SCP_REFRESH_TOKEN")
	if token == "" {
		t.Skip("NETCUP_TEST_SCP_REFRESH_TOKEN not set")
	}
	t.Setenv("NETCUP_SCP_REFRESH_TOKEN", token)
	t.Setenv("NETCUP_SCP_BASE_URL", "")
}

// testAccPreCheckCloudDNS skips the test unless a key explicitly meant for live
// tests is set.
func testAccPreCheckCloudDNS(t *testing.T) {
	t.Helper()
	key := os.Getenv("NETCUP_TEST_CLOUDDNS_API_KEY")
	if key == "" {
		t.Skip("NETCUP_TEST_CLOUDDNS_API_KEY not set")
	}
	t.Setenv("NETCUP_CLOUDDNS_API_KEY", key)
	t.Setenv("NETCUP_CLOUDDNS_BASE_URL", "")
}

// testAccLiveAccount skips a live test without TF_ACC or the token and fails it
// unless every server of the account has the nickname prefix liveServerNickname.
func testAccLiveAccount(t *testing.T) []scp.Server {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC not set")
	}
	testAccPreCheckSCP(t)
	servers, err := testAccSCPClient().ListServers(context.Background())
	if err != nil {
		t.Fatalf("list servers: %v", err)
	}
	for _, server := range servers {
		if !strings.HasPrefix(server.Nickname, liveServerNickname) {
			t.Fatalf("server %d has nickname %q, a live test needs the prefix %q on every server of the account", server.ID, server.Nickname, liveServerNickname)
		}
	}
	return servers
}

// testAccLiveServerID returns the server a live test may change. It checks the
// account like testAccLiveAccount and skips the test unless a server is set.
func testAccLiveServerID(t *testing.T) string {
	t.Helper()
	servers := testAccLiveAccount(t)
	id := os.Getenv("NETCUP_TEST_SERVER_ID")
	if id == "" {
		t.Skip("NETCUP_TEST_SERVER_ID not set")
	}
	serverID, err := strconv.Atoi(id)
	if err != nil {
		t.Fatalf("NETCUP_TEST_SERVER_ID: %v", err)
	}
	if !slices.ContainsFunc(servers, func(server scp.Server) bool { return server.ID == serverID }) {
		t.Fatalf("server %s is not in the account of the token", id)
	}
	return id
}

// testAccSCPClient returns an SCP client for checks of live tests.
func testAccSCPClient() *scp.Client {
	return scp.New(os.Getenv("NETCUP_SCP_REFRESH_TOKEN"))
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
