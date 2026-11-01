# Terraform Provider for netcup

[![Release](https://img.shields.io/github/v/release/hornc-greedy/terraform-provider-netcup?sort=semver)](https://github.com/hornc-greedy/terraform-provider-netcup/releases)
[![Terraform Registry](https://img.shields.io/badge/Terraform%20Registry-844FBA?logo=terraform&logoColor=white)](https://registry.terraform.io/providers/hornc-greedy/netcup/latest)
[![Tests](https://github.com/hornc-greedy/terraform-provider-netcup/actions/workflows/test.yml/badge.svg)](https://github.com/hornc-greedy/terraform-provider-netcup/actions/workflows/test.yml)
[![License: MPL-2.0](https://img.shields.io/github/license/hornc-greedy/terraform-provider-netcup)](LICENSE)

A [Terraform](https://www.terraform.io) provider for [netcup](https://www.netcup.com/),
built on the [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework).

It covers all three netcup APIs, so one provider manages domains on CCP DNS,
domains on CloudDNS and servers:

- **CCP API** (DomainWebservice, JSON) for CCP DNS: records and zones
- **netcup API** (bearer API key) for CloudDNS: records, revisions and ACME
  challenges
- **SCP REST API** (OAuth 2.0) for servers and the objects around them, see
  [Objects](#objects)

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
  - ephemeral resources need 1.10
- [Go](https://go.dev/doc/install) >= 1.26 (to build the provider)

## Using the Provider

```hcl
terraform {
  required_providers {
    netcup = {
      source  = "hornc-greedy/netcup"
      version = "~> 1.1"
    }
  }
}

provider "netcup" {
  # CCP API, only needed for the netcup_dns_* objects.
  customer_number = var.netcup_customer_number
  api_key         = var.netcup_api_key
  api_password    = var.netcup_api_password

  # SCP REST API, only needed for servers and the other SCP objects.
  scp_refresh_token = var.netcup_scp_refresh_token

  # netcup API, only needed for the netcup_clouddns_* objects.
  clouddns_api_key = var.netcup_clouddns_api_key
}

resource "netcup_dns_record" "www" {
  domain      = "example.com"
  hostname    = "www"
  type        = "A"
  destination = "203.0.113.10"
}

resource "netcup_reverse_dns" "mail" {
  ip       = "203.0.113.10"
  hostname = "mail.example.com"
}
```

Declare these variables with `sensitive = true` and, with Terraform 1.10 or
newer, `ephemeral = true`, which keeps them out of plan files. The APIs use
**independent credentials**, so configure only what you need. The credentials
can also come from the environment: `NETCUP_CUSTOMER_NUMBER`, `NETCUP_API_KEY`
and `NETCUP_API_PASSWORD` for the CCP API, `NETCUP_SCP_REFRESH_TOKEN` for the SCP
REST API and `NETCUP_CLOUDDNS_API_KEY` for the netcup API.

Every resource supports `terraform import` and, with Terraform 1.5 or newer,
`import` blocks. The
[documentation on the Terraform Registry](https://registry.terraform.io/providers/hornc-greedy/netcup/latest/docs)
lists every object, and for each resource its import ID.

### Authentication

Create the CCP API key and API password in the CCP under **Master Data > API**.
The API password is not your login password. The API key of the netcup API for
CloudDNS is created on the same page.

The SCP REST API needs an offline refresh token, obtained once via the OIDC
device flow:

```bash
# 1. Request a device code, then open verification_uri_complete in a browser:
curl -s -X POST 'https://www.servercontrolpanel.de/realms/scp/protocol/openid-connect/auth/device' \
  -d 'client_id=scp' -d 'scope=offline_access openid'

# 2. After approving in the browser, exchange the device_code for tokens:
curl -s -X POST 'https://www.servercontrolpanel.de/realms/scp/protocol/openid-connect/token' \
  -d 'grant_type=urn:ietf:params:oauth:grant-type:device_code' \
  -d 'device_code=<device-code>' -d 'client_id=scp'
```

The refresh token stays valid as long as it is used at least once every 30 days.

### Objects

| Category | Resources | Data sources | Ephemeral | Actions |
|---|---|---|---|---|
| CCP DNS | `netcup_dns_record`, `netcup_dns_zone` | `netcup_dns_records` |  |  |
| CloudDNS | `netcup_clouddns_record`, `netcup_clouddns_acme_challenge` | `netcup_clouddns_record`, `netcup_clouddns_domain`, `netcup_clouddns_domains`, `netcup_clouddns_records`, `netcup_clouddns_revision`, `netcup_clouddns_revisions`, `netcup_clouddns_record_types`, `netcup_clouddns_acme_challenges` |  |  |
| Servers | `netcup_server_settings` | `netcup_servers`, `netcup_server`, `netcup_server_guest_agent`, `netcup_server_guest_agent_status` |  |  |
| Disks | `netcup_server_disk_driver` | `netcup_server_disks`, `netcup_server_disk`, `netcup_server_disk_supported_drivers` |  |  |
| Images, ISO and rescue | `netcup_server_iso`, `netcup_server_rescue_system` | `netcup_server_image_flavours`, `netcup_server_iso_images` | `netcup_server_rescue_system`, `netcup_server_gpu_driver` |  |
| Interfaces and reverse DNS | `netcup_server_interface`, `netcup_reverse_dns` | `netcup_server_interface`, `netcup_server_interfaces` |  |  |
| Metrics and logs |  | `netcup_server_metrics_cpu`, `netcup_server_metrics_disk`, `netcup_server_metrics_network`, `netcup_server_metrics_network_packet`, `netcup_server_logs` |  |  |
| Snapshots | `netcup_server_snapshot` | `netcup_server_snapshots`, `netcup_server_snapshot_dryrun` |  |  |
| Firewall | `netcup_firewall_policy`, `netcup_server_firewall` | `netcup_firewall_policies` |  |  |
| Failover IPs | `netcup_failover_ip_route` | `netcup_failover_ips` |  |  |
| VLANs | `netcup_user_vlan` | `netcup_user_vlan`, `netcup_vlan`, `netcup_user_vlans` |  |  |
| User and SSH keys | `netcup_ssh_key` | `netcup_ssh_keys` |  |  |
| Tasks and maintenance |  | `netcup_tasks`, `netcup_maintenance` (deprecated, removed in 2.0.0) |  |  |

## Notes and limitations

- **TTL** is a zone setting at netcup (`updateDnsZone`), managed with
  `netcup_dns_zone` rather than per record.
- netcup may normalize values such as trailing dots of `CNAME` and `MX` targets
  or `TXT` quoting. If a diff persists, write the value exactly as netcup stores
  it.
- **Reverse DNS** runs over the SCP REST API, not a DNS API, and the IP must
  belong to one of your servers.
- **Domain registration and contacts** need reseller-only CCP functions and are
  not implemented.
- A domain has either the **DNS** tab in the CCP and uses the `netcup_dns_*`
  objects, or the **CloudDNS** tab and uses the `netcup_clouddns_*` objects.
- These assumptions of the `netcup_clouddns_*` objects are not in the
  specification:
  - netcup marks the served revision with the state `active`.
  - A changeset with `strict` applies `delete` before `create`, so a TTL change
    can send the same record in both lists.
  - If netcup stores `rdata` in another form, apply fails with a message naming
    it.

## Developing the Provider

Building needs [Go](https://go.dev/doc/install), linting
[golangci-lint](https://golangci-lint.run/) v2 (CI uses v2.12.2) with the config
in `.golangci.yml`.

```shell
make build      # compile
make test       # unit tests
make lint       # golangci-lint
make generate   # regenerate docs/
```

The docs under `docs/` are published to the Terraform Registry. They are
generated from the schema descriptions and `examples/` with
[tfplugindocs](https://github.com/hashicorp/terraform-plugin-docs), so run
`make generate` after changing a schema or an example.

### Tests

Acceptance tests run against a mock of the netcup APIs in `internal/mockapi`. It
keeps state, checks SCP and netcup API requests and responses against the OpenAPI
specifications in `internal/mockapi/spec` and needs no credentials.

```shell
make testmock
```

A mock proves that the provider follows the specification, not that netcup does.
Live tests against the real API therefore exist as well and create **real**
resources.

```shell
make testlive
```

**Never point these tests at a production server.** Use a netcup account of its
own if you can, because its token cannot reach your other servers. Every test
with the token, except the server list, fails unless the nickname of every server
of the account starts with `netcup-provider-live-test`. Set these nicknames
yourself.

The tests ignore your normal `NETCUP_*` credentials and only read `NETCUP_TEST_*`
variables. Each test is skipped unless all of its variables are set.

| Variables | Tests | Effect |
|---|---|---|
| `NETCUP_TEST_SCP_REFRESH_TOKEN` | SSH keys, firewall policies | they are created and deleted |
| the token | server list | none, it only reads |
| the token and `NETCUP_TEST_SERVER_ID` | server | an offline snapshot of `vda` restarts the server, policies are assigned to the first interface and removed again, nickname and autostart are set |
| the token, the server, `NETCUP_TEST_RDNS_IP`, `NETCUP_TEST_RDNS_DOMAIN` | reverse DNS | the PTR of the given IP, which must belong to the server, is set, changed and deleted |
| the token, the server, `NETCUP_TEST_FAILOVER_IP_ID` | failover route | the unrouted IPv4 failover IP stays routed to the server |
| `NETCUP_TEST_CLOUDDNS_API_KEY` | CloudDNS data sources | none, they only read |
| `NETCUP_TEST_CLOUDDNS_API_KEY`, `NETCUP_TEST_CLOUDDNS_DOMAIN` | CloudDNS records, ACME challenges | a record is created, its TTL changed and it is deleted, a challenge is created and deleted |
| `NETCUP_TEST_CUSTOMER_NUMBER`, `NETCUP_TEST_API_KEY`, `NETCUP_TEST_API_PASSWORD`, `NETCUP_TEST_DOMAIN` | CCP DNS | a record is created, changed and deleted, the zone TTL is set to 3600, then to 86400 and stays there |

### Trying a local build

To try a local build against the real API, run `go install .` and point
Terraform at your `GOBIN`, by default `~/go/bin`, with a `dev_overrides` block in
`~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "hornc-greedy/netcup" = "<GOBIN>"
  }
  direct {}
}
```

## License

[MPL-2.0](LICENSE)
