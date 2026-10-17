// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// Destroying the resource deletes the PTR of NETCUP_TEST_RDNS_IP, so the IP must
// be one whose reverse DNS may be reset.
func TestAccReverseDNSResource(t *testing.T) {
	ip := os.Getenv("NETCUP_TEST_RDNS_IP")
	domain := os.Getenv("NETCUP_TEST_DOMAIN")
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckEnv(t, "NETCUP_TEST_RDNS_IP", "NETCUP_TEST_DOMAIN")
			testAccPreCheckSCP(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckReverseDNSDestroy("tf-acc-test-2." + domain),
		Steps: []resource.TestStep{
			{ // create
				Config: testAccReverseDNSConfig(ip, "tf-acc-test-1."+domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("netcup_reverse_dns.test", "id", ip),
					resource.TestCheckResourceAttr("netcup_reverse_dns.test", "hostname", "tf-acc-test-1."+domain),
				),
			},
			{ // update in place
				Config: testAccReverseDNSConfig(ip, "tf-acc-test-2."+domain),
				Check:  resource.TestCheckResourceAttr("netcup_reverse_dns.test", "hostname", "tf-acc-test-2."+domain),
			},
			{ // import (ip)
				ResourceName:      "netcup_reverse_dns.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckReverseDNSDestroy(hostname string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccSCPClient()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "netcup_reverse_dns" {
				continue
			}
			got, err := c.GetReverseDNS(context.Background(), rs.Primary.ID)
			if err != nil {
				return err
			}
			if got == hostname {
				return fmt.Errorf("reverse dns of %s still points to %s", rs.Primary.ID, hostname)
			}
		}
		return nil
	}
}

func testAccReverseDNSConfig(ip, hostname string) string {
	return fmt.Sprintf(`
resource "netcup_reverse_dns" "test" {
  ip       = %[1]q
  hostname = %[2]q
}
`, ip, hostname)
}
