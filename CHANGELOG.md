# Changelog

All notable changes to this provider are documented in this file. The format is
based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- The provider attribute `api_key` is marked sensitive, so Terraform redacts it
  in its output like `api_password` and `scp_refresh_token`. Existing
  configurations need no changes.

### Deprecated

- `netcup_maintenance` data source. netcup removes the underlying SCP endpoint
  `GET /api/v1/maintenance` by 31.12.2026. The data source will be removed in
  the next major version.

### Fixed

- `netcup_failover_ip_route` without `ipv6` failed after apply with "Provider
  returned invalid result object after apply". `ipv6` now defaults to `false`.
  Existing state keeps its resources without replacement.
- `netcup_server_snapshot` with the default `online = false` failed with
  "Validation failed" because netcup requires a disk for offline snapshots. The
  new `disk_name` attribute selects it and is validated at plan time. If you
  imported an offline snapshot, add its disk, e.g. `disk_name = "vda"`, to the
  configuration before upgrading. Online
  snapshots are unchanged. The docs now state that an offline snapshot restarts
  the server.
- SCP validation errors showed only "Validation failed". They now include the
  reason netcup gives for each field.

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

[Unreleased]: https://github.com/hornc-greedy/terraform-provider-netcup/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/hornc-greedy/terraform-provider-netcup/releases/tag/v1.0.0
