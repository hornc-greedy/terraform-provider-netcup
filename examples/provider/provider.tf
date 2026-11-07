terraform {
  required_providers {
    netcup = {
      source  = "hornc-greedy/netcup"
      version = "~> 1.2"
    }
  }
}

# Credentials may also be supplied via the environment variables
# NETCUP_CUSTOMER_NUMBER, NETCUP_API_KEY, NETCUP_API_PASSWORD,
# NETCUP_CLOUDDNS_API_KEY and NETCUP_SCP_REFRESH_TOKEN. Configure only the APIs
# you use.
provider "netcup" {
  customer_number = var.netcup_customer_number
  api_key         = var.netcup_api_key
  api_password    = var.netcup_api_password

  clouddns_api_key = var.netcup_clouddns_api_key

  scp_refresh_token = var.netcup_scp_refresh_token
}
