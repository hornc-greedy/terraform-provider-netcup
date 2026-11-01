resource "netcup_server_disk_driver" "all" {
  server_id = 12345
  driver    = "VIRTIO_SCSI"
}
