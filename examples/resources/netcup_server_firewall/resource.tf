resource "netcup_firewall_policy" "web" {
  name = "web"
  rule {
    action            = "ACCEPT"
    protocol          = "TCP"
    direction         = "INGRESS"
    destination_ports = "443"
    sources           = ["0.0.0.0/0"]
  }
}

resource "netcup_server_firewall" "srv" {
  server_id  = 296155
  mac        = "ba:ad:7a:1f:d3:68"
  policy_ids = [netcup_firewall_policy.web.id]
  active     = true
}
