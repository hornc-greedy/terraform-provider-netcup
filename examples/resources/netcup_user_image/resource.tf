resource "netcup_user_image" "example" {
  key         = "disk.img"
  source      = "${path.module}/disk.img"
  source_hash = filesha256("${path.module}/disk.img")
}
