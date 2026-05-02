data "netcup_dns_records" "all" {
  domain = "example.com"
}

output "records" {
  value = data.netcup_dns_records.all.records
}
