// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccServerSnapshotResource(t *testing.T) {
	serverID := os.Getenv("NETCUP_TEST_SERVER_ID")
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckEnv(t, "NETCUP_TEST_SERVER_ID")
			testAccPreCheckSCP(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckServerSnapshotDestroy,
		Steps: []resource.TestStep{
			{ // create
				Config: testAccServerSnapshotConfig(serverID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("netcup_server_snapshot.test", "id", serverID+"/tf-acc-test"),
					resource.TestCheckResourceAttr("netcup_server_snapshot.test", "online", "false"),
					resource.TestCheckResourceAttr("netcup_server_snapshot.test", "disk_name", "vda"),
					resource.TestCheckResourceAttrSet("netcup_server_snapshot.test", "state"),
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

func testAccServerSnapshotConfig(serverID string) string {
	return fmt.Sprintf(`
resource "netcup_server_snapshot" "test" {
  server_id   = %[1]s
  name        = "tf-acc-test"
  description = "terraform acceptance test"
  disk_name   = "vda"
}
`, serverID)
}
