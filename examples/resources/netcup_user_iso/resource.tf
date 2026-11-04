resource "netcup_user_iso" "example" {
  key         = "install.iso"
  source      = "${path.module}/install.iso"
  source_hash = filesha256("${path.module}/install.iso")
}
