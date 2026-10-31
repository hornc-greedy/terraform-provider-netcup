resource "netcup_clouddns_acme_challenge" "www" {
  domain_id = 12345
  scope     = "www"
  value     = "abcdefghijklmnopqrstuvwxyz0123456789_-ABCDE"
}
