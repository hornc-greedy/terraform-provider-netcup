# Look up failover IPs, then route one to a server.
data "netcup_failover_ips" "all" {}

resource "netcup_failover_ip_route" "primary" {
  failover_ip_id = one([for ip in data.netcup_failover_ips.all.ipv4 : ip.id if ip.ip == "203.0.113.200"])
  server_id      = 12345
}
