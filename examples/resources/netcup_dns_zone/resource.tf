resource "netcup_dns_zone" "example" {
  domain  = "example.com"
  ttl     = 3600
  refresh = 28800
  retry   = 7200
  expire  = 1209600
}
