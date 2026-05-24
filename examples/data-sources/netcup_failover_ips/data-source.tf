data "netcup_failover_ips" "all" {}

output "failover_ipv4_addresses" {
  value = [for ip in data.netcup_failover_ips.all.ipv4 : ip.ip]
}
