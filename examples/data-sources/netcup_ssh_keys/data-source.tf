data "netcup_ssh_keys" "all" {}

output "ssh_key_names" {
  value = [for k in data.netcup_ssh_keys.all.keys : k.name]
}
