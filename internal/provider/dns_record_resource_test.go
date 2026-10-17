// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/client"
)

func TestAccDNSRecordResource(t *testing.T) {
	domain := os.Getenv("NETCUP_TEST_DOMAIN")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckCCP(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSRecordDestroy(domain),
		Steps: []resource.TestStep{
			{ // create
				Config: testAccDNSRecordConfig(domain, "203.0.113.10"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("netcup_dns_record.test", "hostname", "tf-acc-test"),
					resource.TestCheckResourceAttr("netcup_dns_record.test", "type", "A"),
					resource.TestCheckResourceAttr("netcup_dns_record.test", "destination", "203.0.113.10"),
					resource.TestCheckResourceAttrSet("netcup_dns_record.test", "id"),
				),
			},
			{ // update in place
				Config: testAccDNSRecordConfig(domain, "203.0.113.20"),
				Check:  resource.TestCheckResourceAttr("netcup_dns_record.test", "destination", "203.0.113.20"),
			},
			{ // import (composite id <domain>:<record_id>)
				ResourceName:      "netcup_dns_record.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["netcup_dns_record.test"]
					if !ok {
						return "", fmt.Errorf("resource netcup_dns_record.test not found in state")
					}
					return fmt.Sprintf("%s:%s", domain, rs.Primary.Attributes["id"]), nil
				},
			},
		},
	})
}

func testAccCheckDNSRecordDestroy(domain string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := client.New(os.Getenv("NETCUP_CUSTOMER_NUMBER"), os.Getenv("NETCUP_API_KEY"), os.Getenv("NETCUP_API_PASSWORD"))
		recs, err := c.InfoDNSRecords(context.Background(), domain)
		if err != nil {
			return err
		}
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "netcup_dns_record" {
				continue
			}
			for _, rec := range recs {
				if rec.ID == rs.Primary.ID {
					return fmt.Errorf("dns record %s still exists", rec.ID)
				}
			}
		}
		return nil
	}
}

func testAccDNSRecordConfig(domain, dest string) string {
	return fmt.Sprintf(`
resource "netcup_dns_record" "test" {
  domain      = %[1]q
  hostname    = "tf-acc-test"
  type        = "A"
  destination = %[2]q
}
`, domain, dest)
}
