---
page_title: "SCP authentication"
subcategory: ""
description: |-
  How to obtain, use and revoke the offline refresh token of the SCP REST API.
---

# SCP authentication

The SCP REST API accepts a Bearer access token, which expires after 300 seconds.
The provider takes an offline refresh token as `scp_refresh_token` and requests
new access tokens with it. These are the steps netcup documents in the SCP under
API, REST API Documentation.

## Obtain an offline refresh token

Request a device code:

```shell
curl -X POST 'https://www.servercontrolpanel.de/realms/scp/protocol/openid-connect/auth/device' \
  -d 'client_id=scp' \
  -d 'scope=offline_access openid'
```

Open `verification_uri_complete` of the response in a browser, log in and grant
SCP access to `profile`, `offline_access`, `roles` and `email`. Then exchange the
`device_code` of the response within `expires_in` seconds:

```shell
curl -X POST 'https://www.servercontrolpanel.de/realms/scp/protocol/openid-connect/token' \
  -d 'grant_type=urn:ietf:params:oauth:grant-type:device_code' \
  -d 'device_code=<device-code>' \
  -d 'client_id=scp'
```

`refresh_token` of the response is the offline refresh token. Pass it as
`scp_refresh_token` or `NETCUP_SCP_REFRESH_TOKEN`, ideally from a secret store.

## Keep it valid

The refresh token can be used many times. It stays valid as long as it is used at
least once every 30 days, which every Terraform run that reads an SCP object
does. The provider requests the access tokens itself:

```shell
curl 'https://www.servercontrolpanel.de/realms/scp/protocol/openid-connect/token' \
  -d 'client_id=scp' \
  -d 'refresh_token=<refresh-token>' \
  -d 'grant_type=refresh_token'
```

## Revoke it

Revoke a refresh token that leaked or is no longer needed:

```shell
curl 'https://www.servercontrolpanel.de/realms/scp/protocol/openid-connect/revoke' \
  -d 'client_id=scp' \
  -d 'token=<refresh-token>' \
  -d 'token_type_hint=refresh_token'
```

A refresh token you no longer have is revoked in the
[Account Console](https://www.servercontrolpanel.de/realms/scp/account) under
Applications, application `scp`, Remove access.
