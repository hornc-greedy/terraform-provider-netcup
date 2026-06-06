# Changelog

All notable changes to this provider are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-06-06

### Added

- Initial release of the netcup Terraform provider, built on the
  [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework).
- **DNS (CCP API):** `netcup_dns_record` and `netcup_dns_zone` resources,
  `netcup_dns_records` data source.
- **Reverse DNS (SCP API):** `netcup_reverse_dns` resource.
- **Servers (SCP API):** `netcup_server`, `netcup_servers` and
  `netcup_server_interfaces` data sources.
- **SSH keys (SCP API):** `netcup_ssh_key` resource, `netcup_ssh_keys` data source.
- **Firewall (SCP API):** `netcup_firewall_policy` and `netcup_server_firewall`
  resources, `netcup_firewall_policies` data source.
- **Snapshots & failover (SCP API):** `netcup_server_snapshot` and
  `netcup_failover_ip_route` resources, `netcup_failover_ips` data source.
- **Maintenance (SCP API):** `netcup_maintenance` data source.

[1.0.0]: https://github.com/hornc-greedy/terraform-provider-netcup/releases/tag/v1.0.0
