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
  server_id  = 12345
  mac        = "00:00:5e:00:53:01"
  policy_ids = [netcup_firewall_policy.web.id]
  active     = true
}
