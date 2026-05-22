data "netcup_firewall_policies" "all" {}

output "firewall_policy_ids" {
  value = [for p in data.netcup_firewall_policies.all.policies : p.id]
}
