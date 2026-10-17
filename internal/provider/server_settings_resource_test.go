// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

// The test changes only the nickname and autostart of NETCUP_TEST_SERVER_ID.
// netcup_server_settings has no delete, so the server keeps the last nickname.
func TestAccServerSettingsResource(t *testing.T) {
	serverID := os.Getenv("NETCUP_TEST_SERVER_ID")
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheckEnv(t, "NETCUP_TEST_SERVER_ID")
			testAccPreCheckSCP(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{ // create
				Config: testAccServerSettingsConfig(serverID, "tf-acc-test-1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("netcup_server_settings.test", "id", serverID),
					resource.TestCheckResourceAttr("netcup_server_settings.test", "nickname", "tf-acc-test-1"),
					resource.TestCheckResourceAttr("netcup_server_settings.test", "autostart", "true"),
					resource.TestCheckResourceAttr("netcup_server_settings.test", "power_state", "ON"),
				),
			},
			{ // update in place
				Config: testAccServerSettingsConfig(serverID, "tf-acc-test-2"),
				Check:  resource.TestCheckResourceAttr("netcup_server_settings.test", "nickname", "tf-acc-test-2"),
			},
			{ // import (server id)
				ResourceName:            "netcup_server_settings.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func testAccServerSettingsConfig(serverID, nickname string) string {
	return fmt.Sprintf(`
resource "netcup_server_settings" "test" {
  server_id = %[1]s
  nickname  = %[2]q
  autostart = true
}
`, serverID, nickname)
}

func TestServerSettingsPatchOrder(t *testing.T) {
	cur := &scp.Server{
		Nickname: "old",
		LiveInfo: &scp.ServerLiveInfo{State: "RUNNING", Autostart: true, UEFI: false},
	}
	plan := serverSettingsModel{
		Nickname:   types.StringValue("new"),
		Autostart:  types.BoolValue(true),
		UEFI:       types.BoolValue(true),
		PowerState: types.StringValue("OFF"),
	}
	var diags diag.Diagnostics
	got := plan.patches(context.Background(), cur, &diagAppender{diags: &diags})
	want := []map[string]any{
		{"state": "OFF"},
		{"nickname": "new"},
		{"uefi": true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("patches = %v, want %v", got, want)
	}

	cur.LiveInfo.State = "SHUTOFF"
	plan.PowerState = types.StringValue("ON")
	got = plan.patches(context.Background(), cur, &diagAppender{diags: &diags})
	if last := got[len(got)-1]; !reflect.DeepEqual(last, map[string]any{"state": "ON"}) {
		t.Fatalf("last patch = %v, want start", last)
	}
}

func TestPowerState(t *testing.T) {
	for state, want := range map[string]string{
		"RUNNING":       "ON",
		"BLOCKED":       "ON",
		"PAUSED":        "SUSPENDED",
		"PMSUSPENDED":   "SUSPENDED",
		"SHUTOFF":       "OFF",
		"SHUTDOWN":      "OFF",
		"CRASHED":       "OFF",
		"DISK_SNAPSHOT": "OFF",
	} {
		if got := powerState(state); got != want {
			t.Errorf("powerState(%s) = %s, want %s", state, got, want)
		}
	}
}
