---
page_title: "Moving a domain to CloudDNS"
subcategory: ""
description: |-
  How to move the records of a domain from netcup_dns_record to netcup_clouddns_record.
---

# Moving a domain to CloudDNS

The CCP API can report a domain on CloudDNS as an empty zone. The provider then
warns, `netcup_dns_records` returns an empty list and `netcup_dns_record` plans to
create its records again. Do not apply such a plan. Manage the domain with
`netcup_clouddns_record` once its records exist on CloudDNS.

This needs Terraform 1.7 or newer, and both the CCP credentials and
`clouddns_api_key` for that apply. `netcup_clouddns_domain` shows the `domain_id`
and `netcup_clouddns_records` the exact values for each import id. In one step,
replace each `netcup_dns_record` with a `removed` block, an `import` block and a
`netcup_clouddns_record`. Check that the plan only imports and forgets, apply it,
and delete both blocks afterwards.

```terraform
removed {
  from = netcup_dns_record.www
  lifecycle {
    destroy = false
  }
}

import {
  to = netcup_clouddns_record.www
  id = "12345/www/A/192.0.2.10"
}

resource "netcup_clouddns_record" "www" {
  domain_id = 12345
  name      = "www"
  type      = "A"
  rdata     = "192.0.2.10"
  ttl       = 300
}
```

See [`removed`](https://developer.hashicorp.com/terraform/language/block/removed)
and [`import`](https://developer.hashicorp.com/terraform/language/import).
