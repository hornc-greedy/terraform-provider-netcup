data "netcup_clouddns_domain" "example" {
  fqdn = "example.com"
}

resource "netcup_clouddns_record" "www" {
  domain_id = data.netcup_clouddns_domain.example.id
  name      = "www"
  type      = "A"
  rdata     = "192.0.2.10"
  ttl       = 300
}
