resource "netcup_server_snapshot" "daily" {
  server_id   = 296155
  name        = "daily"
  description = "Managed by Terraform"
  disk_name   = "vda"
}
