// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// The test creates a TXT record, finds it through the data source and deletes it.
func TestAccDNSRecordsDataSource(t *testing.T) {
	domain := testAccPreCheckCCP(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
resource "netcup_dns_record" "test" {
  domain      = %[1]q
  hostname    = "tf-acc-records"
  type        = "TXT"
  destination = "terraform acceptance test"
}

data "netcup_dns_records" "test" {
  domain     = %[1]q
  depends_on = [netcup_dns_record.test]
}
`, domain),
			Check: resource.TestCheckTypeSetElemNestedAttrs("data.netcup_dns_records.test", "records.*", map[string]string{
				"hostname":    "tf-acc-records",
				"type":        "TXT",
				"destination": "terraform acceptance test",
			}),
		}},
	})
}
