variable "iso_name" {
  type = string
}

data "netcup_server_iso_images" "available" {
  server_id = 12345
}

resource "netcup_server_iso" "installer" {
  server_id                   = 12345
  iso_id                      = one([for iso in data.netcup_server_iso_images.available.iso_images : iso.id if iso.name == var.iso_name])
  change_boot_device_to_cdrom = true
}
