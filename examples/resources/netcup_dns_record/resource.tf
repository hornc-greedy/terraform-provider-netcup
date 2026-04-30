resource "netcup_dns_record" "www" {
  domain      = "example.com"
  hostname    = "www"
  type        = "A"
  destination = "203.0.113.10"
}

resource "netcup_dns_record" "mx" {
  domain      = "example.com"
  hostname    = "@"
  type        = "MX"
  priority    = "10"
  destination = "mail.example.com"
}

resource "netcup_dns_record" "spf" {
  domain      = "example.com"
  hostname    = "@"
  type        = "TXT"
  destination = "v=spf1 -all"
}
