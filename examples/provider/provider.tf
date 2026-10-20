terraform {
  required_providers {
    netcup = {
      source  = "hornc-greedy/netcup"
      version = "~> 1.1"
    }
  }
}

# Credentials may also be supplied via the environment variables
# NETCUP_CUSTOMER_NUMBER, NETCUP_API_KEY, NETCUP_API_PASSWORD and
# NETCUP_SCP_REFRESH_TOKEN. Configure only the APIs you use.
provider "netcup" {
  customer_number = var.netcup_customer_number
  api_key         = var.netcup_api_key
  api_password    = var.netcup_api_password

  scp_refresh_token = var.netcup_scp_refresh_token
}
