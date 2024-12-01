# headscale-pf

CLI tool for managing user groups in a Headscale policy file.

[![Stand with Ukraine](https://raw.githubusercontent.com/vshymanskyy/StandWithUkraine/main/banner2-direct.svg)](https://github.com/vshymanskyy/StandWithUkraine/blob/main/docs/README.md)

## Supported sources

- [x] Jumpcloud

## TODO

- Sources
    - Authentik
    - Auth0

- Input policy format
    - Yaml
    - HCL

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
Obtaining information about groups and group members from external sources and populating groups in the Headscale policy.

Usage:
  headscale-pf [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  prepare     Prepare policy

Flags:
  -h, --help                   help for headscale-pf
      --input-policy string    Headscale policy file template (default "./policy.hjson")
      --no-color               Disable color output
      --output-policy string   Headscale prepared policy file (default "./current.json")
      --password string        A provider API user password (can use env var PF_USER_PASSWORD)
      --strip-email-domain     Strip e-mail domain (default true)
      --token string           A provider API token (can use env var PF_TOKEN)
      --user string            A provider API user (can use env var PF_USER_NAME)
```

The `--strip-email-domain` flag must be set eq to `oid.strip_email_domain` in your Headscale server config,
this flag determines whether it is necessary to trim the domain from the user's email or not, by default is `true`.

## Example

### Jumpcloud

```sh
// Fill policy user groups from Jumpcloud
PF_TOKEN=0000000 headscale-pf prepare --input-policy=policy.hjson --output-policy=out.json

// Push policy to Headscale
headscale policy set -f out.json
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

3. Add your source to `internal/sources/sources.go`