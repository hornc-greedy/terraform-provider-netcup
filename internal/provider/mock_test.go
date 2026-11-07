// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/echoprovider"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/hornc-greedy/terraform-provider-netcup/internal/mockapi"
	"github.com/hornc-greedy/terraform-provider-netcup/internal/scp"
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

func testMockCloudDNSRecord(rdata string, ttl int) string {
	return fmt.Sprintf(`
data "netcup_clouddns_domain" "example" {
  fqdn = %q
}

resource "netcup_clouddns_record" "test" {
  domain_id = data.netcup_clouddns_domain.example.id
  name      = "www"
  type      = "A"
  rdata     = %q
  ttl       = %d
}
`, mockapi.CloudDNSDomain, rdata, ttl)
}

func TestMockCloudDNSRecord(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_clouddns_record.test"
	domainID := strconv.Itoa(mockapi.CloudDNSDomainID)
	testCase := testMockCase(
		resource.TestStep{
			Config: testMockCloudDNSConfig(m, testMockCloudDNSRecord("192.0.2.1", 300)),
			Check:  resource.TestCheckResourceAttr(name, "id", domainID+"/www/A/192.0.2.1"),
		},
		resource.TestStep{
			Config: testMockCloudDNSConfig(m, testMockCloudDNSRecord("192.0.2.1", 600)),
			Check:  resource.TestCheckResourceAttr(name, "ttl", "600"),
		},
		resource.TestStep{
			Config: testMockCloudDNSConfig(m, testMockCloudDNSRecord("192.0.2.2", 600)),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(name, "id", domainID+"/www/A/192.0.2.2"),
				func(*terraform.State) error {
					if got := m.RecordRData("www", "A"); !slices.Equal(got, []string{"192.0.2.2"}) {
						return fmt.Errorf("records of www: %v", got)
					}
					return nil
				},
			),
		},
		resource.TestStep{ResourceName: name, ImportState: true, ImportStateVerify: true},
	)
	testCase.CheckDestroy = func(*terraform.State) error {
		if got := m.RecordRData("www", "A"); len(got) != 0 {
			return fmt.Errorf("records of www after destroy: %v", got)
		}
		return nil
	}
	resource.Test(t, testCase)
}

func TestMockCloudDNSManyRecordsOfOneDomain(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockCloudDNSConfig(m, fmt.Sprintf(`
resource "netcup_clouddns_record" "test" {
  count     = 5
  domain_id = %d
  name      = "host${count.index}"
  type      = "A"
  rdata     = "192.0.2.1"
  ttl       = 300
}
`, mockapi.CloudDNSDomainID)),
	}))
}

// netcup storing another form of rdata fails the apply instead of losing the
// record at the next refresh.
func TestMockCloudDNSRecordNormalizedRData(t *testing.T) {
	m := mockapi.New(t)
	m.NormalizeRData()
	resource.Test(t, testMockCase(resource.TestStep{
		Config:      testMockCloudDNSConfig(m, testMockCloudDNSRecord("192.0.2.1", 300)),
		ExpectError: regexp.MustCompile(`does not hold www A`),
	}))
}

// A record changed outside of Terraform has other rdata, so the refresh removes
// it from state and the plan wants to create it again.
func TestMockCloudDNSRecordDrift(t *testing.T) {
	m := mockapi.New(t)
	config := testMockCloudDNSConfig(m, testMockCloudDNSRecord("192.0.2.1", 300))
	resource.Test(t, testMockCase(
		resource.TestStep{Config: config},
		resource.TestStep{
			PreConfig:          func() { m.SetRecordRData("www", "A", "192.0.2.9") },
			Config:             config,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		// Restores the record, so that the destroy at the end of the test finds it.
		resource.TestStep{
			PreConfig: func() { m.SetRecordRData("www", "A", "192.0.2.1") },
			Config:    config,
			PlanOnly:  true,
		},
	))
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

func TestMockCloudDNSAcmeChallenge(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_clouddns_acme_challenge.test"
	testCase := testMockCase(
		resource.TestStep{
			Config: testMockCloudDNSConfig(m, fmt.Sprintf(`
resource "netcup_clouddns_acme_challenge" "test" {
  domain_id = %d
  scope     = "www"
  value     = "abcdefghijklmnopqrstuvwxyz0123456789_-ABCDE"
}

data "netcup_clouddns_acme_challenges" "all" {
  domain_id  = %[1]d
  depends_on = [netcup_clouddns_acme_challenge.test]
}
`, mockapi.CloudDNSDomainID)),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet(name, "fqdn"),
				resource.TestCheckResourceAttr("data.netcup_clouddns_acme_challenges.all", "challenges.#", "1"),
			),
		},
		resource.TestStep{ResourceName: name, ImportState: true, ImportStateVerify: true},
	)
	testCase.CheckDestroy = func(*terraform.State) error {
		if got := m.ChallengeCount(); got != 0 {
			return fmt.Errorf("challenges after destroy: %d", got)
		}
		return nil
	}
	resource.Test(t, testCase)
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

func TestMockServerInfoDataSources(t *testing.T) {
	m := mockapi.New(t)
	m.EnableGPU()
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, fmt.Sprintf(`
resource "netcup_server_snapshot" "test" {
  server_id = %[1]d
  name      = "s"
  disk_name = "vda"
}

data "netcup_server_disks" "all" {
  server_id = %[1]d
}

data "netcup_server_disk" "first" {
  server_id = %[1]d
  name      = data.netcup_server_disks.all.disks[0].name
}

data "netcup_server_disk_supported_drivers" "all" {
  server_id = %[1]d
}

data "netcup_server_guest_agent" "agent" {
  server_id = %[1]d
}

data "netcup_server_guest_agent_status" "agent" {
  server_id = %[1]d
}

data "netcup_server_image_flavours" "all" {
  server_id = %[1]d
}

data "netcup_server_iso_images" "all" {
  server_id = %[1]d
}

data "netcup_server_logs" "first" {
  server_id = %[1]d
  limit     = 1
}

data "netcup_server_logs" "second" {
  server_id = %[1]d
  limit     = 1
  offset    = 1
}

data "netcup_server_metrics_cpu" "metric" {
  server_id = %[1]d
  hours     = 2
}

data "netcup_server_metrics_disk" "metric" {
  server_id = %[1]d
}

data "netcup_server_metrics_network" "metric" {
  server_id = %[1]d
}

data "netcup_server_metrics_network_packet" "metric" {
  server_id = %[1]d
}

data "netcup_server_snapshots" "all" {
  server_id  = %[1]d
  depends_on = [netcup_server_snapshot.test]
}

data "netcup_server_snapshot_dryrun" "disk" {
  server_id = %[1]d
  disk_name = "vda"
}

data "netcup_server_snapshot_dryrun" "online_with_disk" {
  server_id       = %[1]d
  disk_name       = "vda"
  online_snapshot = true
}

data "netcup_tasks" "finished" {
  server_id  = %[1]d
  state      = "FINISHED"
  depends_on = [netcup_server_snapshot.test]
}

output "cpu" {
  value = jsondecode(data.netcup_server_metrics_cpu.metric.data)
}

output "guest_agent_version" {
  value = jsondecode(data.netcup_server_guest_agent.agent.guest_agent_data).version
}
`, mockapi.DefaultServerID)),
		Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("data.netcup_server_disk.first", "storage_driver", "VIRTIO"),
			resource.TestCheckResourceAttr("data.netcup_server_disks.all", "disks.0.capacity_in_mib", "8192"),
			resource.TestCheckResourceAttr("data.netcup_server_disk_supported_drivers.all", "supported_drivers.#", "2"),
			resource.TestCheckResourceAttr("data.netcup_server_guest_agent.agent", "guest_agent_available", "true"),
			resource.TestCheckOutput("guest_agent_version", "9.0"),
			resource.TestCheckResourceAttr("data.netcup_server_guest_agent_status.agent", "available", "true"),
			resource.TestCheckResourceAttr("data.netcup_server_image_flavours.all", "image_flavours.0.image.name", "Debian 13.7.0 BIOS amd64"),
			resource.TestCheckResourceAttr("data.netcup_server_iso_images.all", "iso_images.0.filename", "rescue.iso"),
			resource.TestCheckNoResourceAttr("data.netcup_server_iso_images.all", "iso_images.0.description"),
			resource.TestCheckResourceAttr("data.netcup_server_logs.second", "logs.#", "1"),
			resource.TestCheckResourceAttr("data.netcup_server_logs.second", "logs.0.log_key", "server.rebooted"),
			resource.TestCheckResourceAttr("data.netcup_server_logs.first", "logs.0.executing_user.username", "mock"),
			resource.TestCheckNoResourceAttr("data.netcup_server_logs.first", "logs.0.executing_user.company"),
			resource.TestMatchResourceAttr("data.netcup_server_metrics_cpu.metric", "data", regexp.MustCompile(`"cpu0"`)),
			resource.TestMatchResourceAttr("data.netcup_server_metrics_disk.metric", "data", regexp.MustCompile(`"vda"`)),
			resource.TestMatchResourceAttr("data.netcup_server_metrics_network.metric", "data", regexp.MustCompile(`"eth0"`)),
			resource.TestMatchResourceAttr("data.netcup_server_metrics_network_packet.metric", "data", regexp.MustCompile(`"eth0packets"`)),
			func(*terraform.State) error {
				for _, r := range m.Requests() {
					if strings.HasSuffix(r.Path, "/metrics/cpu") && r.Query.Get("hours") != "2" {
						return fmt.Errorf("hours = %q", r.Query.Get("hours"))
					}
					if strings.HasSuffix(r.Path, "/tasks") && (r.Query.Get("state") != "FINISHED" || r.Query.Get("serverId") != strconv.Itoa(mockapi.DefaultServerID)) {
						return fmt.Errorf("task query = %v", r.Query)
					}
				}
				return nil
			},
			resource.TestCheckResourceAttr("data.netcup_server_snapshots.all", "snapshots.0.name", "s"),
			resource.TestCheckResourceAttr("data.netcup_server_snapshots.all", "snapshots.0.disks.0", "vda"),
			resource.TestCheckResourceAttr("data.netcup_server_snapshots.all", "snapshots.0.exported", "false"),
			resource.TestCheckNoResourceAttr("data.netcup_server_snapshots.all", "snapshots.0.exported_size_in_kib"),
			resource.TestCheckResourceAttr("data.netcup_server_snapshot_dryrun.disk", "errors.#", "0"),
			resource.TestCheckResourceAttr("data.netcup_server_snapshot_dryrun.online_with_disk", "errors.0.code", "server.snapshot.create.error.online.diskselected"),
			resource.TestCheckResourceAttr("data.netcup_tasks.finished", "tasks.0.state", "FINISHED"),
			resource.TestCheckResourceAttr("data.netcup_tasks.finished", "tasks.0.executing_user.username", "mock"),
			resource.TestCheckResourceAttr("data.netcup_tasks.finished", "tasks.0.task_progress.progress_in_percent", "100"),
		),
	}))
}

func TestMockGuestAgentWithoutData(t *testing.T) {
	m := mockapi.New(t)
	m.RemoveGuestAgent()
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, fmt.Sprintf(`
data "netcup_server_guest_agent" "agent" {
  server_id = %d
}
`, mockapi.DefaultServerID)),
		Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("data.netcup_server_guest_agent.agent", "guest_agent_available", "false"),
			resource.TestCheckNoResourceAttr("data.netcup_server_guest_agent.agent", "guest_agent_data"),
		),
	}))
}

func TestMockSnapshotDryrunNeedsDisk(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, fmt.Sprintf(`
data "netcup_server_snapshot_dryrun" "offline" {
  server_id = %d
}
`, mockapi.DefaultServerID)),
		ExpectError: regexp.MustCompile(`Missing disk_name`),
	}))
}

// The echo provider is the official way to read an ephemeral value in a test.
func TestMockServerGPUDriverEphemeral(t *testing.T) {
	m := mockapi.New(t)
	m.EnableGPU()
	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_10_0)},
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"netcup": testAccProtoV6ProviderFactories["netcup"],
			"echo":   echoprovider.NewProviderServer(),
		},
		Steps: []resource.TestStep{{
			Config: testMockConfig(m, fmt.Sprintf(`
ephemeral "netcup_server_gpu_driver" "test" {
  server_id = %d
}

provider "echo" {
  data = ephemeral.netcup_server_gpu_driver.test
}

resource "echo" "test" {}
`, mockapi.DefaultServerID)),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("filename"), knownvalue.StringExact("driver.run")),
				statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("presigned_url"), knownvalue.StringRegexp(regexp.MustCompile("signature"))),
				statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("headers").AtMapKey("x-amz-checksum-mode").AtSliceIndex(0), knownvalue.StringExact("ENABLED")),
			},
		}},
	})
}

func testMockServerInterface(driver string) string {
	return fmt.Sprintf(`
resource "netcup_server_interface" "test" {
  server_id = %d
  vlan_id   = 100
  driver    = %q
}

data "netcup_server_interface" "test" {
  server_id = netcup_server_interface.test.server_id
  mac       = netcup_server_interface.test.mac
}
`, mockapi.DefaultServerID, driver)
}

func TestMockServerInterface(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_server_interface.test"
	mac := func(s *terraform.State) string {
		return s.RootModule().Resources[name].Primary.Attributes["mac"]
	}
	testCase := testMockCase(
		resource.TestStep{
			Config: testMockConfig(m, testMockServerInterface("VIRTIO")),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet(name, "mac"),
				resource.TestCheckResourceAttr(name, "speed_in_mbits", "2500"),
				resource.TestCheckResourceAttrPair(name, "mac", "data.netcup_server_interface.test", "mac"),
				resource.TestCheckResourceAttr(name, "driver", "VIRTIO"),
				resource.TestCheckResourceAttr("data.netcup_server_interface.test", "ipv4_addresses.#", "0"),
				func(s *terraform.State) error {
					if got := m.InterfaceVlan(mac(s)); got != 100 {
						return fmt.Errorf("vlan = %d", got)
					}
					return nil
				},
			),
		},
		resource.TestStep{
			Config: testMockConfig(m, testMockServerInterface("E1000")),
			Check: func(s *terraform.State) error {
				if got := m.InterfaceDriver(mac(s)); got != "E1000" {
					return fmt.Errorf("driver = %q", got)
				}
				return nil
			},
		},
		resource.TestStep{
			ResourceName: name, ImportState: true, ImportStateVerify: true,
			ImportStateIdFunc: func(s *terraform.State) (string, error) {
				return fmt.Sprintf("%d/%s", mockapi.DefaultServerID, mac(s)), nil
			},
			ImportStateVerifyIgnore: []string{"timeouts"},
		},
	)
	testCase.CheckDestroy = func(*terraform.State) error {
		if got := m.InterfaceCount(); got != 1 {
			return fmt.Errorf("interfaces after destroy: %d", got)
		}
		return nil
	}
	resource.Test(t, testCase)
}

func TestMockServerInterfaceRemovedOutsideOfTerraform(t *testing.T) {
	m := mockapi.New(t)
	var removed string
	config := testMockConfig(m, `
resource "netcup_server_interface" "test" {
  server_id = `+strconv.Itoa(mockapi.DefaultServerID)+`
  vlan_id   = 100
  driver    = "VIRTIO"
}
`)
	resource.Test(t, testMockCase(
		resource.TestStep{
			Config: config,
			Check: func(s *terraform.State) error {
				removed = s.RootModule().Resources["netcup_server_interface.test"].Primary.Attributes["mac"]
				return nil
			},
		},
		resource.TestStep{
			PreConfig:          func() { m.RemoveInterface(removed) },
			Config:             config,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		// Recreates the interface, so that the destroy at the end finds it.
		resource.TestStep{Config: config},
	))
}

func TestMockServerInterfaceImportRejectsPublicInterface(t *testing.T) {
	m := mockapi.New(t)
	config := testMockConfig(m, `
resource "netcup_server_interface" "test" {
  server_id = `+strconv.Itoa(mockapi.DefaultServerID)+`
  vlan_id   = 100
  driver    = "VIRTIO"
}
`)
	resource.Test(t, testMockCase(
		resource.TestStep{Config: config},
		resource.TestStep{
			ResourceName: "netcup_server_interface.test", ImportState: true,
			ImportStateId: fmt.Sprintf("%d/%s", mockapi.DefaultServerID, mockapi.DefaultMAC),
			ExpectError:   regexp.MustCompile("Not a VLAN interface"),
		},
	))
}

func TestMockServerInterfaceNullValues(t *testing.T) {
	m := mockapi.New(t)
	const singular = "data.netcup_server_interface.default"
	const plural = "data.netcup_server_interfaces.all"
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, fmt.Sprintf(`
data "netcup_server_interface" "default" {
  server_id = %[1]d
  mac       = %[2]q
}

data "netcup_server_interfaces" "all" {
  server_id = %[1]d
}
`, mockapi.DefaultServerID, mockapi.DefaultMAC)),
		Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(singular, "ipv4_addresses.0.type", "IP"),
			resource.TestCheckResourceAttr(singular, "ipv4_addresses.0.interface_mac", mockapi.DefaultMAC),
			resource.TestCheckNoResourceAttr(singular, "ipv4_addresses.0.destination_ip"),
			resource.TestCheckNoResourceAttr(singular, "ipv4_addresses.1.gateway"),
			resource.TestCheckNoResourceAttr(singular, "ipv4_addresses.1.rdns"),
			resource.TestCheckResourceAttr(singular, "ipv4_addresses.1.destination_ip", mockapi.DefaultIPv4),
			resource.TestCheckResourceAttr(singular, "ipv6_addresses.0.editable", "false"),
			resource.TestCheckResourceAttr(plural, "interfaces.0.ipv4_addresses.1.type", "ROUTED_IP"),
			resource.TestCheckResourceAttr(plural, "interfaces.0.ipv4_addresses.1.gateway", ""),
			resource.TestCheckResourceAttr(plural, "interfaces.0.ipv4_addresses.1.rdns", ""),
		),
	}))
}

func testMockUserVlan(name string) string {
	return fmt.Sprintf(`
resource "netcup_user_vlan" "test" {
  vlan_id = %d
  name    = %q
}

data "netcup_user_vlans" "server" {
  server_id  = %d
  depends_on = [netcup_user_vlan.test]
}

data "netcup_user_vlans" "other_server" {
  server_id = 1
}

data "netcup_user_vlan" "test" {
  vlan_id    = netcup_user_vlan.test.vlan_id
  depends_on = [netcup_user_vlan.test]
}

data "netcup_vlan" "foreign" {
  vlan_id = %d
}
`, mockapi.VlanID, name, mockapi.DefaultServerID, mockapi.ForeignVlanID)
}

func TestMockUserVlan(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_user_vlan.test"
	resource.Test(t, testMockCase(
		resource.TestStep{
			Config: testMockConfig(m, testMockUserVlan("one")),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(name, "name", "one"),
				resource.TestCheckResourceAttr(name, "site.city", "Nuernberg"),
				resource.TestCheckResourceAttr(name, "bandwidth_class.speed_in_mbit", "1000"),
				resource.TestCheckResourceAttr(name, "user.username", "mock"),
				resource.TestCheckResourceAttr("data.netcup_user_vlan.test", "name", "one"),
				resource.TestCheckResourceAttr("data.netcup_vlan.foreign", "name", "foreign"),
				resource.TestCheckResourceAttr("data.netcup_user_vlans.server", "vlans.#", "1"),
				resource.TestCheckResourceAttr("data.netcup_user_vlans.server", "vlans.0.vlan_id", strconv.Itoa(mockapi.VlanID)),
				resource.TestCheckResourceAttr("data.netcup_user_vlans.other_server", "vlans.#", "0"),
			),
		},
		resource.TestStep{
			Config: testMockConfig(m, testMockUserVlan("two")),
			Check: func(*terraform.State) error {
				if got := m.VlanName(); got != "two" {
					return fmt.Errorf("name = %q", got)
				}
				return nil
			},
		},
		resource.TestStep{ResourceName: name, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"timeouts"}},
	))
}

func testMockUserVlanOnly(name string) string {
	return fmt.Sprintf(`
resource "netcup_user_vlan" "test" {
  vlan_id = %d
  name    = %q
}
`, mockapi.VlanID, name)
}

func TestMockUserVlanMissing(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, `
resource "netcup_user_vlan" "test" {
  vlan_id = 1
  name    = "one"
}
`),
		ExpectError: regexp.MustCompile("404"),
	}))
}

func TestMockUserVlanSendsNoChangeForTheSameName(t *testing.T) {
	m := mockapi.New(t)
	m.RejectVlanChange()
	resource.Test(t, testMockCase(
		resource.TestStep{
			Config: testMockConfig(m, testMockUserVlanOnly("default")),
			Check: func(*terraform.State) error {
				if got := m.VlanPuts(); got != 0 {
					return fmt.Errorf("PUT requests = %d", got)
				}
				return nil
			},
		},
		resource.TestStep{
			Config:      testMockConfig(m, testMockUserVlanOnly("other")),
			ExpectError: regexp.MustCompile("SHUTOFF"),
		},
	))
}

func TestMockUserVlanRemovedOutsideOfTerraform(t *testing.T) {
	m := mockapi.New(t)
	config := testMockConfig(m, testMockUserVlanOnly("one"))
	resource.Test(t, testMockCase(
		resource.TestStep{Config: config},
		resource.TestStep{
			PreConfig:          m.RemoveVlan,
			Config:             config,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
	))
}

func TestMockUserVlanWithoutName(t *testing.T) {
	m := mockapi.New(t)
	config := testMockConfig(m, testMockUserVlanOnly("one"))
	resource.Test(t, testMockCase(
		resource.TestStep{Config: config},
		resource.TestStep{
			PreConfig:          m.ClearVlanName,
			Config:             config,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		resource.TestStep{Config: config, Check: resource.TestCheckResourceAttr("netcup_user_vlan.test", "name", "one")},
		resource.TestStep{
			PreConfig: m.ClearVlanName,
			Config: testMockConfig(m, fmt.Sprintf(`
data "netcup_user_vlan" "test" {
  vlan_id = %d
}
`, mockapi.VlanID)),
			Check: resource.TestCheckNoResourceAttr("data.netcup_user_vlan.test", "name"),
		},
	))
}

func testMockServerIso() string {
	return fmt.Sprintf(`
resource "netcup_server_iso" "test" {
  server_id                   = %d
  iso_id                      = 3
  change_boot_device_to_cdrom = true
}
`, mockapi.DefaultServerID)
}

func TestMockServerIso(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_server_iso.test"
	config := testMockConfig(m, testMockServerIso())
	testCase := testMockCase(
		resource.TestStep{
			Config: config,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(name, "iso", "rescue.iso"),
				func(*terraform.State) error {
					if _, body := m.AttachedIso(); !strings.Contains(body, `"isoId":3`) || !strings.Contains(body, `"changeBootDeviceToCdrom":true`) {
						return fmt.Errorf("body = %s", body)
					}
					return nil
				},
			),
		},
		resource.TestStep{
			ResourceName: name, ImportState: true, ImportStateVerify: true,
			ImportStateVerifyIgnore: []string{"timeouts", "iso_id", "change_boot_device_to_cdrom"},
		},
		// Detached outside of Terraform, the plan attaches it again.
		resource.TestStep{
			PreConfig:          func() { m.SetAttachedIso("") },
			Config:             config,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		// Another ISO outside of Terraform is drift as well.
		resource.TestStep{
			PreConfig:          func() { m.SetAttachedIso("other.iso") },
			Config:             config,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		resource.TestStep{PreConfig: func() { m.SetAttachedIso("") }, Config: config},
	)
	testCase.CheckDestroy = func(*terraform.State) error {
		if got, _ := m.AttachedIso(); got != "" {
			return fmt.Errorf("ISO after destroy: %s", got)
		}
		return nil
	}
	resource.Test(t, testCase)
}

func TestMockServerIsoUserName(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, fmt.Sprintf(`
resource "netcup_server_iso" "test" {
  server_id     = %d
  user_iso_name = "uploaded.iso"
}
`, mockapi.DefaultServerID)),
		Check: resource.TestCheckResourceAttr("netcup_server_iso.test", "iso", "uploaded.iso"),
	}))
}

func TestMockServerIsoNeedsExactlyOneName(t *testing.T) {
	m := mockapi.New(t)
	for _, attributes := range []string{"iso_id = 3\n  user_iso_name = \"uploaded.iso\"", ""} {
		resource.Test(t, testMockCase(resource.TestStep{
			Config: testMockConfig(m, fmt.Sprintf(`
resource "netcup_server_iso" "test" {
  server_id = %d
  %s
}
`, mockapi.DefaultServerID, attributes)),
			ExpectError: regexp.MustCompile("Invalid Attribute Combination"),
		}))
	}
}

func TestMockServerIsoRefusesToReplaceAttachedIso(t *testing.T) {
	m := mockapi.New(t)
	m.SetAttachedIso("manual.iso")
	resource.Test(t, testMockCase(resource.TestStep{
		Config:      testMockConfig(m, testMockServerIso()),
		ExpectError: regexp.MustCompile("ISO already attached"),
	}))
	if got, _ := m.AttachedIso(); got != "manual.iso" {
		t.Fatalf("ISO = %s", got)
	}
}

// A failed task leaves the resource tainted, so the next plan replaces it.
func TestMockServerIsoFailedTaskKeepsState(t *testing.T) {
	m := mockapi.New(t)
	config := testMockConfig(m, testMockServerIso())
	m.FailTasks("disk full")
	resource.Test(t, testMockCase(
		resource.TestStep{Config: config, ExpectError: regexp.MustCompile("disk full")},
		resource.TestStep{
			PreConfig: m.ClearFailedTasks,
			Config:    config, PlanOnly: true, ExpectNonEmptyPlan: true,
		},
	))
}

func TestMockServerIsoImportWithIgnoreChanges(t *testing.T) {
	m := mockapi.New(t)
	m.SetAttachedIso("rescue.iso")
	id := strconv.Itoa(mockapi.DefaultServerID)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, `
import {
  to = netcup_server_iso.test
  id = "`+id+`"
}

resource "netcup_server_iso" "test" {
  server_id = `+id+`
  iso_id    = 3

  lifecycle {
    ignore_changes = [iso_id, user_iso_name, change_boot_device_to_cdrom]
  }
}
`),
		Check: resource.TestCheckResourceAttr("netcup_server_iso.test", "iso", "rescue.iso"),
	}))
}

func testMockRescueSystem() string {
	return fmt.Sprintf(`
resource "netcup_server_rescue_system" "test" {
  server_id = %d
}
`, mockapi.DefaultServerID)
}

func TestMockServerRescueSystem(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_server_rescue_system.test"
	config := testMockConfig(m, testMockRescueSystem())
	testCase := testMockCase(
		resource.TestStep{
			Config: config,
			Check: func(*terraform.State) error {
				if !m.RescueActive() {
					return fmt.Errorf("rescue system is not active")
				}
				return nil
			},
		},
		resource.TestStep{ResourceName: name, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"timeouts"}},
		resource.TestStep{
			PreConfig: func() { m.SetRescueActive(false) },
			Config:    config, PlanOnly: true, ExpectNonEmptyPlan: true,
		},
		// Activates again, so that the destroy has something to deactivate.
		resource.TestStep{Config: config},
	)
	testCase.CheckDestroy = func(*terraform.State) error {
		if m.RescueActive() {
			return fmt.Errorf("rescue system still active after destroy")
		}
		return nil
	}
	resource.Test(t, testCase)
}

// testMockPatches checks the bodies of all PATCH requests so far.
func testMockPatches(m *mockapi.Server, want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := m.PatchBodies(); !slices.Equal(got, want) {
			return fmt.Errorf("PATCH bodies = %v, want %v", got, want)
		}
		return nil
	}
}

// netcup activates the rescue system only on a shut down server and shuts the
// server down to deactivate it, so a running server runs again afterwards.
func TestMockServerRescueSystemRestartsRunningServer(t *testing.T) {
	m := mockapi.New(t)
	const off, on = `{"state":"OFF"}`, `{"state":"ON"}`
	testCase := testMockCase(resource.TestStep{
		Config: testMockConfig(m, testMockRescueSystem()),
		Check:  testMockPatches(m, off, on),
	})
	testCase.CheckDestroy = testMockPatches(m, off, on, on)
	resource.Test(t, testCase)
}

func TestMockServerRescueSystemKeepsShutOffServer(t *testing.T) {
	m := mockapi.New(t)
	m.ShutOff()
	testCase := testMockCase(resource.TestStep{
		Config: testMockConfig(m, testMockRescueSystem()),
		Check:  testMockPatches(m),
	})
	testCase.CheckDestroy = testMockPatches(m)
	resource.Test(t, testCase)
}

func TestMockServerRescueSystemAlreadyActive(t *testing.T) {
	m := mockapi.New(t)
	m.SetRescueActive(true)
	resource.Test(t, testMockCase(resource.TestStep{
		Config:      testMockConfig(m, testMockRescueSystem()),
		ExpectError: regexp.MustCompile(`terraform\s+import`),
	}))
}

// The refresh of the second step removes the resource, so this covers an empty
// configuration after deactivation rather than Delete itself.
func TestMockServerRescueSystemDestroyAfterDeactivation(t *testing.T) {
	m := mockapi.New(t)
	config := testMockConfig(m, testMockRescueSystem())
	resource.Test(t, testMockCase(
		resource.TestStep{Config: config},
		resource.TestStep{
			PreConfig: func() { m.SetRescueActive(false) },
			Config:    testMockConfig(m, ""),
		},
	))
}

func TestMockServerRescueSystemFailedTaskKeepsState(t *testing.T) {
	m := mockapi.New(t)
	config := testMockConfig(m, testMockRescueSystem())
	m.FailTasks("disk full")
	resource.Test(t, testMockCase(
		resource.TestStep{Config: config, ExpectError: regexp.MustCompile("disk full")},
		resource.TestStep{
			PreConfig: m.ClearFailedTasks,
			Config:    config, PlanOnly: true, ExpectNonEmptyPlan: true,
		},
	))
}

func TestMockServerRescueSystemEphemeral(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_10_0)},
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"netcup": testAccProtoV6ProviderFactories["netcup"],
			"echo":   echoprovider.NewProviderServer(),
		},
		Steps: []resource.TestStep{{
			Config: testMockConfig(m, fmt.Sprintf(`
resource "netcup_server_rescue_system" "test" {
  server_id = %d
}

ephemeral "netcup_server_rescue_system" "test" {
  server_id  = netcup_server_rescue_system.test.server_id
  depends_on = [netcup_server_rescue_system.test]
}

provider "echo" {
  data = ephemeral.netcup_server_rescue_system.test
}

resource "echo" "test" {}
`, mockapi.DefaultServerID)),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("active"), knownvalue.Bool(true)),
				statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("password"), knownvalue.StringExact("mock-rescue-password")),
			},
		}},
	})
}

func testMockDiskDriver(driver string) string {
	return fmt.Sprintf(`
resource "netcup_server_disk_driver" "test" {
  server_id = %d
  driver    = %q
}
`, mockapi.DefaultServerID, driver)
}

func TestMockServerDiskDriver(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_server_disk_driver.test"
	config := testMockConfig(m, testMockDiskDriver("VIRTIO_SCSI"))
	resource.Test(t, testMockCase(
		resource.TestStep{
			Config: config,
			Check: func(*terraform.State) error {
				if got := m.DiskDriver(); got != "VIRTIO_SCSI" {
					return fmt.Errorf("driver = %q", got)
				}
				return nil
			},
		},
		resource.TestStep{ResourceName: name, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"timeouts"}},
		resource.TestStep{
			PreConfig: func() { m.SetDiskDriver("SATA") },
			Config:    config, PlanOnly: true, ExpectNonEmptyPlan: true,
		},
		resource.TestStep{
			Config: testMockConfig(m, testMockDiskDriver("IDE")),
			Check: func(*terraform.State) error {
				if got := m.DiskDriver(); got != "IDE" {
					return fmt.Errorf("driver = %q", got)
				}
				return nil
			},
		},
		// Only the second disk differs, the plan still shows it.
		resource.TestStep{
			PreConfig: func() { m.SetSecondDiskDriver("SATA") },
			Config:    testMockConfig(m, testMockDiskDriver("IDE")), PlanOnly: true, ExpectNonEmptyPlan: true,
		},
		resource.TestStep{Config: testMockConfig(m, testMockDiskDriver("IDE"))},
	))
}

func TestMockServerDiskDriverSendsNoChangeForTheSameDriver(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, testMockDiskDriver("VIRTIO")),
		Check: func(*terraform.State) error {
			if got := m.DiskDriver(); got != "" {
				return fmt.Errorf("PATCH sent, driver = %q", got)
			}
			return nil
		},
	}))
}

func testMockRootPassword(version int) string {
	return fmt.Sprintf(`
resource "netcup_server_settings" "test" {
  server_id                = %d
  root_password_wo         = "secret-%d"
  root_password_wo_version = %d
}
`, mockapi.DefaultServerID, version, version)
}

func TestMockServerSettingsRootPassword(t *testing.T) {
	m := mockapi.New(t)
	const name = "netcup_server_settings.test"
	count := func(want int, last string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			var got []string
			for _, body := range m.PatchBodies() {
				if strings.Contains(body, "rootPassword") {
					got = append(got, body)
				}
			}
			if len(got) != want || !strings.Contains(got[want-1], last) {
				return fmt.Errorf("root password requests = %v", got)
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testMockConfig(m, testMockRootPassword(1)),
				Check: resource.ComposeAggregateTestCheckFunc(
					count(1, "secret-1"),
					resource.TestCheckNoResourceAttr(name, "root_password_wo"),
					resource.TestCheckResourceAttr(name, "root_password_wo_version", "1"),
				),
			},
			{
				// The same version sends nothing again, whatever the value is.
				Config: testMockConfig(m, strings.ReplaceAll(testMockRootPassword(1), "secret-1", "other")),
				Check:  count(1, "secret-1"),
			},
			{
				Config: testMockConfig(m, testMockRootPassword(2)),
				Check:  count(2, "secret-2"),
			},
		},
	})
}

// The password goes after a shutdown and before a start.
func TestMockServerSettingsRootPasswordOrder(t *testing.T) {
	m := mockapi.New(t)
	config := func(state string, version int) string {
		return testMockConfig(m, fmt.Sprintf(`
resource "netcup_server_settings" "test" {
  server_id                = %d
  power_state              = %q
  root_password_wo         = "secret"
  root_password_wo_version = %d
}
`, mockapi.DefaultServerID, state, version))
	}
	lastTwo := func(first, second string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			bodies := m.PatchBodies()
			got := bodies[len(bodies)-2:]
			if !strings.Contains(got[0], first) || !strings.Contains(got[1], second) {
				return fmt.Errorf("last PATCH bodies = %v", got)
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config("OFF", 1), Check: lastTwo(`"state":"OFF"`, "rootPassword")},
			{Config: config("ON", 2), Check: lastTwo("rootPassword", `"state":"ON"`)},
		},
	})
}

func TestMockServerSettingsRootPasswordNeedsVersion(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: testMockConfig(m, fmt.Sprintf(`
resource "netcup_server_settings" "test" {
  server_id        = %d
  root_password_wo = "secret"
}
`, mockapi.DefaultServerID)),
			ExpectError: regexp.MustCompile("Invalid Attribute Combination"),
		}},
	})
}

func TestMockServerSettingsStateOption(t *testing.T) {
	m := mockapi.New(t)
	config := func(state, option string) string {
		return testMockConfig(m, fmt.Sprintf(`
resource "netcup_server_settings" "test" {
  server_id    = %d
  power_state  = %q
  state_option = %q
}
`, mockapi.DefaultServerID, state, option))
	}
	lastOption := func(want string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			requests := m.Requests()
			for i := len(requests) - 1; i >= 0; i-- {
				if requests[i].Method == http.MethodPatch {
					if got := requests[i].Query.Get("stateOption"); got != want {
						return fmt.Errorf("stateOption = %q, want %q", got, want)
					}
					return nil
				}
			}
			return fmt.Errorf("no PATCH request")
		}
	}
	resource.Test(t, testMockCase(
		resource.TestStep{Config: config("OFF", "POWERCYCLE"), ExpectError: regexp.MustCompile("Invalid state_option")},
		resource.TestStep{Config: config("OFF", "POWEROFF"), Check: lastOption("POWEROFF")},
		resource.TestStep{Config: config("ON", "RESET"), Check: lastOption("RESET")},
	))
}

// testMockAction runs an action through Terraform itself, because the testing
// library has no support for actions.
func testMockAction(m *mockapi.Server, actionType, config string, check resource.TestCheckFunc) resource.TestCase {
	return resource.TestCase{
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_14_0)},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: testMockConfig(m, fmt.Sprintf(`
action %[1]q "test" {
  config {
    %[2]s
  }
}

variable "password" {
  type      = string
  sensitive = true
  default   = "Secret123"
}

resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.%[1]s.test]
    }
  }
}
`, actionType, config)),
			Check: check,
		}},
	}
}

func testMockRequested(m *mockapi.Server, method, path string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		for _, r := range m.Requests() {
			if r.Method == method && r.Path == path {
				return nil
			}
		}
		return fmt.Errorf("no %s %s in %+v", method, path, m.Requests())
	}
}

func testMockServerPath(suffix string) string {
	return fmt.Sprintf("/api/v1/servers/%d%s", mockapi.DefaultServerID, suffix)
}

func TestMockActionDiskFormat(t *testing.T) {
	m := mockapi.New(t)
	m.ShutOff()
	resource.Test(t, testMockAction(m, "netcup_server_disk_format",
		fmt.Sprintf(`server_id = %d
    disk_name = "vda"`, mockapi.DefaultServerID),
		testMockRequested(m, http.MethodPost, testMockServerPath("/disks/vda:format")),
	))
}

// testMockBody compares the only body sent to path with want.
func testMockBody(m *mockapi.Server, path, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		for _, r := range m.Requests() {
			if r.Path == path {
				var got, expected any
				if err := json.Unmarshal([]byte(r.Body), &got); err != nil {
					return err
				}
				if err := json.Unmarshal([]byte(want), &expected); err != nil {
					return err
				}
				if !reflect.DeepEqual(got, expected) {
					return fmt.Errorf("body = %s, want %s", r.Body, want)
				}
				return nil
			}
		}
		return fmt.Errorf("no request to %s", path)
	}
}

func TestMockActionDiskFormatFailsOnRunningServer(t *testing.T) {
	testCase := testMockAction(mockapi.New(t), "netcup_server_disk_format",
		fmt.Sprintf(`server_id = %d
    disk_name = "vda"`, mockapi.DefaultServerID), nil)
	testCase.Steps[0].ExpectError = regexp.MustCompile("not in state SHUTOFF")
	resource.Test(t, testCase)
}

func TestMockActionImageSetup(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockAction(m, "netcup_server_image_setup",
		fmt.Sprintf(`server_id = %d
    image_flavour_id = 1
    disk_name = "vda"
    root_partition_full_disk_size = true
    hostname = "host.example.com"
    locale = "de_DE.UTF-8"
    timezone = "Europe/Berlin"
    additional_user_username = "admin"
    additional_user_password = var.password
    ssh_key_ids = [7, 8]
    ssh_password_authentication = false
    custom_script = "echo hi"
    email_to_executing_user = true`, mockapi.DefaultServerID),
		testMockBody(m, testMockServerPath("/image"), `{
			"imageFlavourId": 1, "diskName": "vda", "rootPartitionFullDiskSize": true,
			"hostname": "host.example.com", "locale": "de_DE.UTF-8", "timezone": "Europe/Berlin",
			"additionalUserUsername": "admin", "additionalUserPassword": "Secret123",
			"sshKeyIds": [7, 8], "sshPasswordAuthentication": false, "customScript": "echo hi",
			"emailToExecutingUser": true
		}`),
	))
}

func TestMockActionImageSetupSendsOnlyTheConfig(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockAction(m, "netcup_server_image_setup",
		fmt.Sprintf(`server_id = %d`, mockapi.DefaultServerID),
		testMockBody(m, testMockServerPath("/image"), `{}`),
	))
}

func TestMockActionImageSetupValidates(t *testing.T) {
	for attribute, config := range map[string]string{
		"additional_user_username": `additional_user_username = "Admin"`,
		"hostname":                 `hostname = "-bad-"`,
	} {
		t.Run(attribute, func(t *testing.T) {
			testCase := testMockAction(mockapi.New(t), "netcup_server_image_setup",
				fmt.Sprintf("server_id = %d\n    %s", mockapi.DefaultServerID, config), nil)
			testCase.Steps[0].ExpectError = regexp.MustCompile("Invalid Attribute Value Match")
			resource.Test(t, testCase)
		})
	}
}

func TestMockActionUserImageSetup(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockAction(m, "netcup_server_user_image_setup",
		fmt.Sprintf(`server_id = %d
    user_image_name = "custom.img"
    disk_name = "vda"
    email_notification = true`, mockapi.DefaultServerID),
		testMockBody(m, testMockServerPath("/user-image"), `{"userImageName": "custom.img", "diskName": "vda", "emailNotification": true}`),
	))
}

func TestMockActionStorageOptimization(t *testing.T) {
	m := mockapi.New(t)
	path := testMockServerPath("/storageoptimization")
	resource.Test(t, testMockAction(m, "netcup_server_storage_optimization",
		fmt.Sprintf(`server_id = %d
    disks = ["vda", "vdb"]
    start_after_optimization = true`, mockapi.DefaultServerID),
		func(*terraform.State) error {
			for _, r := range m.Requests() {
				if r.Path == path {
					if !slices.Equal(r.Query["disks"], []string{"vda", "vdb"}) || r.Query.Get("startAfterOptimization") != "true" {
						return fmt.Errorf("query = %v", r.Query)
					}
					return nil
				}
			}
			return fmt.Errorf("no request to %s", path)
		},
	))
}

func testMockSnapshotAction(t *testing.T, actionType, suffix string) {
	t.Helper()
	m := mockapi.New(t)
	m.ShutOff()
	m.AddSnapshot("s")
	resource.Test(t, testMockAction(m, actionType,
		fmt.Sprintf(`server_id = %d
    name = "s"`, mockapi.DefaultServerID),
		testMockRequested(m, http.MethodPost, testMockServerPath("/snapshots/s/"+suffix)),
	))
}

func TestMockActionSnapshotRevert(t *testing.T) {
	testMockSnapshotAction(t, "netcup_server_snapshot_revert", "revert")
}

func TestMockActionSnapshotExport(t *testing.T) {
	testMockSnapshotAction(t, "netcup_server_snapshot_export", "export")
}

func TestMockActionSnapshotExportFailsOnRunningServer(t *testing.T) {
	m := mockapi.New(t)
	m.AddSnapshot("s")
	testCase := testMockAction(m, "netcup_server_snapshot_export",
		fmt.Sprintf(`server_id = %d
    name = "s"`, mockapi.DefaultServerID), nil)
	testCase.Steps[0].ExpectError = regexp.MustCompile("no more snapshots left")
	resource.Test(t, testCase)
}

func testMockFirewallAction(t *testing.T, actionType, suffix string) {
	t.Helper()
	m := mockapi.New(t)
	resource.Test(t, testMockAction(m, actionType,
		fmt.Sprintf(`server_id = %d
    mac = %q`, mockapi.DefaultServerID, mockapi.DefaultMAC),
		testMockRequested(m, http.MethodPost, testMockServerPath("/interfaces/"+mockapi.DefaultMAC+"/firewall:"+suffix)),
	))
}

func TestMockActionFirewallReapply(t *testing.T) {
	testMockFirewallAction(t, "netcup_server_firewall_reapply", "reapply")
}

func TestMockActionFirewallRestore(t *testing.T) {
	testMockFirewallAction(t, "netcup_server_firewall_restore_copied_policies", "restore-copied-policies")
}

func TestMockActionFirewallValidatesMac(t *testing.T) {
	testCase := testMockAction(mockapi.New(t), "netcup_server_firewall_reapply",
		fmt.Sprintf(`server_id = %d
    mac = "nope"`, mockapi.DefaultServerID), nil)
	testCase.Steps[0].ExpectError = regexp.MustCompile("must be a MAC address")
	resource.Test(t, testCase)
}

func TestMockActionTaskCancelOfFinishedTask(t *testing.T) {
	m := mockapi.New(t)
	uuid := m.StartTask()
	m.FinishTask(uuid)
	resource.Test(t, testMockAction(m, "netcup_task_cancel",
		fmt.Sprintf(`uuid = %q`, uuid),
		testMockRequested(m, http.MethodPut, "/api/v1/tasks/"+uuid+":cancel"),
	))
}

func TestMockActionTaskCancel(t *testing.T) {
	m := mockapi.New(t)
	uuid := m.StartTask()
	resource.Test(t, testMockAction(m, "netcup_task_cancel",
		fmt.Sprintf(`uuid = %q`, uuid),
		testMockRequested(m, http.MethodPut, "/api/v1/tasks/"+uuid+":cancel"),
	))
}

func TestMockActionServerState(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockAction(m, "netcup_server_state",
		fmt.Sprintf(`server_id = %d
    state = "ON"
    state_option = "RESET"`, mockapi.DefaultServerID),
		func(*terraform.State) error {
			for _, r := range m.Requests() {
				if r.Method == http.MethodPatch {
					if r.Query.Get("stateOption") != "RESET" || !strings.Contains(r.Body, `"state":"ON"`) {
						return fmt.Errorf("request = %+v", r)
					}
					return nil
				}
			}
			return fmt.Errorf("no PATCH request")
		},
	))
}

func TestMockActionServerStateValidates(t *testing.T) {
	for _, tc := range []struct{ state, option, want string }{
		{"OFF", "RESET", "does not fit state OFF"},
		{"SUSPENDED", "RESET", "does not fit state SUSPENDED"},
		{"ON", "POWEROFF", "does not fit state ON"},
		{"HALF", "", "value must be one of"},
	} {
		t.Run(tc.state+"_"+tc.option, func(t *testing.T) {
			config := fmt.Sprintf("server_id = %d\n    state = %q", mockapi.DefaultServerID, tc.state)
			if tc.option != "" {
				config += fmt.Sprintf("\n    state_option = %q", tc.option)
			}
			testCase := testMockAction(mockapi.New(t), "netcup_server_state", config, nil)
			testCase.Steps[0].ExpectError = regexp.MustCompile(tc.want)
			resource.Test(t, testCase)
		})
	}
}

func TestMockActionFailedTaskFailsTheApply(t *testing.T) {
	m := mockapi.New(t)
	m.ShutOff()
	m.FailTasks("disk full")
	testCase := testMockAction(m, "netcup_server_disk_format",
		fmt.Sprintf(`server_id = %d
    disk_name = "vda"`, mockapi.DefaultServerID), nil)
	testCase.Steps[0].ExpectError = regexp.MustCompile("disk full")
	resource.Test(t, testCase)
}

func TestMockActionTimeoutEndsTheWait(t *testing.T) {
	m := mockapi.New(t)
	m.ShutOff()
	m.HoldTasks()
	testCase := testMockAction(m, "netcup_server_disk_format",
		fmt.Sprintf(`server_id = %d
    disk_name = "vda"
    timeouts = { invoke = "1s" }`, mockapi.DefaultServerID), nil)
	testCase.Steps[0].ExpectError = regexp.MustCompile("context deadline exceeded")
	resource.Test(t, testCase)
}

func testMockUserBodies(m *mockapi.Server) []string {
	var bodies []string
	for _, r := range m.Requests() {
		if r.Method == http.MethodPut && r.Path == fmt.Sprintf("/api/v1/users/%d", mockapi.UserID) {
			bodies = append(bodies, r.Body)
		}
	}
	return bodies
}

func testMockUserConfig(m *mockapi.Server, language, extra string) string {
	return testMockConfig(m, fmt.Sprintf(`
resource "netcup_user" "test" {
  language  = %q
  time_zone = "Europe/Vienna"
  %s
}
`, language, extra))
}

// testMockLastUserBody checks what the last PUT contained.
func testMockLastUserBody(m *mockapi.Server, wanted, unwanted []string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		bodies := testMockUserBodies(m)
		if len(bodies) == 0 {
			return fmt.Errorf("no PUT")
		}
		last := bodies[len(bodies)-1]
		for _, want := range wanted {
			if !strings.Contains(last, want) {
				return fmt.Errorf("body %s lacks %s", last, want)
			}
		}
		for _, not := range unwanted {
			if strings.Contains(last, not) {
				return fmt.Errorf("body %s has %s", last, not)
			}
		}
		return nil
	}
}

func testMockUserCase(steps ...resource.TestStep) resource.TestCase {
	return resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_11_0)},
		Steps:                    steps,
	}
}

func TestMockUser(t *testing.T) {
	m := mockapi.New(t)
	m.SetUserSettings("ACME", "192.0.2.1", true)
	check := func(name, value string) resource.TestCheckFunc {
		return resource.TestCheckResourceAttr("netcup_user.test", name, value)
	}
	resource.Test(t, testMockUserCase(
		resource.TestStep{
			// Unset settings keep what netcup has.
			Config: testMockUserConfig(m, "de", ""),
			Check: resource.ComposeAggregateTestCheckFunc(
				check("id", strconv.Itoa(mockapi.UserID)),
				check("username", "mock"),
				check("company", "ACME"),
				check("language", "de"),
				check("time_zone", "Europe/Vienna"),
				check("show_nickname", "true"),
				check("passwordless_mode", "true"),
				check("api_ip_login_restrictions", "192.0.2.1"),
				testMockLastUserBody(m, []string{
					`"showNickname":true`, `"passwordlessMode":true`, `"apiIpLoginRestrictions":"192.0.2.1"`, fmt.Sprintf(`"id":%d`, mockapi.UserID),
				}, []string{`"password"`, `"oldPassword"`}),
			),
		},
		resource.TestStep{
			ResourceName:            "netcup_user.test",
			ImportState:             true,
			ImportStateId:           strconv.Itoa(mockapi.UserID),
			ImportStateVerify:       true,
			ImportStateVerifyIgnore: []string{"timeouts"},
		},
		resource.TestStep{
			Config: testMockUserConfig(m, "en", `show_nickname = false
  passwordless_mode = false
  api_ip_login_restrictions = "192.0.2.2"`),
			Check: resource.ComposeAggregateTestCheckFunc(
				check("language", "en"),
				check("show_nickname", "false"),
				check("passwordless_mode", "false"),
				check("api_ip_login_restrictions", "192.0.2.2"),
			),
		},
		resource.TestStep{
			// Settings removed from the config stay.
			Config: testMockUserConfig(m, "de", ""),
			Check: resource.ComposeAggregateTestCheckFunc(
				check("show_nickname", "false"),
				check("api_ip_login_restrictions", "192.0.2.2"),
				testMockLastUserBody(m, []string{`"showNickname":false`, `"apiIpLoginRestrictions":"192.0.2.2"`}, nil),
			),
		},
		resource.TestStep{
			Config: testMockUserConfig(m, "de", `api_ip_login_restrictions = ""`),
			Check: resource.ComposeAggregateTestCheckFunc(
				check("api_ip_login_restrictions", ""),
				testMockLastUserBody(m, []string{`"apiIpLoginRestrictions":""`}, nil),
			),
		},
	))
}

func TestMockUserPasswords(t *testing.T) {
	m := mockapi.New(t)
	password := func(version int) string {
		return fmt.Sprintf(`password_wo = "NewSecret%[1]d"
  old_password_wo = "OldSecret%[1]d"
  password_wo_version = %[1]d`, version)
	}
	soap := func(version int) string {
		return fmt.Sprintf(`soap_webservice_password_wo = "SoapSecret%[1]d"
  soap_webservice_password_wo_version = %[1]d`, version)
	}
	resource.Test(t, testMockUserCase(
		resource.TestStep{
			Config: testMockUserConfig(m, "de", password(1)+"\n  "+soap(1)),
			Check: testMockLastUserBody(m, []string{
				`"password":"NewSecret1"`, `"oldPassword":"OldSecret1"`, `"soapWebservicePassword":"SoapSecret1"`,
			}, nil),
		},
		resource.TestStep{
			Config: testMockUserConfig(m, "en", password(1)+"\n  "+soap(1)),
			Check:  testMockLastUserBody(m, nil, []string{"Secret"}),
		},
		resource.TestStep{
			Config: testMockUserConfig(m, "en", password(2)+"\n  "+soap(1)),
			Check:  testMockLastUserBody(m, []string{`"password":"NewSecret2"`, `"oldPassword":"OldSecret2"`}, []string{"SoapSecret"}),
		},
		resource.TestStep{
			Config: testMockUserConfig(m, "en", password(2)+"\n  "+soap(2)),
			Check:  testMockLastUserBody(m, []string{`"soapWebservicePassword":"SoapSecret2"`}, []string{"NewSecret"}),
		},
		resource.TestStep{
			Config: testMockUserConfig(m, "de", ""),
			Check:  testMockLastUserBody(m, nil, []string{"Secret"}),
		},
	))
}

func TestMockUserValidates(t *testing.T) {
	for name, tc := range map[string]struct{ config, want string }{
		"password without version":      {`password_wo = "x"`, "password_wo_version"},
		"version without password":      {`password_wo_version = 1`, "password_wo"},
		"old password without password": {`old_password_wo = "x"`, "password_wo"},
		"soap without version":          {`soap_webservice_password_wo = "x"`, "soap_webservice_password_wo_version"},
		"soap version without password": {`soap_webservice_password_wo_version = 1`, "soap_webservice_password_wo"},
	} {
		t.Run(name, func(t *testing.T) {
			step := resource.TestStep{Config: testMockUserConfig(mockapi.New(t), "de", tc.config), ExpectError: regexp.MustCompile(tc.want)}
			resource.Test(t, testMockUserCase(step))
		})
	}
	step := resource.TestStep{Config: testMockUserConfig(mockapi.New(t), "english", ""), ExpectError: regexp.MustCompile("value must be one of")}
	resource.Test(t, testMockUserCase(step))
}

func TestMockUserOfAnotherTokenIsNotAdopted(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockUserCase(
		resource.TestStep{Config: testMockUserConfig(m, "de", "")},
		resource.TestStep{
			PreConfig:          func() { m.ChangeUserID(mockapi.UserID + 1) },
			RefreshState:       true,
			ExpectNonEmptyPlan: true,
		},
	))
}

func TestMockUserImportOfOtherUserFails(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: testMockConfig(m, `resource "netcup_user" "test" {
  language  = "de"
  time_zone = "Europe/Vienna"
}
`),
			ResourceName:  "netcup_user.test",
			ImportState:   true,
			ImportStateId: "1",
			ExpectError:   regexp.MustCompile("not the user of the refresh token"),
		}},
	})
}

func TestMockUserDataSources(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: testMockConfig(m, `
data "netcup_user" "test" {}

data "netcup_user_logs" "test" {
  limit = 1
}
`),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.netcup_user.test", "id", strconv.Itoa(mockapi.UserID)),
				resource.TestCheckResourceAttr("data.netcup_user.test", "email", "ada@example.com"),
				resource.TestCheckResourceAttr("data.netcup_user.test", "time_zone", "Europe/Berlin"),
				resource.TestCheckResourceAttr("data.netcup_user.test", "show_nickname", "true"),
				resource.TestCheckNoResourceAttr("data.netcup_user.test", "company"),
				resource.TestCheckResourceAttr("data.netcup_user_logs.test", "logs.#", "1"),
				resource.TestCheckResourceAttr("data.netcup_user_logs.test", "logs.0.log_key", "user.login"),
				resource.TestCheckResourceAttr("data.netcup_user_logs.test", "logs.0.executing_user.username", "mock"),
			),
		}},
	})
}

func testMockObjectConfig(m *mockapi.Server, typeName, source, hash string) string {
	hashLine := ""
	if hash != "" {
		hashLine = fmt.Sprintf("source_hash = %q", hash)
	}
	return testMockConfig(m, fmt.Sprintf(`
resource %q "test" {
  key    = "disk #1?.img"
  source = %q
  %s
}
`, typeName, source, hashLine))
}

func testMockObjectFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func testMockObjectLifecycle(t *testing.T, kind, typeName, dataSource string) {
	t.Helper()
	m := mockapi.New(t)
	address := typeName + ".test"
	small := testMockObjectFile(t, "small", "12345")
	big := testMockObjectFile(t, "big", "1234567890")
	bigElsewhere := testMockObjectFile(t, "elsewhere", "this content must not reach netcup")
	const key = "disk #1?.img"
	size := func(want string) resource.TestCheckFunc {
		return resource.TestCheckResourceAttr(address, "size_in_b", want)
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			if _, ok := m.Object(kind, key); ok {
				return fmt.Errorf("object survived the destroy")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: testMockObjectConfig(m, typeName, small, "a"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "id", key),
					size("5"),
					resource.TestCheckResourceAttr(address, "type", "MANUAL"),
					resource.TestCheckResourceAttrSet(address, "last_modified"),
				),
			},
			{
				ResourceName:            address,
				ImportState:             true,
				ImportStateId:           key,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"source", "source_hash", "timeouts"},
			},
			{
				Config: testMockObjectConfig(m, typeName, big, "b"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: size("10"),
			},
			{
				// A new path with the same hash uploads nothing.
				Config: testMockObjectConfig(m, typeName, bigElsewhere, "b"),
				Check:  size("10"),
			},
			{
				// Without a hash nothing tells of a change.
				Config: testMockObjectConfig(m, typeName, small, ""),
				Check:  size("10"),
			},
			{
				Config: testMockObjectConfig(m, typeName, small, "c"),
				Check:  size("5"),
			},
			{
				// The replacement deletes the object first, so a failed upload leaves none
				// and the next apply uploads again.
				PreConfig:   func() { m.FailParts(http.StatusForbidden, 1) },
				Config:      testMockObjectConfig(m, typeName, big, "d"),
				ExpectError: regexp.MustCompile("Error uploading"),
			},
			{
				Config: testMockObjectConfig(m, typeName, big, "d"),
				Check:  size("10"),
			},
			{
				PreConfig: func() { m.RemoveObject(kind, key) },
				Config:    testMockObjectConfig(m, typeName, small, ""),
				Check:     size("5"),
			},
			{
				Config: testMockObjectConfig(m, typeName, small, "") + fmt.Sprintf(`
data %q "test" {
  depends_on = [%s]
}
`, dataSource, address),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data."+dataSource+".test", "objects.#", "1"),
					resource.TestCheckResourceAttr("data."+dataSource+".test", "objects.0.key", key),
					resource.TestCheckResourceAttr("data."+dataSource+".test", "objects.0.size_in_b", "5"),
					resource.TestCheckResourceAttr("data."+dataSource+".test", "objects.0.type", "MANUAL"),
				),
			},
		},
	})
}

func TestMockUserImage(t *testing.T) {
	testMockObjectLifecycle(t, "images", "netcup_user_image", "netcup_user_images")
}

func TestMockUserIso(t *testing.T) {
	testMockObjectLifecycle(t, "isos", "netcup_user_iso", "netcup_user_isos")
}

// netcup refuses an upload to an existing key, so the provider never overwrites
// an object it does not manage.
func TestMockUserObjectExistingKeyFails(t *testing.T) {
	m := mockapi.New(t)
	source := testMockObjectFile(t, "file", "1")
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			PreConfig: func() {
				client := scp.NewForServer(m.URL, mockapi.RefreshToken)
				if err := client.UploadObject(context.Background(), scp.ObjectKindIsos, "disk #1?.img", source); err != nil {
					t.Fatal(err)
				}
			},
			Config:      testMockObjectConfig(m, "netcup_user_iso", source, ""),
			ExpectError: regexp.MustCompile("already exists"),
		}},
	})
}

func TestMockUserObjectMissingFileFailsTheApply(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      testMockObjectConfig(m, "netcup_user_iso", filepath.Join(t.TempDir(), "none"), ""),
			ExpectError: regexp.MustCompile("Error uploading ISO"),
		}},
	})
}

func testMockObjectDownload(t *testing.T, kind, typeName string) {
	t.Helper()
	m := mockapi.New(t)
	source := testMockObjectFile(t, "file", "1")
	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_10_0)},
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"netcup": testAccProtoV6ProviderFactories["netcup"],
			"echo":   echoprovider.NewProviderServer(),
		},
		Steps: []resource.TestStep{{
			PreConfig: func() {
				client := scp.NewForServer(m.URL, mockapi.RefreshToken)
				if err := client.UploadObject(context.Background(), scp.ObjectKind(kind), "file.bin", source); err != nil {
					t.Fatal(err)
				}
			},
			Config: testMockConfig(m, fmt.Sprintf(`
ephemeral %[1]q "test" {
  key = "file.bin"
}

provider "echo" {
  data = ephemeral.%[1]s.test
}

resource "echo" "test" {}
`, typeName)),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("filename"), knownvalue.StringExact("file.bin")),
				statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("presigned_url"), knownvalue.StringRegexp(regexp.MustCompile("signature=mock"))),
				statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("presigned_url_validity_duration_in_hours"), knownvalue.Int64Exact(1)),
				statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("headers").AtMapKey("x-amz-te"), knownvalue.ListExact([]knownvalue.Check{knownvalue.StringExact("trailers")})),
			},
		}},
	})
}

func TestMockUserImageDownload(t *testing.T) {
	testMockObjectDownload(t, "images", "netcup_user_image_download")
}

func TestMockUserIsoDownload(t *testing.T) {
	testMockObjectDownload(t, "isos", "netcup_user_iso_download")
}

func testMockCCPConfig(m *mockapi.Server, password, config string) string {
	return fmt.Sprintf(`
provider "netcup" {
  customer_number = %q
  api_key         = %q
  api_password    = %q
  ccp_endpoint    = %q
}
`, mockapi.CCPCustomer, mockapi.CCPKey, password, m.URL+mockapi.CCPPath+"?JSON") + config
}

func testMockDNSRecord(hostname, kind, priority, destination string) string {
	priorityLine := ""
	if priority != "" {
		priorityLine = fmt.Sprintf("priority    = %q", priority)
	}
	return fmt.Sprintf(`
resource "netcup_dns_record" "test" {
  domain      = %q
  hostname    = %q
  type        = %q
  %s
  destination = %q
}
`, mockapi.CCPDomain, hostname, kind, priorityLine, destination)
}

func TestMockDNSRecord(t *testing.T) {
	m := mockapi.New(t)
	address := "netcup_dns_record.test"
	var id string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(*terraform.State) error {
			if n := m.CCPRecordCount(); n != 0 {
				return fmt.Errorf("%d records survived the destroy", n)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: testMockCCPConfig(m, mockapi.CCPPassword, testMockDNSRecord("@", "MX", "10", "mail.example.com")),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "hostname", "@"),
					resource.TestCheckResourceAttr(address, "priority", "10"),
					resource.TestCheckResourceAttrSet(address, "id"),
					func(s *terraform.State) error {
						id = s.RootModule().Resources[address].Primary.ID
						return nil
					},
				),
			},
			{
				ResourceName:      address,
				ImportState:       true,
				ImportStateIdFunc: func(*terraform.State) (string, error) { return mockapi.CCPDomain + ":" + id, nil },
				ImportStateVerify: true,
			},
			{
				Config: testMockCCPConfig(m, mockapi.CCPPassword, testMockDNSRecord("@", "MX", "20", "mail2.example.com")),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(address, "priority", "20"),
					resource.TestCheckResourceAttr(address, "destination", "mail2.example.com"),
					func(s *terraform.State) error {
						if got := s.RootModule().Resources[address].Primary.ID; got != id {
							return fmt.Errorf("id changed from %s to %s", id, got)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestMockDNSRecordRemovedOutsideOfTerraform(t *testing.T) {
	m := mockapi.New(t)
	config := testMockCCPConfig(m, mockapi.CCPPassword, testMockDNSRecord("www", "A", "", "192.0.2.1"))
	var id string
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					id = s.RootModule().Resources["netcup_dns_record.test"].Primary.ID
					return nil
				},
			},
			{
				PreConfig: func() { m.RemoveCCPRecord(id) },
				Config:    config,
				Check: func(s *terraform.State) error {
					got := s.RootModule().Resources["netcup_dns_record.test"].Primary.ID
					if got == id || m.CCPRecordCount() != 1 {
						return fmt.Errorf("id %s, was %s, %d records", got, id, m.CCPRecordCount())
					}
					return nil
				},
			},
		},
	})
}

func TestMockDNSZone(t *testing.T) {
	m := mockapi.New(t)
	config := func(ttl int) string {
		return testMockCCPConfig(m, mockapi.CCPPassword, fmt.Sprintf(`
resource "netcup_dns_zone" "test" {
  domain  = %q
  ttl     = %d
  refresh = 28800
  retry   = 7200
  expire  = 1209600
}
`, mockapi.CCPDomain, ttl))
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config(3600),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("netcup_dns_zone.test", "ttl", "3600"),
					resource.TestCheckResourceAttr("netcup_dns_zone.test", "dnssec_status", "false"),
					resource.TestCheckResourceAttrSet("netcup_dns_zone.test", "serial"),
				),
			},
			{
				ResourceName:      "netcup_dns_zone.test",
				ImportState:       true,
				ImportStateId:     mockapi.CCPDomain,
				ImportStateVerify: true,
			},
			{
				Config: config(7200),
				Check:  resource.TestCheckResourceAttr("netcup_dns_zone.test", "ttl", "7200"),
			},
		},
	})
}

func TestMockDNSRecordsDataSource(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: testMockCCPConfig(m, mockapi.CCPPassword, testMockDNSRecord("www", "A", "", "192.0.2.1")+fmt.Sprintf(`
data "netcup_dns_records" "test" {
  domain     = %q
  depends_on = [netcup_dns_record.test]
}
`, mockapi.CCPDomain)),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.netcup_dns_records.test", "records.#", "1"),
				resource.TestCheckResourceAttr("data.netcup_dns_records.test", "records.0.hostname", "www"),
				resource.TestCheckResourceAttr("data.netcup_dns_records.test", "records.0.destination", "192.0.2.1"),
			),
		}},
	})
}

func TestMockDNSRecordsDataSourceOfEmptyZone(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: testMockCCPConfig(m, mockapi.CCPPassword, fmt.Sprintf(`
data "netcup_dns_records" "test" {
  domain = %q
}
`, mockapi.CCPDomain)),
			Check: resource.TestCheckResourceAttr("data.netcup_dns_records.test", "records.#", "0"),
		}},
	})
}

func TestMockCCPWrongPasswordFails(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      testMockCCPConfig(m, "wrong", testMockDNSRecord("www", "A", "", "192.0.2.1")),
			ExpectError: regexp.MustCompile("Validation Error"),
		}},
	})
}

func TestMockServerDataSourceFields(t *testing.T) {
	m := mockapi.New(t)
	m.EnableGPU()
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, fmt.Sprintf(`
data "netcup_server" "live" {
  id = %[1]d
}

data "netcup_server" "plain" {
  id                    = %[1]d
  load_server_live_info = false
}
`, mockapi.DefaultServerID)),
		Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("data.netcup_server.live", "export_count", strconv.Itoa(mockapi.DefaultExportCount)),
			resource.TestCheckResourceAttr("data.netcup_server.live", "template.name", "VPS mock"),
			resource.TestCheckResourceAttr("data.netcup_server.live", "site.city", "Nuremberg"),
			resource.TestCheckResourceAttr("data.netcup_server.live", "ipv4_addresses.0.ip", "192.0.2.10"),
			resource.TestCheckNoResourceAttr("data.netcup_server.live", "ipv4_addresses.0.broadcast"),
			resource.TestCheckResourceAttr("data.netcup_server.live", "ipv6_addresses.0.network_prefix_length", "64"),
			resource.TestCheckResourceAttr("data.netcup_server.live", "disks_available_space_in_mib", "1024"),
			resource.TestCheckResourceAttr("data.netcup_server.live", "snapshot_allowed", "true"),
			resource.TestCheckResourceAttr("data.netcup_server.live", "gpu_driver_available", "true"),
			resource.TestCheckResourceAttr("data.netcup_server.live", "server_live_info.state", "RUNNING"),
			resource.TestCheckResourceAttr("data.netcup_server.live", "server_live_info.keyboard_layout", "de"),
			resource.TestCheckNoResourceAttr("data.netcup_server.plain", "server_live_info.state"),
			resource.TestCheckResourceAttr("data.netcup_server.plain", "template.name", "VPS mock"),
		),
	}))
}

func TestMockServersTemplate(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, `data "netcup_servers" "test" {}`),
		Check:  resource.TestCheckResourceAttr("data.netcup_servers.test", "servers.0.template.name", "VPS mock"),
	}))
}

func TestMockServerLiveInfoFields(t *testing.T) {
	m := mockapi.New(t)
	check := func(name, value string) resource.TestCheckFunc {
		return resource.TestCheckResourceAttr("data.netcup_server.test", name, value)
	}
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, fmt.Sprintf(`
data "netcup_server" "test" {
  id = %d
}
`, mockapi.DefaultServerID)),
		Check: resource.ComposeAggregateTestCheckFunc(
			check("template.id", "1"), check("site.id", "2"), check("site.name", "NUE"),
			check("server_live_info.disks.0.dev", "vda"),
			check("server_live_info.disks.0.capacity_in_mib", "2048"),
			check("server_live_info.disks.0.allocation_in_mib", "1024"),
			check("server_live_info.required_storage_optimization", "NO"),
			check("server_live_info.template", "VPS mock"),
			check("server_live_info.uptime_in_seconds", "60"),
			check("server_live_info.current_server_memory_in_mib", "1024"),
			check("server_live_info.max_server_memory_in_mib", "2048"),
			check("server_live_info.cpu_count", "2"),
			check("server_live_info.cpu_max_count", "4"),
			check("server_live_info.latest_qemu", "true"),
			check("server_live_info.config_changed", "true"),
			check("server_live_info.nested_guest", "true"),
			check("server_live_info.machine_type", "q35"),
			check("server_live_info.cloudinit_attached", "true"),
		),
	}))
}

func TestMockLookupDataSources(t *testing.T) {
	m := mockapi.New(t)
	m.AddSnapshot("s")
	m.MarkSnapshotExported("s", 42)
	m.SetAttachedIso("install.iso")
	uuid := m.StartTask()
	task := func(name, value string) resource.TestCheckFunc {
		return resource.TestCheckResourceAttr("data.netcup_task.test", name, value)
	}
	snapshot := func(name, value string) resource.TestCheckFunc {
		return resource.TestCheckResourceAttr("data.netcup_server_snapshot.test", name, value)
	}
	resource.Test(t, testMockCase(resource.TestStep{
		PreConfig: func() {
			client := scp.NewForServer(m.URL, mockapi.RefreshToken)
			for ip, host := range map[string]string{"192.0.2.10": "v4.example.com", "2001:db8::1": "v6.example.com"} {
				if err := client.SetReverseDNS(context.Background(), ip, host); err != nil {
					t.Fatal(err)
				}
			}
		},
		Config: testMockConfig(m, fmt.Sprintf(`
data "netcup_server_snapshot" "test" {
  server_id = %[1]d
  name      = "s"
}

data "netcup_server_iso" "test" {
  server_id = %[1]d
}

data "netcup_reverse_dns" "v4" {
  ip = "192.0.2.10"
}

data "netcup_reverse_dns" "v6" {
  ip = "2001:db8::1"
}

data "netcup_reverse_dns" "none" {
  ip = "192.0.2.99"
}

data "netcup_task" "test" {
  uuid = %[2]q
}

data "netcup_maintenance" "test" {}
`, mockapi.DefaultServerID, uuid)),
		Check: resource.ComposeAggregateTestCheckFunc(
			snapshot("exported_size_in_kib", "42"), snapshot("exported", "true"), snapshot("uuid", "snap-s"),
			snapshot("disks.0", "vda"), snapshot("state", "SHUTOFF"), snapshot("online", "false"),
			resource.TestCheckResourceAttrSet("data.netcup_server_snapshot.test", "creation_time"),
			resource.TestCheckNoResourceAttr("data.netcup_server_snapshot.test", "description"),
			resource.TestCheckResourceAttr("data.netcup_server_iso.test", "iso_attached", "true"),
			resource.TestCheckResourceAttr("data.netcup_server_iso.test", "iso", "install.iso"),
			resource.TestCheckResourceAttr("data.netcup_reverse_dns.v4", "rdns", "v4.example.com"),
			resource.TestCheckResourceAttr("data.netcup_reverse_dns.v6", "rdns", "v6.example.com"),
			resource.TestCheckNoResourceAttr("data.netcup_reverse_dns.none", "rdns"),
			task("name", "ServerMockTask"), task("state", "FINISHED"),
			task("started_at", "2026-11-06T10:00:00Z"), task("finished_at", "2026-11-06T10:00:00Z"),
			task("executing_user.username", "mock"),
			task("task_progress.progress_in_percent", "50"),
			task("task_progress.expected_finished_at", "2026-11-06T10:00:00Z"),
			task("message", "mock message"), task("on_rollback", "true"),
			task("steps.0.name", "first"), task("steps.0.uuid", "step-1"), task("steps.0.state", "FINISHED"),
			task("result", `{"ok":true}`),
			resource.TestCheckResourceAttrSet("data.netcup_maintenance.test", "%"),
		),
	}))
}

func TestMockTaskResponseError(t *testing.T) {
	m := mockapi.New(t)
	uuid := m.StartTask()
	m.FailTasks("disk full")
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, fmt.Sprintf(`
data "netcup_task" "test" {
  uuid = %q
}
`, uuid)),
		Check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("data.netcup_task.test", "state", "ERROR"),
			resource.TestCheckResourceAttr("data.netcup_task.test", "response_error.message", "disk full"),
		),
	}))
}

func TestMockServerFirewallDataSource(t *testing.T) {
	m := mockapi.New(t)
	config := func(check string) string {
		return testMockConfig(m, fmt.Sprintf(`
resource "netcup_firewall_policy" "test" {
  name = "web"

  rule {
    direction         = "INGRESS"
    protocol          = "TCP"
    action            = "ACCEPT"
    destination_ports = "443"
  }
}

resource "netcup_server_firewall" "test" {
  server_id  = %[1]d
  mac        = %[2]q
  policy_ids = [netcup_firewall_policy.test.id]
}

data "netcup_server_firewall" "test" {
  server_id  = %[1]d
  mac        = %[2]q
  %[3]s
  depends_on = [netcup_server_firewall.test]
}
`, mockapi.DefaultServerID, mockapi.DefaultMAC, check))
	}
	address := "data.netcup_server_firewall.test"
	resource.Test(t, testMockCase(
		resource.TestStep{
			Config: config(""),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckNoResourceAttr(address, "consistent"),
				resource.TestCheckResourceAttr(address, "active", "true"),
				resource.TestCheckResourceAttr(address, "ingress_implicit_rule", "DROP_ALL"),
				resource.TestCheckResourceAttr(address, "egress_implicit_rule", "ACCEPT_ALL"),
				resource.TestCheckResourceAttr(address, "copied_policies.#", "0"),
				resource.TestCheckResourceAttr(address, "user_policies.0.name", "web"),
				resource.TestCheckResourceAttr(address, "user_policies.0.rules.0.protocol", "TCP"),
				resource.TestCheckResourceAttr(address, "user_policies.0.rules.0.destination_ports", "443"),
			),
		},
		resource.TestStep{
			Config: config("consistency_check = true"),
			Check:  resource.TestCheckResourceAttr(address, "consistent", "true"),
		},
	))
}

func TestMockFailoverRouteTimeoutRoutesNothing(t *testing.T) {
	m := mockapi.New(t)
	config := func(extra string) string {
		return testMockConfig(m, fmt.Sprintf(`
resource "netcup_failover_ip_route" "test" {
  failover_ip_id = %d
  server_id      = %d
  %s
}
`, mockapi.FailoverV4ID, mockapi.DefaultServerID, extra))
	}
	routes := func() int {
		count := 0
		for _, r := range m.Requests() {
			if r.Method == http.MethodPatch {
				count++
			}
		}
		return count
	}
	before := 0
	resource.Test(t, testMockCase(
		resource.TestStep{
			Config: config(""),
			Check: func(*terraform.State) error {
				before = routes()
				return nil
			},
		},
		resource.TestStep{
			Config: config(`timeouts = { create = "1m", update = "1m" }`),
			Check: func(*terraform.State) error {
				if got := routes(); got != before {
					return fmt.Errorf("%d routing requests, want %d", got, before)
				}
				return nil
			},
		},
	))
}

func TestMockFailoverRouteTimeoutEndsTheWait(t *testing.T) {
	m := mockapi.New(t)
	m.HoldTasks()
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, fmt.Sprintf(`
resource "netcup_failover_ip_route" "test" {
  failover_ip_id = %d
  server_id      = %d
  timeouts       = { create = "1s" }
}
`, mockapi.FailoverV4ID, mockapi.DefaultServerID)),
		ExpectError: regexp.MustCompile("context deadline exceeded"),
	}))
}

func TestMockSnapshotNotFoundFails(t *testing.T) {
	m := mockapi.New(t)
	resource.Test(t, testMockCase(resource.TestStep{
		Config: testMockConfig(m, fmt.Sprintf(`
data "netcup_server_snapshot" "test" {
  server_id = %d
  name      = "none"
}
`, mockapi.DefaultServerID)),
		ExpectError: regexp.MustCompile("Snapshot not found"),
	}))
}

func TestMockServerSnapshotDownload(t *testing.T) {
	m := mockapi.New(t)
	m.AddSnapshot("s")
	m.MarkSnapshotExported("s", 42)
	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_10_0)},
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"netcup": testAccProtoV6ProviderFactories["netcup"],
			"echo":   echoprovider.NewProviderServer(),
		},
		Steps: []resource.TestStep{{
			Config: testMockConfig(m, fmt.Sprintf(`
ephemeral "netcup_server_snapshot_download" "test" {
  server_id = %d
  name      = "s"
}

provider "echo" {
  data = ephemeral.netcup_server_snapshot_download.test
}

resource "echo" "test" {}
`, mockapi.DefaultServerID)),
			ConfigStateChecks: []statecheck.StateCheck{
				statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("filename"), knownvalue.StringExact("s.qcow2")),
				statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("presigned_url_validity_duration_in_hours"), knownvalue.Int64Exact(1)),
				statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("presigned_url"), knownvalue.StringRegexp(regexp.MustCompile("signature=mock"))),
			},
		}},
	})
}

func TestMockServerSnapshotDownloadWithoutExportFails(t *testing.T) {
	m := mockapi.New(t)
	m.AddSnapshot("s")
	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{tfversion.SkipBelow(tfversion.Version1_10_0)},
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"netcup": testAccProtoV6ProviderFactories["netcup"],
			"echo":   echoprovider.NewProviderServer(),
		},
		Steps: []resource.TestStep{{
			Config: testMockConfig(m, fmt.Sprintf(`
ephemeral "netcup_server_snapshot_download" "test" {
  server_id = %d
  name      = "s"
}

provider "echo" {
  data = ephemeral.netcup_server_snapshot_download.test
}

resource "echo" "test" {}
`, mockapi.DefaultServerID)),
			ExpectError: regexp.MustCompile("no exported snapshot"),
		}},
	})
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
