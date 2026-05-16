data "netcup_server" "web" {
  id = 12345
}

output "web_hostname" {
  value = data.netcup_server.web.hostname
}
