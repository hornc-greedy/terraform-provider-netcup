// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/echoprovider"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

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
