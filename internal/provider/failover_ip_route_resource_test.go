// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// The test routes the IPv4 failover IP NETCUP_TEST_FAILOVER_IP_ID to
// NETCUP_TEST_SERVER_ID. netcup has no unroute, so the IP stays routed there.
func TestAccFailoverIPRouteResource(t *testing.T) {
	failoverID := os.Getenv("NETCUP_TEST_FAILOVER_IP_ID")
	serverID := os.Getenv("NETCUP_TEST_SERVER_ID")
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckEnv(t, "NETCUP_TEST_FAILOVER_IP_ID", "NETCUP_TEST_SERVER_ID")
			testAccPreCheckSCP(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // create
				Config: testAccFailoverIPRouteConfig(failoverID, serverID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("netcup_failover_ip_route.test", "id", "v4/"+failoverID),
					resource.TestCheckResourceAttr("netcup_failover_ip_route.test", "server_id", serverID),
					resource.TestCheckResourceAttr("netcup_failover_ip_route.test", "ipv6", "false"),
				),
			},
			{ // import (composite id v4/<failover_ip_id>)
				ResourceName:      "netcup_failover_ip_route.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccFailoverIPRouteConfig(failoverID, serverID string) string {
	return fmt.Sprintf(`
resource "netcup_failover_ip_route" "test" {
  failover_ip_id = %[1]s
  server_id      = %[2]s
}
`, failoverID, serverID)
}
