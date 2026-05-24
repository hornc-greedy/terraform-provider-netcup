# Look up failover IPs, then route one to a server.
data "netcup_failover_ips" "all" {}

resource "netcup_failover_ip_route" "primary" {
  failover_ip_id = data.netcup_failover_ips.all.ipv4[0].id
  server_id      = 296155
}
