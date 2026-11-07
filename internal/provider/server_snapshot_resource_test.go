// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccServerSnapshotResource(t *testing.T) {
	serverID := testAccLiveServerID(t)
	name := acctest.RandomWithPrefix("tf-acc")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckServerSnapshotDestroy,
		Steps: []resource.TestStep{
			{ // create
				Config: testAccServerSnapshotConfig(serverID, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("netcup_server_snapshot.test", "id", serverID+"/"+name),
					resource.TestCheckResourceAttr("netcup_server_snapshot.test", "online", "false"),
					resource.TestCheckResourceAttr("netcup_server_snapshot.test", "disk_name", "vda"),
					resource.TestCheckResourceAttrSet("netcup_server_snapshot.test", "state"),
				),
			},
			{ // lookup of the snapshot and of a finished task
				Config: testAccServerSnapshotConfig(serverID, name) + testAccServerSnapshotLookupConfig(serverID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.netcup_server_snapshot.test", "uuid"),
					resource.TestCheckResourceAttrSet("data.netcup_server_snapshot.test", "creation_time"),
					resource.TestCheckResourceAttr("data.netcup_server_snapshot.test", "disks.0", "vda"),
					resource.TestCheckResourceAttr("data.netcup_server_snapshot.test", "online", "false"),
					resource.TestCheckTypeSetElemNestedAttrs("data.netcup_server_snapshots.test", "snapshots.*", map[string]string{"name": name}),
					resource.TestCheckResourceAttr("data.netcup_task.test", "state", "FINISHED"),
					resource.TestCheckResourceAttrSet("data.netcup_task.test", "name"),
				),
			},
			{ // import (composite id <server_id>/<name>)
				ResourceName:            "netcup_server_snapshot.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func testAccCheckServerSnapshotDestroy(s *terraform.State) error {
	c := testAccSCPClient()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "netcup_server_snapshot" {
			continue
		}
		serverID, err := strconv.Atoi(rs.Primary.Attributes["server_id"])
		if err != nil {
			return err
		}
		snap, err := c.GetSnapshot(context.Background(), serverID, rs.Primary.Attributes["name"])
		if err != nil {
			return err
		}
		if snap != nil {
			return fmt.Errorf("snapshot %s still exists", rs.Primary.ID)
		}
	}
	return nil
}

func testAccServerSnapshotConfig(serverID, name string) string {
	return fmt.Sprintf(`
resource "netcup_server_snapshot" "test" {
  server_id   = %[1]s
  name        = %[2]q
  description = "terraform acceptance test"
  disk_name   = "vda"
}
`, serverID, name)
}

func testAccServerSnapshotLookupConfig(serverID string) string {
	return fmt.Sprintf(`
data "netcup_server_snapshot" "test" {
  server_id = %[1]s
  name      = netcup_server_snapshot.test.name
}

data "netcup_server_snapshots" "test" {
  server_id  = %[1]s
  depends_on = [netcup_server_snapshot.test]
}

data "netcup_tasks" "test" {
  server_id = %[1]s
  state     = "FINISHED"
  limit     = 1
}

data "netcup_task" "test" {
  uuid = data.netcup_tasks.test.tasks[0].uuid
}
`, serverID)
}
