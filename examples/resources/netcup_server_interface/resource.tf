resource "netcup_server_interface" "private" {
  server_id = 12345
  vlan_id   = 100
  driver    = "VIRTIO"
}
