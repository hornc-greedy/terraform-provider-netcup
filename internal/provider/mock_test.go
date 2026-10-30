// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/mockapi"
)

// testMockConfig prepends the provider block that points at the mock.
func testMockConfig(m *mockapi.Server, config string) string {
	return fmt.Sprintf(`
provider "netcup" {
  scp_refresh_token = %q
  scp_base_url      = %q
}
`, mockapi.RefreshToken, m.URL) + config
}

func testMockCase(steps ...resource.TestStep) resource.TestCase {
	return resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	}
}

// resource.Test fails on a non-empty plan after each step. The mock returns
// addresses reversed, so this proves that the configured order stays stable.
func TestMockFirewallPolicy(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_firewall_policy.test"
	resource.Test(t, testMockCase(
		resource.TestStep{
			Config: testMockConfig(m, `
resource "netcup_firewall_policy" "test" {
  name        = "p"
  description = "d"

  rule {
    action       = "ACCEPT"
    protocol     = "TCP"
    direction    = "INGRESS"
    sources      = ["203.0.113.20/32", "192.0.2.10/32", "198.51.100.5/32"]
    destinations = []
    description  = ""
  }
}

output "first_source" {
  value = netcup_firewall_policy.test.rule[0].sources[0]
}
`),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckOutput("first_source", "203.0.113.20/32"),
				resource.TestCheckResourceAttr(name, "rule.0.destinations.#", "0"),
			),
		},
		resource.TestStep{
			Config: testMockConfig(m, `
resource "netcup_firewall_policy" "test" {
  name = "p"
}
`),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(name, "description", ""),
				resource.TestCheckResourceAttr(name, "rule.#", "0"),
			),
		},
		resource.TestStep{
			ResourceName: name, ImportState: true, ImportStateVerify: true,
			ImportStateVerifyIgnore: []string{"timeouts"},
		},
	))
}

func TestMockFirewallPolicyFailedUpdateTask(t *testing.T) {
	m := mockapi.New(t)
	config := func(name string) string {
		return testMockConfig(m, fmt.Sprintf(`
resource "netcup_firewall_policy" "test" {
  name = %q
}
`, name))
	}
	resource.Test(t, testMockCase(
		resource.TestStep{Config: config("a")},
		resource.TestStep{
			PreConfig:   func() { m.FailTasks("disk full") },
			Config:      config("b"),
			ExpectError: regexp.MustCompile("disk full"),
		},
	))
}

func TestMockSSHKey(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_ssh_key.test"
	resource.Test(t, testMockCase(
		resource.TestStep{
			Config: testMockConfig(m, `
resource "netcup_ssh_key" "test" {
  name = "k"
  key  = "ssh-ed25519 AAAA tf-mock\n"
}

data "netcup_ssh_keys" "all" {
  depends_on = [netcup_ssh_key.test]
}
`),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet(name, "id"),
				resource.TestCheckResourceAttr("data.netcup_ssh_keys.all", "keys.#", "1"),
			),
		},
		resource.TestStep{ResourceName: name, ImportState: true, ImportStateVerify: true},
	))
}

func TestMockReverseDNS(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_reverse_dns.test"
	config := func(hostname string) string {
		return testMockConfig(m, fmt.Sprintf(`
resource "netcup_reverse_dns" "test" {
  ip       = %q
  hostname = %q
}
`, mockapi.DefaultIPv4, hostname))
	}
	resource.Test(t, testMockCase(
		resource.TestStep{Config: config("a.example.com"), Check: resource.TestCheckResourceAttr(name, "hostname", "a.example.com")},
		resource.TestStep{Config: config("b.example.com"), Check: resource.TestCheckResourceAttr(name, "hostname", "b.example.com")},
		resource.TestStep{ResourceName: name, ImportState: true, ImportStateVerify: true},
	))
}

func TestMockServerSettings(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_server_settings.test"
	config := func(nickname string) string {
		return testMockConfig(m, fmt.Sprintf(`
resource "netcup_server_settings" "test" {
  server_id = %d
  nickname  = %q
  autostart = true
  bootorder = ["CDROM", "HDD"]
}
`, mockapi.DefaultServerID, nickname))
	}
	resource.Test(t, testMockCase(
		resource.TestStep{
			Config: config("one"),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(name, "nickname", "one"),
				resource.TestCheckResourceAttr(name, "autostart", "true"),
				resource.TestCheckResourceAttr(name, "power_state", "ON"),
			),
		},
		resource.TestStep{Config: config("two"), Check: resource.TestCheckResourceAttr(name, "nickname", "two")},
		resource.TestStep{
			ResourceName: name, ImportState: true, ImportStateVerify: true,
			ImportStateVerifyIgnore: []string{"timeouts"},
		},
	))
}

func TestMockServerSnapshot(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_server_snapshot.test"
	resource.Test(t, testMockCase(
		resource.TestStep{
			Config: testMockConfig(m, fmt.Sprintf(`
resource "netcup_server_snapshot" "test" {
  server_id = %d
  name      = "s"
  disk_name = "vda"
}
`, mockapi.DefaultServerID)),
			Check: resource.TestCheckResourceAttr(name, "disk_name", "vda"),
		},
		resource.TestStep{
			ResourceName: name, ImportState: true, ImportStateVerify: true,
			ImportStateVerifyIgnore: []string{"timeouts"},
		},
	))
}

func testMockServerFirewallConfig(m *mockapi.Server, keepCopied bool) string {
	return testMockConfig(m, fmt.Sprintf(`
resource "netcup_firewall_policy" "a" {
  name = "a"
}

resource "netcup_firewall_policy" "b" {
  name = "b"
}

resource "netcup_server_firewall" "test" {
  server_id            = %d
  mac                  = %q
  policy_ids           = [netcup_firewall_policy.b.id, netcup_firewall_policy.a.id]
  keep_copied_policies = %t
}
`, mockapi.DefaultServerID, mockapi.DefaultMAC, keepCopied))
}

func TestMockServerFirewallKeepsCopiedPolicies(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockServerFirewallConfig(m, true),
		Check: func(*terraform.State) error {
			copied, user, active := m.FirewallOf(mockapi.DefaultMAC)
			if len(copied) != 1 || len(user) != 2 || !active {
				return fmt.Errorf("copied %v, user %v, active %v", copied, user, active)
			}
			return nil
		},
	}))
}

func TestMockServerFirewallRemovesCopiedPolicies(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_server_firewall.test"
	resource.Test(t, testMockCase(
		resource.TestStep{
			Config: testMockServerFirewallConfig(m, false),
			Check: func(*terraform.State) error {
				if copied, _, _ := m.FirewallOf(mockapi.DefaultMAC); len(copied) != 0 {
					return fmt.Errorf("copied policies still assigned: %v", copied)
				}
				return nil
			},
		},
		resource.TestStep{
			ResourceName: name, ImportState: true, ImportStateVerify: true,
			ImportStateVerifyIgnore: []string{"timeouts"},
		},
	))
}

func TestMockFailoverRoute(t *testing.T) {
	m := mockapi.New(t)
	config := testMockConfig(m, fmt.Sprintf(`
resource "netcup_failover_ip_route" "test" {
  failover_ip_id = %d
  server_id      = %d
  ipv6           = true
}
`, mockapi.FailoverV6ID, mockapi.DefaultServerID))
	resource.Test(t, testMockCase(
		resource.TestStep{Config: config, Check: resource.TestCheckResourceAttr("netcup_failover_ip_route.test", "id", fmt.Sprintf("v6/%d", mockapi.FailoverV6ID))},
		resource.TestStep{ResourceName: "netcup_failover_ip_route.test", ImportState: true, ImportStateVerify: true},
	))
}

// The failover IP disappears outside of Terraform, so the refresh removes the
// route from state and the plan wants to create it again.
func TestMockFailoverRouteLostIP(t *testing.T) {
	m := mockapi.New(t)
	config := testMockConfig(m, fmt.Sprintf(`
resource "netcup_failover_ip_route" "test" {
  failover_ip_id = %d
  server_id      = %d
}
`, mockapi.FailoverV4ID, mockapi.DefaultServerID))
	resource.Test(t, testMockCase(
		resource.TestStep{Config: config},
		resource.TestStep{
			PreConfig:          func() { m.RemoveFailover(mockapi.FailoverV4ID) },
			Config:             config,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
	))
}

func TestMockDataSources(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, fmt.Sprintf(`
data "netcup_servers" "all" {}

data "netcup_server" "one" {
  id = %[1]d
}

data "netcup_server_interfaces" "one" {
  server_id = %[1]d
}

data "netcup_firewall_policies" "all" {}

data "netcup_failover_ips" "all" {}
`, mockapi.DefaultServerID)),
		Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("data.netcup_servers.all", "servers.#", "1"),
			resource.TestCheckResourceAttr("data.netcup_server.one", "snapshot_count", strconv.Itoa(mockapi.DefaultExportCount)),
			resource.TestCheckResourceAttr("data.netcup_server_interfaces.one", "interfaces.0.mac", mockapi.DefaultMAC),
			resource.TestCheckResourceAttr("data.netcup_server_interfaces.one", "interfaces.0.ipv6_addresses.0.rdns.%", "0"),
			resource.TestCheckResourceAttr("data.netcup_firewall_policies.all", "policies.#", "0"),
			resource.TestCheckResourceAttr("data.netcup_failover_ips.all", "ipv4.#", "1"),
		),
	}))
}

// testMockCloudDNSConfig prepends the provider block that points at the CloudDNS
// part of the mock.
func testMockCloudDNSConfig(m *mockapi.Server, config string) string {
	return fmt.Sprintf(`
provider "netcup" {
  clouddns_api_key  = %q
  clouddns_base_url = %q
}
`, mockapi.APIKey, m.URL) + config
}

func TestMockCloudDNSRecordsOnLegacyDomain(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockCloudDNSConfig(m, fmt.Sprintf(`
data "netcup_clouddns_records" "legacy" {
  domain_id = %d
}
`, mockapi.LegacyDomainID)),
		ExpectError: regexp.MustCompile(`does not run on CloudDNS`),
	}))
}

func TestMockCloudDNSDataSources(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockCloudDNSConfig(m, fmt.Sprintf(`
data "netcup_clouddns_record_types" "all" {}

data "netcup_clouddns_domains" "all" {}

data "netcup_clouddns_revisions" "all" {
  domain_id = %[1]d
}

data "netcup_clouddns_revision" "active" {
  domain_id = %[1]d
}

data "netcup_clouddns_records" "active" {
  domain_id = %[1]d
}

data "netcup_clouddns_record" "first" {
  domain_id  = %[1]d
  identifier = data.netcup_clouddns_records.active.records[0].identifier
}

data "netcup_clouddns_revision" "by_id" {
  domain_id  = %[1]d
  identifier = data.netcup_clouddns_revision.active.identifier
}

data "netcup_clouddns_records" "by_revision" {
  domain_id   = %[1]d
  revision_id = data.netcup_clouddns_revision.active.identifier
}
`, mockapi.CloudDNSDomainID)),
		Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("data.netcup_clouddns_domains.all", "domains.#", "2"),
			resource.TestCheckResourceAttr("data.netcup_clouddns_revisions.all", "revisions.#", "1"),
			resource.TestCheckResourceAttr("data.netcup_clouddns_revision.active", "state", "active"),
			resource.TestCheckResourceAttr("data.netcup_clouddns_records.active", "records.0.immutable", "true"),
			resource.TestCheckResourceAttr("data.netcup_clouddns_record.first", "type", "NS"),
			resource.TestCheckResourceAttr("data.netcup_clouddns_revision.by_id", "state", "active"),
			resource.TestCheckResourceAttr("data.netcup_clouddns_records.by_revision", "records.#", "1"),
			resource.TestCheckResourceAttrSet("data.netcup_clouddns_record_types.all", "types.#"),
		),
	}))
}

func TestMockCloudDNSMissingDomain(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockCloudDNSConfig(m, `
data "netcup_clouddns_domain" "missing" {
  fqdn = "missing.example"
}
`),
		ExpectError: regexp.MustCompile(`Domain not found`),
	}))
}

func TestMockProviderRejectsUnknownAttribute(t *testing.T) {
	for attribute, object := range map[string]string{
		"scp_refresh_token": `data "netcup_servers" "test" {}`,
		"api_key":           `data "netcup_dns_records" "test" { domain = "example.com" }`,
		"clouddns_api_key":  `data "netcup_clouddns_domains" "test" {}`,
	} {
		t.Run(attribute, func(t *testing.T) {
			// Its environment variable would stand in for the unknown value.
			t.Setenv("NETCUP_"+strings.ToUpper(attribute), "")
			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config: fmt.Sprintf(`
resource "terraform_data" "value" {
  input = "x"
}

provider "netcup" {
  %s = terraform_data.value.output
}

%s
`, attribute, object),
					ExpectError: regexp.MustCompile("Unknown provider attribute"),
				}},
			})
		})
	}
}

// Another API needs nothing of an attribute that is not known yet.
func TestMockProviderAllowsUnknownAttributeOfUnusedAPI(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: fmt.Sprintf(`
resource "terraform_data" "value" {
  input = "x"
}

provider "netcup" {
  scp_refresh_token = %q
  scp_base_url      = %q
  api_key           = terraform_data.value.output
}

data "netcup_servers" "test" {}
`, mockapi.RefreshToken, m.URL),
		Check: resource.TestCheckResourceAttrSet("data.netcup_servers.test", "servers.#"),
	}))
}
