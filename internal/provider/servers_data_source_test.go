// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testAccPreCheckServer fails fast unless the SCP credentials and a concrete
// test server id are configured. NETCUP_TEST_SERVER_ID must reference a server
// that already exists in the account; these data sources only read it.
func testAccPreCheckServer(t *testing.T) {
	t.Helper()
	testAccPreCheckSCP(t)
	if os.Getenv("NETCUP_TEST_SERVER_ID") == "" {
		t.Fatal("NETCUP_TEST_SERVER_ID must be set for server data source acceptance tests")
	}
}

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
	id := os.Getenv("NETCUP_TEST_SERVER_ID")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckServer(t) },
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
	id := os.Getenv("NETCUP_TEST_SERVER_ID")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckServer(t) },
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
