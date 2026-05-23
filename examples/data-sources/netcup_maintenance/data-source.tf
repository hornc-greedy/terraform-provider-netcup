data "netcup_maintenance" "next" {}

output "maintenance_window" {
  value = "${data.netcup_maintenance.next.start_at} - ${data.netcup_maintenance.next.finish_at}"
}
