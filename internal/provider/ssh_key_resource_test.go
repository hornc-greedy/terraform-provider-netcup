// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// A syntactically valid ed25519 public key used only for acceptance tests.
const testAccSSHPubKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAINb8r6kZ8h0xQ4Yk2m9Jw1pL3vX5tZ7cR0aF2dG4hKp tf-acc-test"

func TestAccSSHKeyResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckSCP(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // create
				Config: testAccSSHKeyConfig("tf-acc-test", testAccSSHPubKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("netcup_ssh_key.test", "name", "tf-acc-test"),
					resource.TestCheckResourceAttrSet("netcup_ssh_key.test", "id"),
				),
			},
			{ // import (passthrough numeric id)
				ResourceName:      "netcup_ssh_key.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccSSHKeyConfig(name, key string) string {
	return fmt.Sprintf(`
resource "netcup_ssh_key" "test" {
  name = %[1]q
  key  = %[2]q
}
`, name, key)
}

func TestAccSSHKeysDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckSCP(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "netcup_ssh_keys" "all" {}`,
				Check:  resource.TestCheckResourceAttrSet("data.netcup_ssh_keys.all", "keys.#"),
			},
		},
	})
}
