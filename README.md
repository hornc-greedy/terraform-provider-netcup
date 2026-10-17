# Terraform Provider for netcup

[![Release](https://img.shields.io/github/v/release/hornc-greedy/terraform-provider-netcup?sort=semver)](https://github.com/hornc-greedy/terraform-provider-netcup/releases)
[![Terraform Registry](https://img.shields.io/badge/Terraform%20Registry-844FBA?logo=terraform&logoColor=white)](https://registry.terraform.io/providers/hornc-greedy/netcup/latest)
[![Tests](https://github.com/hornc-greedy/terraform-provider-netcup/actions/workflows/test.yml/badge.svg)](https://github.com/hornc-greedy/terraform-provider-netcup/actions/workflows/test.yml)
[![License: MPL-2.0](https://img.shields.io/github/license/hornc-greedy/terraform-provider-netcup)](LICENSE)

A [Terraform](https://www.terraform.io) provider for [netcup](https://www.netcup.com/),
built on the [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework).

It manages two independent netcup APIs:

- **CCP / DomainWebservice** (JSON) — DNS records and zones
- **SCP REST API** (OAuth 2.0) — reverse DNS, SSH keys, firewall policies,
  snapshots and failover routing for your servers

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://go.dev/doc/install) >= 1.26 (to build the provider)

## Using the Provider

```hcl
terraform {
  required_providers {
    netcup = {
      source = "hornc-greedy/netcup"
    }
  }
}

provider "netcup" {
  # CCP API (DNS) — only needed for DNS resources and data sources.
  customer_number = var.netcup_customer_number
  api_key         = var.netcup_api_key
  api_password    = var.netcup_api_password

  # SCP REST API — only needed for reverse DNS and server resources.
  scp_refresh_token = var.netcup_scp_refresh_token
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

The two APIs use **independent credentials** — configure only what you need.
Credentials can also be supplied via the environment:
`NETCUP_CUSTOMER_NUMBER`, `NETCUP_API_KEY`, `NETCUP_API_PASSWORD` (CCP) and
`NETCUP_SCP_REFRESH_TOKEN` (SCP).

Generate the CCP API key and password in the netcup CCP under
**Stammdaten → Webservice / API** (the API password is separate from your login
password).

Full documentation for every resource and data source is published on the
[Terraform Registry](https://registry.terraform.io/providers/hornc-greedy/netcup/latest/docs).

### Resources and data sources

| Type | Names |
|---|---|
| DNS (CCP) | `netcup_dns_record`, `netcup_dns_zone`, `netcup_dns_records` |
| Reverse DNS (SCP) | `netcup_reverse_dns` |
| Servers (SCP) | `netcup_servers`, `netcup_server`, `netcup_server_interfaces` |
| SSH keys (SCP) | `netcup_ssh_key`, `netcup_ssh_keys` |
| Firewall (SCP) | `netcup_firewall_policy`, `netcup_firewall_policies`, `netcup_server_firewall` |
| Snapshots / failover (SCP) | `netcup_server_snapshot`, `netcup_failover_ip_route`, `netcup_failover_ips` |
| Misc (SCP) | `netcup_maintenance` (deprecated, removed in 2.0.0) |

### SCP refresh token

The SCP REST API uses OAuth 2.0. Obtain an offline refresh token once via the
OIDC device flow and pass it as `scp_refresh_token`:

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

### Import

```bash
terraform import netcup_dns_record.www            example.com:123456789
terraform import netcup_dns_zone.example          example.com
terraform import netcup_reverse_dns.mail          203.0.113.10
terraform import netcup_ssh_key.deploy            42
terraform import netcup_firewall_policy.web       174780
terraform import netcup_server_snapshot.daily     296155/daily
terraform import netcup_server_firewall.srv       296155/ba:ad:7a:1f:d3:68
terraform import netcup_failover_ip_route.primary v4/1234
```

## Developing the Provider

To build and test locally you need [Go](https://go.dev/doc/install) >= 1.26.

```shell
go build ./...   # compile
go test ./...    # unit tests (no network, httptest-based)
go vet ./...
```

Linting uses [golangci-lint](https://golangci-lint.run/) (config in
`.golangci.yml`):

```shell
golangci-lint run ./...
```

The per-resource docs under `docs/` (published to the Terraform Registry) are
generated from the schema descriptions and the files in `examples/` with
[tfplugindocs](https://github.com/hashicorp/terraform-plugin-docs); regenerate
them after changing a schema or an example:

```shell
go generate ./...   # or: make generate
```

Acceptance tests create **real** resources and are gated behind `TF_ACC=1` and
the `NETCUP_*` environment variables. Run them only against a dedicated test
account or domain:

```shell
TF_ACC=1 go test ./... -run TestAcc
```

To try the provider against the live API without releasing it, build a binary
and point Terraform at it with a `dev_overrides` block in your CLI config
(`~/.terraformrc`):

```hcl
provider_installation {
  dev_overrides {
    "hornc-greedy/netcup" = "/path/to/built/binary/dir"
  }
  direct {}
}
```

## Notes and limitations

- **TTL** is a zone-level setting at netcup (`updateDnsZone`), exposed via
  `netcup_dns_zone` rather than as an attribute of individual records
- `netcup_dns_zone` manages the settings of an **existing** zone; it never
  creates or deletes the zone (`destroy` only drops it from state)
- netcup may normalize some values (trailing dots on `CNAME`/`MX` targets, `TXT`
  quoting); if you hit a perpetual diff, write the value exactly as netcup
  stores it
- **Reverse DNS (PTR)** is managed via the SCP REST API, not the DNS API; the IP
  must already belong to one of your servers
- **Domain registration and contacts** require netcup's reseller-only CCP
  methods, which return `4020 Function not available` on a standard account, so
  they are intentionally not implemented
- An **offline snapshot** (`online = false`, the default) restarts the server
  and needs `disk_name`

## License

[MPL-2.0](LICENSE)
