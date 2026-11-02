action "netcup_server_state" "example" {
  config {
    server_id    = 12345
    state        = "ON"
    state_option = "RESET"
  }
}
