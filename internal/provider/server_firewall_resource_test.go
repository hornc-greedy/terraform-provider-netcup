// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// The test assigns policies to the first interface of NETCUP_TEST_SERVER_ID with
// the firewall inactive. Destroying clears the assignment, so the server must
// not depend on its current firewall.
func TestAccServerFirewallResource(t *testing.T) {
	serverID := os.Getenv("NETCUP_TEST_SERVER_ID")
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckEnv(t, "NETCUP_TEST_SERVER_ID")
			testAccPreCheckSCP(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckServerFirewallDestroy,
		Steps: []resource.TestStep{
			{ // create
				Config: testAccServerFirewallConfig(serverID, "[netcup_firewall_policy.a.id]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("netcup_server_firewall.test", "policy_ids.#", "1"),
					resource.TestCheckResourceAttrPair("netcup_server_firewall.test", "policy_ids.0", "netcup_firewall_policy.a", "id"),
					resource.TestCheckResourceAttr("netcup_server_firewall.test", "active", "false"),
				),
			},
			{ // update in place
				Config: testAccServerFirewallConfig(serverID, "[netcup_firewall_policy.a.id, netcup_firewall_policy.b.id]"),
				Check:  resource.TestCheckResourceAttr("netcup_server_firewall.test", "policy_ids.#", "2"),
			},
			{ // import (composite id <server_id>/<mac>)
				ResourceName:            "netcup_server_firewall.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func testAccCheckServerFirewallDestroy(s *terraform.State) error {
	c := testAccSCPClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "netcup_server_firewall" {
			continue
		}
		serverID, err := strconv.Atoi(rs.Primary.Attributes["server_id"])
		if err != nil {
			return err
		}
		fw, err := c.GetServerFirewall(context.Background(), serverID, rs.Primary.Attributes["mac"])
		if err != nil {
			return err
		}
		if len(fw.UserPolicies) > 0 || fw.Active {
			return fmt.Errorf("firewall of %s is still assigned", rs.Primary.ID)
		}
	}
	return nil
}

func testAccServerFirewallConfig(serverID, policyIDs string) string {
	return fmt.Sprintf(`
data "netcup_server_interfaces" "test" {
  server_id = %[1]s
}

resource "netcup_firewall_policy" "a" {
  name = "tf-acc-test-a"

  rule {
    action    = "ACCEPT"
    protocol  = "TCP"
    direction = "INGRESS"
    sources   = ["0.0.0.0/0"]
  }
}

resource "netcup_firewall_policy" "b" {
  name = "tf-acc-test-b"

  rule {
    action    = "ACCEPT"
    protocol  = "UDP"
    direction = "INGRESS"
    sources   = ["0.0.0.0/0"]
  }
}

resource "netcup_server_firewall" "test" {
  server_id  = %[1]s
  mac        = data.netcup_server_interfaces.test.interfaces[0].mac
  policy_ids = %[2]s
  active     = false
}
`, serverID, policyIDs)
}
