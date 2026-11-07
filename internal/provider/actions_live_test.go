// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// testAccActionCase runs actions through Terraform itself, because the testing
// library has no support for actions.
func testAccActionCase(steps ...resource.TestStep) resource.TestCase {
	return resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_14_0)},
		ProtoV6ProviderFactories: testAccEchoProviderFactories,
		Steps:                    steps,
	}
}

// testAccActionTrigger runs the actions in their order once terraform_data is
// created. A new run replaces it and runs them again.
func testAccActionTrigger(run int, actions ...string) string {
	return fmt.Sprintf(`
resource "terraform_data" "trigger" {
  triggers_replace = %d

  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [%s]
    }
  }
}
`, run, strings.Join(actions, ", "))
}

// testAccServerActions defines the power actions of the server.
func testAccServerActions(id string) string {
	return fmt.Sprintf(`
action "netcup_server_state" "off" {
  config {
    server_id    = %[1]s
    state        = "OFF"
    state_option = "POWEROFF"
  }
}

action "netcup_server_state" "on" {
  config {
    server_id = %[1]s
    state     = "ON"
  }
}
`, id)
}

// The test shuts NETCUP_TEST_SERVER_ID down and starts it again.
func TestAccServerStateAction(t *testing.T) {
	id := testAccLiveServerID(t)
	testAccKeepServerRunning(t, id)
	resource.Test(t, testAccActionCase(
		resource.TestStep{
			Config: testAccServerActions(id) + testAccActionTrigger(1, "action.netcup_server_state.off"),
			Check:  testAccCheckServerState(id, "SHUTOFF"),
		},
		resource.TestStep{
			Config: testAccServerActions(id) + testAccActionTrigger(2, "action.netcup_server_state.on"),
			Check:  testAccCheckServerState(id, "RUNNING"),
		},
	))
}

// The test applies the firewall of the first interface again and restores its
// copied policies.
func TestAccServerFirewallActions(t *testing.T) {
	id := testAccLiveServerID(t)
	resource.Test(t, testAccActionCase(resource.TestStep{
		Config: fmt.Sprintf(`
data "netcup_server_interfaces" "test" {
  server_id = %[1]s
}

action "netcup_server_firewall_reapply" "test" {
  config {
    server_id = %[1]s
    mac       = data.netcup_server_interfaces.test.interfaces[0].mac
  }
}

action "netcup_server_firewall_restore_copied_policies" "test" {
  config {
    server_id = %[1]s
    mac       = data.netcup_server_interfaces.test.interfaces[0].mac
  }
}
`, id) + testAccActionTrigger(1, "action.netcup_server_firewall_reapply.test", "action.netcup_server_firewall_restore_copied_policies.test"),
	}))
}

// The test asks netcup to cancel the last finished task of the server.
func TestAccTaskCancelAction(t *testing.T) {
	id := testAccLiveServerID(t)
	resource.Test(t, testAccActionCase(resource.TestStep{
		Config: fmt.Sprintf(`
data "netcup_tasks" "test" {
  server_id = %s
  state     = "FINISHED"
  limit     = 1
}

action "netcup_task_cancel" "test" {
  config {
    uuid = data.netcup_tasks.test.tasks[0].uuid
  }
}
`, id) + testAccActionTrigger(1, "action.netcup_task_cancel.test"),
	}))
}

// The test optimizes the storage of the server, which netcup does while it is
// shut down.
func TestAccServerStorageOptimizationAction(t *testing.T) {
	id := testAccLiveServerID(t)
	testAccKeepServerRunning(t, id)
	resource.Test(t, testAccActionCase(
		resource.TestStep{
			Config: testAccServerActions(id) + fmt.Sprintf(`
action "netcup_server_storage_optimization" "test" {
  config {
    server_id                = %s
    disks                    = ["vda"]
    start_after_optimization = true
  }
}
`, id) + testAccActionTrigger(1, "action.netcup_server_state.off", "action.netcup_server_storage_optimization.test"),
		},
	))
}

// The test takes an offline snapshot and reverts the server to it. With
// NETCUP_TEST_SNAPSHOT_EXPORT=1 it also exports the snapshot, which uses the one
// export netcup includes per server, and fetches its download link.
func TestAccServerSnapshotActions(t *testing.T) {
	id := testAccLiveServerID(t)
	testAccKeepServerRunning(t, id)
	snapshot := fmt.Sprintf(`
resource "netcup_server_snapshot" "test" {
  server_id = %[1]s
  name      = %[2]q
  disk_name = "vda"
}

action "netcup_server_snapshot_revert" "test" {
  config {
    server_id = %[1]s
    name      = netcup_server_snapshot.test.name
  }
}

action "netcup_server_snapshot_export" "test" {
  config {
    server_id = %[1]s
    name      = netcup_server_snapshot.test.name
  }
}
`, id, acctest.RandomWithPrefix("tf-acc")) + testAccServerActions(id)
	steps := []resource.TestStep{{
		Config: snapshot + testAccActionTrigger(1, "action.netcup_server_state.off", "action.netcup_server_snapshot_revert.test", "action.netcup_server_state.on"),
		Check:  testAccCheckServerState(id, "RUNNING"),
	}}
	if os.Getenv("NETCUP_TEST_SNAPSHOT_EXPORT") == "1" {
		exported := snapshot + testAccActionTrigger(2, "action.netcup_server_state.off", "action.netcup_server_snapshot_export.test", "action.netcup_server_state.on")
		steps = append(steps,
			resource.TestStep{
				Config: exported,
				Check:  testAccCheckServerState(id, "RUNNING"),
			},
			resource.TestStep{
				Config: exported + fmt.Sprintf(`
data "netcup_server_snapshot" "test" {
  server_id = %s
  name      = netcup_server_snapshot.test.name
}

ephemeral "netcup_server_snapshot_download" "test" {
  server_id = data.netcup_server_snapshot.test.server_id
  name      = data.netcup_server_snapshot.test.name
}

provider "echo" {
  data = ephemeral.netcup_server_snapshot_download.test
}

resource "echo" "test" {}
`, id),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.netcup_server_snapshot.test", "exported", "true"),
					resource.TestCheckResourceAttrSet("data.netcup_server_snapshot.test", "exported_size_in_kib"),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("presigned_url"), knownvalue.NotNull()),
				},
			},
		)
	}
	testCase := testAccActionCase(steps...)
	testCase.CheckDestroy = testAccCheckServerSnapshotDestroy
	resource.Test(t, testCase)
}

// The test formats the disk of the server and installs the first Debian flavour
// of its boot mode on it. All data of the server is lost.
func TestAccServerImageSetupActions(t *testing.T) {
	id := testAccLiveServerID(t)
	testAccKeepServerRunning(t, id)
	config := testAccServerActions(id) + fmt.Sprintf(`
data "netcup_server" "test" {
  id = %[1]s
}

data "netcup_server_image_flavours" "test" {
  server_id = %[1]s
}

locals {
  boot_mode = data.netcup_server.test.server_live_info.uefi ? "UEFI" : "BIOS"
}

action "netcup_server_disk_format" "test" {
  config {
    server_id = %[1]s
    disk_name = "vda"
  }
}

action "netcup_server_image_setup" "test" {
  config {
    server_id                     = %[1]s
    image_flavour_id              = [for flavour in data.netcup_server_image_flavours.test.image_flavours : flavour.id if startswith(flavour.image.name, "Debian ") && strcontains(flavour.image.name, local.boot_mode)][0]
    disk_name                     = "vda"
    root_partition_full_disk_size = true
    hostname                      = "tf-acc-test"
    ssh_password_authentication   = false
  }
}
`, id)
	resource.Test(t, testAccActionCase(resource.TestStep{
		Config: config + testAccActionTrigger(1, "action.netcup_server_state.off", "action.netcup_server_disk_format.test", "action.netcup_server_image_setup.test"),
	}))
}

// The test uploads the image NETCUP_TEST_USER_IMAGE and installs it on the
// server. A file larger than 64 MiB is uploaded in several parts.
func TestAccServerUserImageSetupAction(t *testing.T) {
	image := os.Getenv("NETCUP_TEST_USER_IMAGE")
	if image == "" {
		t.Skip("NETCUP_TEST_USER_IMAGE not set")
	}
	id := testAccLiveServerID(t)
	testAccKeepServerRunning(t, id)
	config := testAccServerActions(id) + fmt.Sprintf(`
resource "netcup_user_image" "test" {
  key         = "tf-acc-setup.qcow2"
  source      = %[2]q
  source_hash = filesha256(%[2]q)
}

action "netcup_server_user_image_setup" "test" {
  config {
    server_id       = %[1]s
    user_image_name = netcup_user_image.test.key
    disk_name       = "vda"
  }
}
`, id, image)
	resource.Test(t, testAccActionCase(resource.TestStep{
		Config: config + testAccActionTrigger(1, "action.netcup_server_state.off", "action.netcup_server_user_image_setup.test"),
	}))
}
