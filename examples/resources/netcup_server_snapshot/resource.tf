resource "netcup_server_snapshot" "daily" {
  server_id   = 12345
  name        = "daily"
  description = "Managed by Terraform"
  disk_name   = "vda"
}
