resource "netcup_firewall_policy" "web" {
  name        = "web"
  description = "Allow SSH and HTTPS in"

  rule {
    action            = "ACCEPT"
    protocol          = "TCP"
    direction         = "INGRESS"
    destination_ports = "22"
    sources           = ["0.0.0.0/0"]
    description       = "SSH"
  }

  rule {
    action            = "ACCEPT"
    protocol          = "TCP"
    direction         = "INGRESS"
    destination_ports = "443"
    sources           = ["0.0.0.0/0"]
    description       = "HTTPS"
  }
}
