// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccServersDataSource reads the full server list (SCP REST API).
func TestAccServersDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckSCP(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `data "netcup_servers" "all" {}`,
				Check:  resource.TestCheckResourceAttrSet("data.netcup_servers.all", "servers.#"),
			},
		},
	})
}

// TestAccServerDataSource reads a single server by id.
func TestAccServerDataSource(t *testing.T) {
	id := testAccLiveServerID(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccServerConfig(id),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.netcup_server.test", "id", id),
					resource.TestCheckResourceAttrSet("data.netcup_server.test", "name"),
					resource.TestCheckResourceAttrSet("data.netcup_server.test", "architecture"),
				),
			},
		},
	})
}

func testAccServerConfig(id string) string {
	return fmt.Sprintf(`
data "netcup_server" "test" {
  id = %[1]s
}
`, id)
}

// TestAccServerInterfacesDataSource reads the network interfaces of a server.
func TestAccServerInterfacesDataSource(t *testing.T) {
	id := testAccLiveServerID(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccServerInterfacesConfig(id),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.netcup_server_interfaces.test", "server_id", id),
					resource.TestCheckResourceAttrSet("data.netcup_server_interfaces.test", "interfaces.#"),
				),
			},
		},
	})
}

func testAccServerInterfacesConfig(id string) string {
	return fmt.Sprintf(`
data "netcup_server_interfaces" "test" {
  server_id = %[1]s
}
`, id)
}
