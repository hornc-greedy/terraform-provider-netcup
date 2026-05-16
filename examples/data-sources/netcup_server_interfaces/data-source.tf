data "netcup_server_interfaces" "srv" {
  server_id = 296155
}

# All IPv4 addresses with their current reverse DNS.
output "ipv4_rdns" {
  value = {
    for ifc in data.netcup_server_interfaces.srv.interfaces :
    ifc.mac => [for a in ifc.ipv4_addresses : { ip = a.ip, rdns = a.rdns }]
  }
}
