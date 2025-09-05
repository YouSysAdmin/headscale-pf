# headscale-pf

[![Stand with Ukraine](https://raw.githubusercontent.com/vshymanskyy/StandWithUkraine/main/banner2-direct.svg)](https://github.com/vshymanskyy/StandWithUkraine/blob/main/docs/README.md)

`headscale-pf` is a CLI tool for managing user groups in a [**Headscale**](https://github.com/juanfont/headscale) policy file.  
It integrates with external identity providers such as **Jumpcloud**, **Authentik**, and **LDAP** to automatically build or update Headscale ACL policies.

---

## Compatibility

- For **Headscale < 0.26**, use `headscale-pf` version **0.0.3**.  
- From **1.0.0** onward, `headscale-pf` uses a new user definition format compatible with **Headscale ≥ 0.26**.

---

## Supported Sources

- Jumpcloud
- Authentik
- LDAP / OpenLDAP / Active Directory** (tested with Jumpcloud LDAP)

Planned:
- Auth0
- CSV
- JSON
- REMOTE JSON
- ...

---

## Installation

### Using Go
```bash
go install github.com/YouSysAdmin/headscale-pf/cmd/headscale-pf@latest
```

### Using install script
```bash
curl -L https://raw.githubusercontent.com/YouSysAdmin/headscale-pf/master/scripts/install.sh | bash
```

Options:
- `-b` → install directory (default: `~/.bin`)
- `-d` → enable debug logging
- `[tag]` → optional release tag (latest if omitted)

---

## Usage

```bash
headscale-pf [command] [flags]
```

### Commands
- `prepare` – fetch group membership and generate a Headscale policy
- `completion` – generate autocomplete script for your shell
- `help` – show help for any command

### Global Flags
- `--source string` → source type (`jc`, `ak`, `ldap`) (`PF_SOURCE`)
- `--endpoint string` → source endpoint (`PF_ENDPOINT`)
- `--token string` → API token (`PF_TOKEN`)
- `--input-policy string` → input policy template (default: `./policy.hjson`)
- `--output-policy string` → output policy file (default: `./current.json`)
- `--ldap-base-dn string` → LDAP base DN (`PF_LDAP_BASE_DN`)
- `--ldap-bind-dn string` → LDAP bind DN (`PF_LDAP_BIND_DN`)
- `--ldap-bind-password string` → LDAP password (`PF_LDAP_BIND_PASSWORD`)
- `--ldap-default-email-domain string` → LDAP default email domain (`PF_LDAP_DEFAULT_USER_EMAIL_DOMAIN`)
- `--strip-email-domain` → strip email domain (default: `true`) – must match Headscale config
- `--no-color` → disable colored output
- `-v, --version` → show version

---

## Examples

### Jumpcloud
```bash
headscale-pf prepare \
            --source=jc \
            --token=$JC_TOKEN \
            --input-policy=policy.hjson \
            --output-policy=out.json

headscale policy set -f out.json
```

### Authentik
```bash
headscale-pf prepare \
            --source=ak \
            --endpoint="https://auth.example.com" \
            --token=$AK_TOKEN \
            --input-policy=policy.hjson \
            --output-policy=out.json

headscale policy set -f out.json
```

### LDAP
```bash
headscale-pf prepare \
            --source=ldap \
            --endpoint=ldap.example.com:636 \
            --ldap-base-dn="ou=Users,dc=example,dc=com" \
            --ldap-bind-dn="cn=service,ou=Users,dc=example,dc=com" \
            --ldap-bind-password=$LDAP_PASS \
            --ldap-default-email-domain="example.com" \
            --input-policy=policy.hjson \
            --output-policy=out.json

headscale policy set -f out.json
```

---

## Adding a New Source

1. Create a new file under `internal/sources/`.
2. Implement the interface:
   - `GetGroupByName(groupName string) (*models.Group, error)`
   - `GetGroupMembers(groupID string, stripEmailDomain bool) ([]models.User, error)`
   - `GetUserInfo(userID string, stripEmailDomain bool) (models.User, error)`
3. Register it in `internal/sources/sources.go`.
