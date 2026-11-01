resource "netcup_server_rescue_system" "repair" {
  server_id = 12345
}

ephemeral "netcup_server_rescue_system" "repair" {
  server_id  = netcup_server_rescue_system.repair.server_id
  depends_on = [netcup_server_rescue_system.repair]
}
