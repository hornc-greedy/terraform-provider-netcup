// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccServerInfoDataSources reads every data source of one server. A step
// without error proves that each real response passes the decoding of the
// provider, the checks cover fields every server has.
func TestAccServerInfoDataSources(t *testing.T) {
	id := testAccLiveServerID(t)
	set := func(address, attribute string) resource.TestCheckFunc {
		return resource.TestCheckResourceAttrSet(address, attribute)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
data "netcup_server" "test" {
  id = %[1]s
}

data "netcup_server_interfaces" "test" {
  server_id = %[1]s
}

data "netcup_server_interface" "test" {
  server_id = %[1]s
  mac       = data.netcup_server_interfaces.test.interfaces[0].mac
}

data "netcup_server_firewall" "test" {
  server_id         = %[1]s
  mac               = data.netcup_server_interfaces.test.interfaces[0].mac
  consistency_check = true
}

data "netcup_reverse_dns" "test" {
  ip = data.netcup_server_interfaces.test.interfaces[0].ipv4_addresses[0].ip
}

data "netcup_server_disks" "test" {
  server_id = %[1]s
}

data "netcup_server_disk" "test" {
  server_id = %[1]s
  name      = data.netcup_server_disks.test.disks[0].name
}

data "netcup_server_disk_supported_drivers" "test" {
  server_id = %[1]s
}

data "netcup_server_guest_agent" "test" {
  server_id = %[1]s
}

data "netcup_server_guest_agent_status" "test" {
  server_id = %[1]s
}

data "netcup_server_image_flavours" "test" {
  server_id = %[1]s
}

data "netcup_server_iso_images" "test" {
  server_id = %[1]s
}

data "netcup_server_iso" "test" {
  server_id = %[1]s
}

data "netcup_server_logs" "test" {
  server_id = %[1]s
  limit     = 5
}

data "netcup_server_metrics_cpu" "test" {
  server_id = %[1]s
  hours     = 1
}

data "netcup_server_metrics_disk" "test" {
  server_id = %[1]s
}

data "netcup_server_metrics_network" "test" {
  server_id = %[1]s
}

data "netcup_server_metrics_network_packet" "test" {
  server_id = %[1]s
}

data "netcup_server_snapshots" "test" {
  server_id = %[1]s
}

data "netcup_server_snapshot_dryrun" "test" {
  server_id = %[1]s
  disk_name = data.netcup_server_disks.test.disks[0].name
}

data "netcup_tasks" "test" {
  server_id = %[1]s
  limit     = 5
}
`, id),
			Check: resource.ComposeAggregateTestCheckFunc(
				set("data.netcup_server.test", "name"),
				set("data.netcup_server_interface.test", "driver"),
				set("data.netcup_server_firewall.test", "consistent"),
				set("data.netcup_server_disk.test", "capacity_in_mib"),
				set("data.netcup_server_disk.test", "storage_driver"),
				set("data.netcup_server_disk_supported_drivers.test", "supported_drivers.0"),
				set("data.netcup_server_guest_agent.test", "guest_agent_available"),
				set("data.netcup_server_guest_agent_status.test", "available"),
				set("data.netcup_server_image_flavours.test", "image_flavours.0.id"),
				set("data.netcup_server_iso_images.test", "iso_images.0.id"),
				set("data.netcup_server_metrics_cpu.test", "data"),
				set("data.netcup_server_metrics_disk.test", "data"),
				set("data.netcup_server_metrics_network.test", "data"),
				set("data.netcup_server_metrics_network_packet.test", "data"),
			),
		}},
	})
}

// TestAccAccountDataSources reads every data source of the account, like
// TestAccServerInfoDataSources.
func TestAccAccountDataSources(t *testing.T) {
	set := func(address, attribute string) resource.TestCheckFunc {
		return resource.TestCheckResourceAttrSet(address, attribute)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccLiveAccount(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: `
data "netcup_user" "test" {}

data "netcup_user_logs" "test" {
  limit = 5
}

data "netcup_user_vlans" "test" {}

data "netcup_user_images" "test" {}

data "netcup_user_isos" "test" {}

data "netcup_ssh_keys" "test" {}

data "netcup_failover_ips" "test" {}

data "netcup_firewall_policies" "test" {}

data "netcup_maintenance" "test" {}

data "netcup_tasks" "test" {
  limit = 5
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				set("data.netcup_user.test", "id"),
				set("data.netcup_user.test", "username"),
				set("data.netcup_user.test", "language"),
			),
		}},
	})
}
