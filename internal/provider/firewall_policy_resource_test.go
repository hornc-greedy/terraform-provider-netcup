// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

func TestAccFirewallPolicyResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckSCP(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckFirewallPolicyDestroy,
		Steps: []resource.TestStep{
			{ // create
				Config: testAccFirewallPolicyConfig("tf-acc-test", "web ingress"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("netcup_firewall_policy.test", "name", "tf-acc-test"),
					resource.TestCheckResourceAttr("netcup_firewall_policy.test", "rule.#", "1"),
					resource.TestCheckResourceAttr("netcup_firewall_policy.test", "rule.0.action", "ACCEPT"),
					resource.TestCheckResourceAttrSet("netcup_firewall_policy.test", "id"),
				),
			},
			{ // update the description
				Config: testAccFirewallPolicyConfig("tf-acc-test", "web ingress updated"),
				Check:  resource.TestCheckResourceAttr("netcup_firewall_policy.test", "rule.0.description", "web ingress updated"),
			},
			{ // import (passthrough numeric id)
				ResourceName:      "netcup_firewall_policy.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// resource.Test fails on a non-empty plan after each step, so this also proves
// that the unsorted addresses and the empty values stay stable.
func TestAccFirewallPolicyKeepsConfiguredValues(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckSCP(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckFirewallPolicyDestroy,
		Steps: []resource.TestStep{
			{ // unsorted addresses, empty values, index access
				Config: `
resource "netcup_firewall_policy" "test" {
  name        = "tf-acc-test-values"
  description = "cleared in the next step"

  rule {
    action       = "ACCEPT"
    protocol     = "TCP"
    direction    = "INGRESS"
    sources      = ["203.0.113.20/32", "192.0.2.10/32", "198.51.100.5/32"]
    destinations = []
    description  = ""
  }
}

output "first_source" {
  value = netcup_firewall_policy.test.rule[0].sources[0]
}
`,
				Check: resource.TestCheckOutput("first_source", "203.0.113.20/32"),
			},
			{ // cleared description and rules
				Config: `
resource "netcup_firewall_policy" "test" {
  name = "tf-acc-test-values"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("netcup_firewall_policy.test", "description", ""),
					resource.TestCheckResourceAttr("netcup_firewall_policy.test", "rule.#", "0"),
				),
			},
		},
	})
}

func testAccCheckFirewallPolicyDestroy(s *terraform.State) error {
	c := testAccSCPClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "netcup_firewall_policy" {
			continue
		}
		id, err := strconv.Atoi(rs.Primary.ID)
		if err != nil {
			return err
		}
		_, err = c.GetFirewallPolicy(context.Background(), id)
		if err == nil {
			return fmt.Errorf("firewall policy %d still exists", id)
		}
		if !scp.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func testAccFirewallPolicyConfig(name, ruleDesc string) string {
	return fmt.Sprintf(`
resource "netcup_firewall_policy" "test" {
  name = %[1]q

  rule {
    action      = "ACCEPT"
    protocol    = "TCP"
    direction   = "INGRESS"
    sources     = ["0.0.0.0/0"]
    description = %[2]q
  }
}
`, name, ruleDesc)
}
