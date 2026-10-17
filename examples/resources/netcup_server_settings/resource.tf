# Manage selected settings of an existing server. Attributes left out keep
# their current value.
resource "netcup_server_settings" "web" {
  server_id   = 296155
  hostname    = "web01.example.com"
  nickname    = "web01"
  autostart   = true
  backup_time = "01:30:00+02:00"
}
