variable "new_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

variable "current_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "netcup_user" "example" {
  language      = "de"
  time_zone     = "Europe/Berlin"
  show_nickname = true

  password_wo         = var.new_password
  old_password_wo     = var.current_password
  password_wo_version = 1
}
