terraform {
  required_providers {
    netcup = {
      source  = "hornc-greedy/netcup"
      version = "~> 1.1"
    }
  }
}

# Credentials may also be supplied via the environment variables
# NETCUP_CUSTOMER_NUMBER, NETCUP_API_KEY and NETCUP_API_PASSWORD.
provider "netcup" {
  customer_number = var.netcup_customer_number
  api_key         = var.netcup_api_key
  api_password    = var.netcup_api_password
}
