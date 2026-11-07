# Changelog

## 1.2.0 (November 7, 2026)

NOTES:

* provider: Ephemeral resources need Terraform 1.10 or newer, actions need Terraform 1.14 or newer.
* provider: The provider covers every operation of the SCP and netcup REST APIs. The [API coverage guide](docs/guides/api-coverage.md) maps each operation to its resource, data source, ephemeral resource or action.
* data-source/netcup_server: `snapshot_count` is deprecated, use `export_count`. netcup removes `snapshotCount` by December 31, 2026, so `snapshot_count` now reads `exportCount`.
* resource/netcup_dns_record: A record that is gone is planned for creation. The CCP API reports a domain on CloudDNS the same way, see the README before applying such a plan.

FEATURES:

* **New Action:** `netcup_server_disk_format`
* **New Action:** `netcup_server_firewall_reapply`
* **New Action:** `netcup_server_firewall_restore_copied_policies`
* **New Action:** `netcup_server_image_setup`
* **New Action:** `netcup_server_snapshot_export`
* **New Action:** `netcup_server_snapshot_revert`
* **New Action:** `netcup_server_state`
* **New Action:** `netcup_server_storage_optimization`
* **New Action:** `netcup_server_user_image_setup`
* **New Action:** `netcup_task_cancel`
* **New Data Source:** `netcup_clouddns_acme_challenges`
* **New Data Source:** `netcup_clouddns_domain`
* **New Data Source:** `netcup_clouddns_domains`
* **New Data Source:** `netcup_clouddns_record`
* **New Data Source:** `netcup_clouddns_record_types`
* **New Data Source:** `netcup_clouddns_records`
* **New Data Source:** `netcup_clouddns_revision`
* **New Data Source:** `netcup_clouddns_revisions`
* **New Data Source:** `netcup_reverse_dns`
* **New Data Source:** `netcup_server_disk`
* **New Data Source:** `netcup_server_disk_supported_drivers`
* **New Data Source:** `netcup_server_disks`
* **New Data Source:** `netcup_server_firewall`
* **New Data Source:** `netcup_server_guest_agent`
* **New Data Source:** `netcup_server_guest_agent_status`
* **New Data Source:** `netcup_server_image_flavours`
* **New Data Source:** `netcup_server_interface`
* **New Data Source:** `netcup_server_iso`
* **New Data Source:** `netcup_server_iso_images`
* **New Data Source:** `netcup_server_logs`
* **New Data Source:** `netcup_server_metrics_cpu`
* **New Data Source:** `netcup_server_metrics_disk`
* **New Data Source:** `netcup_server_metrics_network`
* **New Data Source:** `netcup_server_metrics_network_packet`
* **New Data Source:** `netcup_server_snapshot`
* **New Data Source:** `netcup_server_snapshot_dryrun`
* **New Data Source:** `netcup_server_snapshots`
* **New Data Source:** `netcup_task`
* **New Data Source:** `netcup_tasks`
* **New Data Source:** `netcup_user`
* **New Data Source:** `netcup_user_images`
* **New Data Source:** `netcup_user_isos`
* **New Data Source:** `netcup_user_logs`
* **New Data Source:** `netcup_user_vlan`
* **New Data Source:** `netcup_user_vlans`
* **New Data Source:** `netcup_vlan`
* **New Ephemeral Resource:** `netcup_server_gpu_driver`
* **New Ephemeral Resource:** `netcup_server_rescue_system`
* **New Ephemeral Resource:** `netcup_server_snapshot_download`
* **New Ephemeral Resource:** `netcup_user_image_download`
* **New Ephemeral Resource:** `netcup_user_iso_download`
* **New Resource:** `netcup_clouddns_acme_challenge`
* **New Resource:** `netcup_clouddns_record`
* **New Resource:** `netcup_server_disk_driver`
* **New Resource:** `netcup_server_interface`
* **New Resource:** `netcup_server_iso`
* **New Resource:** `netcup_server_rescue_system`
* **New Resource:** `netcup_user`
* **New Resource:** `netcup_user_image`
* **New Resource:** `netcup_user_iso`
* **New Resource:** `netcup_user_vlan`

IMPROVEMENTS:

* provider: Add `ccp_endpoint` (`NETCUP_CCP_ENDPOINT`), `clouddns_api_key`, `clouddns_base_url` and `scp_base_url` (`NETCUP_SCP_BASE_URL`).
* provider: SCP requests renew the token after a 401 and retry idempotent requests after a transport error or a 5xx. List endpoints read all pages.
* provider: Errors of failed SCP tasks include `responseError` if netcup sets it.
* provider: Unknown provider attributes fail with a clear error instead of reading as unset.
* data-source/netcup_dns_records: Return an empty list with a warning for a zone without records instead of failing.
* data-source/netcup_server: Add every field of the API, including `server_live_info` and `load_server_live_info`. The interfaces of the live info are in `netcup_server_interfaces`.
* data-source/netcup_servers: Add `template`.
* resource/netcup_failover_ip_route: Add `timeouts`.
* resource/netcup_firewall_policy: Add the `update` timeout.
* resource/netcup_firewall_policy: Updates wait for the task netcup returns and fail if the task fails.
* resource/netcup_server_firewall: Add `keep_copied_policies`. It defaults to `false`, which removes the copied policies of the interface as before.
* resource/netcup_server_settings: Add `root_password_wo`, `root_password_wo_version` and `state_option`.
* resource/netcup_user_image: Upload in multiple parts.
* resource/netcup_user_iso: Upload in multiple parts.

BUG FIXES:

* provider: CCP requests failed after 15 minutes without a request in the same run, e.g. during a long snapshot. The provider now logs in again.
* provider: CCP requests failed when netcup ended a session early. The provider now logs in again once.
* resource/netcup_dns_record: Creating the first record of an empty zone and refreshing after the last one was gone failed, because netcup answers an empty zone with an error.
* resource/netcup_dns_record: The id of another record could be stored if the zone changed during the create.
* resource/netcup_failover_ip_route: A failover IP that no longer exists or has no route is now removed from state or routed again.
* resource/netcup_firewall_policy: A permanent diff appeared when netcup returned `sources` or `destinations` in a different order, or when a rule set them to `[]` or a text attribute to `""`. The configured values now stay stable.
* resource/netcup_firewall_policy: Removing the description or all rules is now sent to netcup.
* resource/netcup_server_firewall: Refresh failed when netcup returned 404 for the firewall. The resource is now removed from state.

## 1.1.0 (October 17, 2026)

NOTES:

* data-source/netcup_maintenance: Deprecated. netcup removes the underlying SCP endpoint `GET /api/v1/maintenance` by December 31, 2026. The data source will be removed in the next major version.
* resource/netcup_server_snapshot: If you imported an offline snapshot, add its disk, e.g. `disk_name = "vda"`, to the configuration before upgrading. Online snapshots are unchanged.

FEATURES:

* **New Resource:** `netcup_server_settings`

IMPROVEMENTS:

* provider: `api_key` is marked sensitive, so Terraform redacts it in its output like `api_password` and `scp_refresh_token`. Existing configurations need no changes.

BUG FIXES:

* provider: SCP validation errors showed only "Validation failed". They now include the reason netcup gives for each field.
* resource/netcup_failover_ip_route: Without `ipv6` the apply failed with "Provider returned invalid result object after apply". `ipv6` now defaults to `false`. Existing state keeps its resources without replacement.
* resource/netcup_server_snapshot: With the default `online = false` the create failed with "Validation failed", because netcup requires a disk for offline snapshots. The new `disk_name` attribute selects it and is validated at plan time. The docs now state that an offline snapshot restarts the server.

## 1.0.0 (June 6, 2026)

NOTES:

* provider: Initial release, built on the [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework).

FEATURES:

* **New Data Source:** `netcup_dns_records`
* **New Data Source:** `netcup_failover_ips`
* **New Data Source:** `netcup_firewall_policies`
* **New Data Source:** `netcup_maintenance`
* **New Data Source:** `netcup_server`
* **New Data Source:** `netcup_server_interfaces`
* **New Data Source:** `netcup_servers`
* **New Data Source:** `netcup_ssh_keys`
* **New Resource:** `netcup_dns_record`
* **New Resource:** `netcup_dns_zone`
* **New Resource:** `netcup_failover_ip_route`
* **New Resource:** `netcup_firewall_policy`
* **New Resource:** `netcup_reverse_dns`
* **New Resource:** `netcup_server_firewall`
* **New Resource:** `netcup_server_snapshot`
* **New Resource:** `netcup_ssh_key`
