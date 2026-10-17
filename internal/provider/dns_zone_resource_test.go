// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// netcup_dns_zone has no delete, so the test leaves the zone with the last
// configured ttl.
func TestAccDNSZoneResource(t *testing.T) {
	domain := os.Getenv("NETCUP_TEST_DOMAIN")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckCCP(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // create
				Config: testAccDNSZoneConfig(domain, 3600),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("netcup_dns_zone.test", "id", domain),
					resource.TestCheckResourceAttr("netcup_dns_zone.test", "ttl", "3600"),
					resource.TestCheckResourceAttrSet("netcup_dns_zone.test", "serial"),
				),
			},
			{ // update in place
				Config: testAccDNSZoneConfig(domain, 86400),
				Check:  resource.TestCheckResourceAttr("netcup_dns_zone.test", "ttl", "86400"),
			},
			{ // import (domain)
				ResourceName:      "netcup_dns_zone.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccDNSZoneConfig(domain string, ttl int) string {
	return fmt.Sprintf(`
resource "netcup_dns_zone" "test" {
  domain = %[1]q
  ttl    = %[2]d
}
`, domain, ttl)
}
