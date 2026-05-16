data "netcup_servers" "all" {}

output "server_ids" {
  value = [for s in data.netcup_servers.all.servers : s.id]
}
