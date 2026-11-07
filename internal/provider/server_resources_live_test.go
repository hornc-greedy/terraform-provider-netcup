// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// testAccServerInt returns the id testAccLiveServerID checked as a number.
func testAccServerInt(id string) int {
	serverID, _ := strconv.Atoi(id)
	return serverID
}

// testAccKeepServerRunning starts the server after the test unless it runs, so
// that a failed step leaves it as every live test expects it.
func testAccKeepServerRunning(t *testing.T, id string) {
	t.Cleanup(func() {
		ctx := context.Background()
		server, err := testAccSCPClient().GetServerLive(ctx, testAccServerInt(id))
		if err == nil && server.LiveInfo.State != "RUNNING" {
			err = testAccSCPClient().PatchServerState(ctx, testAccServerInt(id), "ON", "")
		}
		if err != nil {
			t.Errorf("start server %s: %v", id, err)
		}
	})
}

// testAccCheckServerState checks the power state of the server.
func testAccCheckServerState(id, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		server, err := testAccSCPClient().GetServerLive(context.Background(), testAccServerInt(id))
		if err != nil {
			return err
		}
		if server.LiveInfo.State != want {
			return fmt.Errorf("server is %s, want %s", server.LiveInfo.State, want)
		}
		return nil
	}
}

// The test activates the rescue system of NETCUP_TEST_SERVER_ID, reads its
// password and deactivates it again. The server runs after each step.
func TestAccServerRescueSystemResource(t *testing.T) {
	id := testAccLiveServerID(t)
	testAccKeepServerRunning(t, id)
	serverID := testAccServerInt(id)
	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_10_0)},
		ProtoV6ProviderFactories: testAccEchoProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			status, err := testAccSCPClient().GetRescueSystem(context.Background(), serverID)
			if err != nil {
				return err
			}
			if status.Active != nil && *status.Active {
				return fmt.Errorf("rescue system still active")
			}
			return testAccCheckServerState(id, "RUNNING")(nil)
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "netcup_server_rescue_system" "test" {
  server_id = %s
}

ephemeral "netcup_server_rescue_system" "test" {
  server_id  = netcup_server_rescue_system.test.server_id
  depends_on = [netcup_server_rescue_system.test]
}

provider "echo" {
  data = ephemeral.netcup_server_rescue_system.test
}

resource "echo" "test" {}
`, id),
				Check: testAccCheckServerState(id, "RUNNING"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("active"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("password"), knownvalue.NotNull()),
				},
			},
			{
				ResourceName:            "netcup_server_rescue_system.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

// The test attaches the first ISO netcup offers to NETCUP_TEST_SERVER_ID without
// changing the boot device and detaches it again.
func TestAccServerIsoResource(t *testing.T) {
	id := testAccLiveServerID(t)
	serverID := testAccServerInt(id)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			iso, err := testAccSCPClient().GetIso(context.Background(), serverID)
			if err != nil {
				return err
			}
			if iso.IsoAttached {
				return fmt.Errorf("ISO %s still attached", iso.Iso)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
data "netcup_server_iso_images" "test" {
  server_id = %[1]s
}

resource "netcup_server_iso" "test" {
  server_id                   = %[1]s
  iso_id                      = data.netcup_server_iso_images.test.iso_images[0].id
  change_boot_device_to_cdrom = false
}

data "netcup_server_iso" "test" {
  server_id  = %[1]s
  depends_on = [netcup_server_iso.test]
}
`, id),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("netcup_server_iso.test", "iso"),
					resource.TestCheckResourceAttr("data.netcup_server_iso.test", "iso_attached", "true"),
					resource.TestCheckResourceAttrPair("data.netcup_server_iso.test", "iso", "netcup_server_iso.test", "iso"),
				),
			},
			{
				ResourceName:            "netcup_server_iso.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts", "iso_id", "change_boot_device_to_cdrom"},
			},
		},
	})
}

// The test switches the disks of NETCUP_TEST_SERVER_ID to another supported
// driver and back, after a failed step as well.
func TestAccServerDiskDriverResource(t *testing.T) {
	id := testAccLiveServerID(t)
	serverID := testAccServerInt(id)
	ctx := context.Background()
	disks, err := testAccSCPClient().ListDisks(ctx, serverID)
	if err != nil {
		t.Fatal(err)
	}
	drivers, err := testAccSCPClient().ListSupportedDrivers(ctx, serverID)
	if err != nil {
		t.Fatal(err)
	}
	current := disks[0].StorageDriver
	other := slices.IndexFunc(drivers, func(driver string) bool { return driver != current })
	if other < 0 {
		t.Skipf("server supports only %v", drivers)
	}
	t.Cleanup(func() {
		disks, err := testAccSCPClient().ListDisks(ctx, serverID)
		if err == nil && disks[0].StorageDriver != current {
			err = testAccSCPClient().PatchDiskDriver(ctx, serverID, current)
		}
		if err != nil {
			t.Errorf("restore driver %s: %v", current, err)
		}
	})
	config := func(driver string) string {
		return fmt.Sprintf(`
resource "netcup_server_disk_driver" "test" {
  server_id = %s
  driver    = %q
}
`, id, driver)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(drivers[other]),
				Check:  resource.TestCheckResourceAttr("netcup_server_disk_driver.test", "driver", drivers[other]),
			},
			{
				ResourceName:            "netcup_server_disk_driver.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				Config: config(current),
				Check:  resource.TestCheckResourceAttr("netcup_server_disk_driver.test", "driver", current),
			},
		},
	})
}

// The test shuts NETCUP_TEST_SERVER_ID down, sets a random root password, sets
// another one and starts the server again. Nobody learns the passwords.
func TestAccServerSettingsRootPassword(t *testing.T) {
	id := testAccLiveServerID(t)
	testAccKeepServerRunning(t, id)
	config := func(powerState, stateOption string, version int) string {
		return fmt.Sprintf(`
resource "netcup_server_settings" "test" {
  server_id                = %s
  power_state              = %q
  state_option             = %s
  root_password_wo         = %q
  root_password_wo_version = %d
}
`, id, powerState, stateOption, "Tf-"+acctest.RandString(20)+"-7", version)
	}
	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("OFF", `"POWEROFF"`, 1),
				Check:  resource.TestCheckResourceAttr("netcup_server_settings.test", "power_state", "OFF"),
			},
			{
				Config: config("ON", "null", 2),
				Check:  resource.TestCheckResourceAttr("netcup_server_settings.test", "power_state", "ON"),
			},
		},
	})
}
