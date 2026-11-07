// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
)

// The test changes the language, the time zone and show_nickname of the user of
// the token and then clears its API IP login restrictions.
func TestAccUserResource(t *testing.T) {
	config := func(language, timeZone string, showNickname bool, extra string) string {
		return fmt.Sprintf(`
resource "netcup_user" "test" {
  language      = %q
  time_zone     = %q
  show_nickname = %t
  %s
}
`, language, timeZone, showNickname, extra)
	}
	check := func(language, timeZone, showNickname string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("netcup_user.test", "language", language),
			resource.TestCheckResourceAttr("netcup_user.test", "time_zone", timeZone),
			resource.TestCheckResourceAttr("netcup_user.test", "show_nickname", showNickname),
		)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccLiveAccount(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("de", "Europe/Berlin", true, ""),
				Check:  check("de", "Europe/Berlin", "true"),
			},
			{
				ResourceName:            "netcup_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				Config: config("en", "Europe/Vienna", false, `api_ip_login_restrictions = ""`),
				Check: resource.ComposeAggregateTestCheckFunc(
					check("en", "Europe/Vienna", "false"),
					resource.TestCheckResourceAttr("netcup_user.test", "api_ip_login_restrictions", ""),
				),
			},
		},
	})
}

// testAccUserObject uploads a file under a key with a space, overwrites it with
// a larger one, lists it, fetches its download link and deletes it.
func testAccUserObject(t *testing.T, kind scp.ObjectKind, typeName, dataSource, download, key string) {
	t.Helper()
	dir := t.TempDir()
	file := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	small := file("small", "12345")
	big := file("big", "1234567890")
	config := func(source, hash string) string {
		return fmt.Sprintf(`
resource %[1]q "test" {
  key         = %[2]q
  source      = %[3]q
  source_hash = %[4]q
}
`, typeName, key, source, hash)
	}
	address := typeName + ".test"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccLiveAccount(t) },
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_10_0)},
		ProtoV6ProviderFactories: testAccEchoProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			objects, err := testAccSCPClient().ListObjects(context.Background(), kind)
			if err != nil {
				return err
			}
			for _, object := range objects {
				if object.Key == key {
					return fmt.Errorf("%s survived the destroy", key)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: config(small, "1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", key),
					resource.TestCheckResourceAttr(address, "size_in_b", "5"),
					resource.TestCheckResourceAttrSet(address, "type"),
					resource.TestCheckResourceAttrSet(address, "last_modified"),
				),
			},
			{
				ResourceName:            address,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"source", "source_hash", "timeouts"},
			},
			{
				Config: config(big, "2") + fmt.Sprintf(`
data %[1]q "test" {
  depends_on = [%[2]s]
}

ephemeral %[3]q "test" {
  key        = %[2]s.key
  depends_on = [%[2]s]
}

provider "echo" {
  data = ephemeral.%[3]s.test
}

resource "echo" "test" {}
`, dataSource, address, download),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "size_in_b", "10"),
					resource.TestCheckTypeSetElemNestedAttrs("data."+dataSource+".test", "objects.*", map[string]string{
						"key":       key,
						"size_in_b": "10",
					}),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("presigned_url"), knownvalue.NotNull()),
				},
			},
		},
	})
}

func TestAccUserImageResource(t *testing.T) {
	testAccUserObject(t, scp.ObjectKindImages, "netcup_user_image", "netcup_user_images", "netcup_user_image_download", "tf-acc test.raw")
}

func TestAccUserIsoResource(t *testing.T) {
	testAccUserObject(t, scp.ObjectKindIsos, "netcup_user_iso", "netcup_user_isos", "netcup_user_iso_download", "tf-acc test.iso")
}
