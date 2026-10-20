# Reverse DNS (PTR) for a server IP. Needs scp_refresh_token on the provider.
resource "netcup_reverse_dns" "mail_v4" {
  ip       = "203.0.113.10"
  hostname = "mail.example.com"
}

resource "netcup_reverse_dns" "mail_v6" {
  ip       = "2001:db8::1"
  hostname = "mail.example.com"
}
