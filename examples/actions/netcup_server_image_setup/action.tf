action "netcup_server_image_setup" "example" {
  config {
    server_id        = 12345
    image_flavour_id = 1
    disk_name        = "vda"
  }
}
