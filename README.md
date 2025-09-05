# headscale-pf

CLI tool for managing user groups in a Headscale policy file.

[![Stand with Ukraine](https://raw.githubusercontent.com/vshymanskyy/StandWithUkraine/main/banner2-direct.svg)](https://github.com/vshymanskyy/StandWithUkraine/blob/main/docs/README.md)

## Important
For Headscale versions below 0.26.*, use headscale-pf version 0.0.3.
Starting with version 1.0.0, headscale-pf uses a new user definition name format that corresponds to Headscale version 0.26.* and above.


## Supported sources

- [x] Jumpcloud
- [x] Authentik
- [x] LDAP/OpenLDAP/AD (tested on Jumpcloud LDAP)

## TODO

- Sources
    - Auth0

## Install

```shell
go install github.com/yousysadmin/headscale-pf/cmd/headscale-pf@latest
```

```shell
# By default install to $HOME/.bin dir
curl -L https://raw.githubusercontent.com/yousysadmin/headscale-pf/master/scripts/install.sh | bash

# Usage: install.sh [-b] bindir [-d] [tag]
#  -b sets bindir or installation directory, Defaults to ~/.bin
#  -d turns on debug logging
#   [tag] is a tag from
#   https://github.com/yousysadmin/headscale-pf/releases
#   If tag is missing, then the latest will be used.

```

## Usage

```
Usage:
  headscale-pf [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  prepare     Prepare policy

Flags:
      --endpoint string                    Source endpoint (can use env var PF_ENDPOINT)
  -h, --help                               help for headscale-pf
      --input-policy string                Headscale policy file template (default "./policy.hjson")
      --ldap-base-dn string                Base DN to use for LDAP searches (can use env var PF_LDAP_BASE_DN)
      --ldap-bind-dn string                Distinguished Name of the LDAP bind user account (can use env var PF_LDAP_BIND_DN)
      --ldap-bind-password string          LDAP password (can use env var PF_LDAP_BIND_PASSWORD)
      --ldap-default-email-domain string   Default email domain to append when user entries lack a mail attribute (can use env var PF_LDAP_DEFAULT_USER_EMAIL_DOMAIN)
      --no-color                           Disable color output
      --output-policy string               Headscale prepared policy file (default "./current.json")
      --source string                      Source (can use env var PF_SOURCE)
      --strip-email-domain                 Strip e-mail domain (default true)
      --token string                       A provider API token (can use env var PF_TOKEN) (default "jca_2W2JQEs6wKqHckLj2kqfCWTw4sVEK5jQygdG")
  -v, --version                            version for headscale-pf
```

The `--strip-email-domain` flag must be set eq to `oid.strip_email_domain` in your Headscale server config,
this flag determines whether it is necessary to trim the domain from the user's email or not, by default is `true`.

## Example

```json5
{
  "groups": {
    "group:admin": [
      "mega-admin"
    ],
    "group:network-all": [],
    // The Best Service
    "group:network-prod": [],
    "group:network-stage": [],
    "group:network-demo": [],
  }
}
```

### Jumpcloud
```sh
// Fill policy user groups from Jumpcloud
headscale-pf prepare --token=OOjjHH --source=jc --input-policy=policy.hjson --output-policy=out.json

// Push policy to Headscale
headscale policy set -f out.json
```

### Authentik
```sh
// Fill policy user groups from Authentik
headscale-pf prepare --token=OOjjHH --source=ak --input-policy=policy.hjson --output-policy=out.json

// Push policy to Headscale
headscale policy set -f out.json
```

### LDAP

`--endpoint` - LDAP server address "host:389" or "host:636" (can use env var PF_ENDPOINT)  
`--ldap-base-dn` - Base DN to use for LDAP searches (can use env var PF_LDAP_BASE_DN)  
`--ldap-bind-dn` - Distinguished Name of the LDAP bind user account (can use env var PF_LDAP_BIND_DN)  
`--ldap-bind-password` - LDAP password (can use env var PF_LDAP_BIND_PASSWORD)  
`--ldap-default-email-domain` - Default email domain to append when user entries lack a mail attribute (can use env var PF_LDAP_DEFAULT_USER_EMAIL_DOMAIN)  


```sh
headscale-pf prepare --source ldap \
                     --input-policy policy.hjson \
                     --output-policy=out.json \
                     --endpoint ldap.jumpcloud.com:636 \
                     --ldap-base-dn "o=<ORG_ID>,dc=jumpcloud,dc=com" \
                     --ldap-bind-dn "uid=<Service_User>,ou=Users,o=<ORG_ID>,dc=jumpcloud,dc=com" \
                     --ldap-bind-password "MySuperSecretPassword"
```

## Add a new source

1. Create a new file in the `internal/sources` dir
2. Write a new client, it must implement the interface

```go
// Must implement the search of a group by name in an external source
// used to find groups by group names from a policy file
GetGroupByName(grounName string) (*models.Group, error)

// Must implement the returns a list of users for group
// used for getting a list of users in a group
GetGroupMembers(groupId string, stripEmailDomain bool) ([]models.User, error)

// Must implement the returns a user info
// used for getting a user email and other needed information
GetUserInfo(userId string, stripEmailDomain bool) (models.User, error)
```

3. Add your source to the `internal/sources/sources.go` file and update `internal/sources/sources.go`.
