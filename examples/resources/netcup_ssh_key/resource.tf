resource "netcup_ssh_key" "deploy" {
  name = "deploy-key"
  key  = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... user@host"
}
