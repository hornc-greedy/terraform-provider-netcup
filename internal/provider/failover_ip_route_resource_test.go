// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// The test routes the IPv4 failover IP NETCUP_TEST_FAILOVER_IP_ID to
// NETCUP_TEST_SERVER_ID. netcup has no unroute, so the IP stays routed there.
func TestAccFailoverIPRouteResource(t *testing.T) {
	serverID := testAccLiveServerID(t)
	failoverID := os.Getenv("NETCUP_TEST_FAILOVER_IP_ID")
	if failoverID == "" {
		t.Skip("NETCUP_TEST_FAILOVER_IP_ID not set")
	}
	testAccRequireFailoverIdle(t, failoverID, serverID)
	resource.Test(t, resource.TestCase{
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

// testAccRequireFailoverIdle fails unless the IPv4 failover IP is unrouted or
// already routed to the live server, so no other server loses it.
func testAccRequireFailoverIdle(t *testing.T, failoverID, serverID string) {
	t.Helper()
	id, err := strconv.Atoi(failoverID)
	if err != nil {
		t.Fatalf("NETCUP_TEST_FAILOVER_IP_ID: %v", err)
	}
	found, server, err := testAccSCPClient().FailoverRoute(context.Background(), id, false)
	if err != nil {
		t.Fatalf("read failover IP %s: %v", failoverID, err)
	}
	if !found {
		t.Fatalf("failover IP %s does not exist", failoverID)
	}
	if server != nil && strconv.Itoa(server.ID) != serverID {
		t.Fatalf("failover IP %s is routed to server %d", failoverID, server.ID)
	}
}
