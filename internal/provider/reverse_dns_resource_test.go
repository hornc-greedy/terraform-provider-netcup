// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// The test sets the PTR of NETCUP_TEST_RDNS_IPV4 and of NETCUP_TEST_RDNS_IPV6,
// which must belong to NETCUP_TEST_SERVER_ID, and deletes it afterwards.
func TestAccReverseDNSResource(t *testing.T) {
	serverID := testAccLiveServerID(t)
	domain := os.Getenv("NETCUP_TEST_RDNS_DOMAIN")
	if domain == "" {
		t.Skip("NETCUP_TEST_RDNS_DOMAIN not set")
	}
	for _, variable := range []string{"NETCUP_TEST_RDNS_IPV4", "NETCUP_TEST_RDNS_IPV6"} {
		t.Run(variable, func(t *testing.T) {
			ip := os.Getenv(variable)
			if ip == "" {
				t.Skipf("%s not set", variable)
			}
			testAccRequireServerIP(t, serverID, ip)
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             testAccCheckReverseDNSDestroy("tf-acc-test-2." + domain),
				Steps: []resource.TestStep{
					{ // create
						Config: testAccReverseDNSConfig(ip, "tf-acc-test-1."+domain),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr("netcup_reverse_dns.test", "id", ip),
							resource.TestCheckResourceAttr("netcup_reverse_dns.test", "hostname", "tf-acc-test-1."+domain),
						),
					},
					{ // update in place
						Config: testAccReverseDNSConfig(ip, "tf-acc-test-2."+domain),
						Check:  resource.TestCheckResourceAttr("netcup_reverse_dns.test", "hostname", "tf-acc-test-2."+domain),
					},
					{ // import (ip)
						ResourceName:      "netcup_reverse_dns.test",
						ImportState:       true,
						ImportStateVerify: true,
					},
				},
			})
		})
	}
}

func testAccCheckReverseDNSDestroy(hostname string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccSCPClient()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "netcup_reverse_dns" {
				continue
			}
			got, err := c.GetReverseDNS(context.Background(), rs.Primary.ID)
			if err != nil {
				return err
			}
			if got == hostname {
				return fmt.Errorf("reverse dns of %s still points to %s", rs.Primary.ID, hostname)
			}
		}
		return nil
	}
}

func testAccReverseDNSConfig(ip, hostname string) string {
	return fmt.Sprintf(`
resource "netcup_reverse_dns" "test" {
  ip       = %[1]q
  hostname = %[2]q
}
`, ip, hostname)
}

// testAccRequireServerIP fails unless ip is an IPv4 address of the server or lies
// in one of its IPv6 networks.
func testAccRequireServerIP(t *testing.T, serverID, ip string) {
	t.Helper()
	id, err := strconv.Atoi(serverID)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		t.Fatal(err)
	}
	interfaces, err := testAccSCPClient().GetInterfaces(context.Background(), id)
	if err != nil {
		t.Fatalf("read interfaces of server %s: %v", serverID, err)
	}
	for _, i := range interfaces {
		for _, a := range i.IPv4Addresses {
			if a.IP == ip {
				return
			}
		}
		for _, a := range i.IPv6Addresses {
			if network, err := netip.ParsePrefix(a.CIDR); err == nil && !a.LinkLocal && network.Contains(addr) {
				return
			}
		}
	}
	t.Fatalf("%s is no address of server %s", ip, serverID)
}
