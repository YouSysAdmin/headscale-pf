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
- Keycloak

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

### Sources
- `jc`, `jumpcloud` - Jumpcloud
- `ak`, `authentik` - Authentik
- `ldap`, `ldaps` - LDAP
- `kk`, `keycloak` - Keycloak

### Global Flags
| Flag / Option                  | Description                                         | Env var                              | Default            |
|--------------------------------|-----------------------------------------------------|--------------------------------------|--------------------|
| `--source string`              | Source type (`jc`, `ak`, `ldap`, `kk`)              | `PF_SOURCE`                          | –                  |
| `--endpoint string`            | Source endpoint                                     | `PF_ENDPOINT`                        | –                  |
| `--token string`               | API token                                           | `PF_TOKEN`                           | –                  |
| `--input-policy string`        | Input policy template                               | –                                    | `./policy.hjson`   |
| `--output-policy string`       | Output policy file                                  | –                                    | `./current.json`   |
| `--ldap-base-dn string`        | LDAP base DN                                        | `PF_LDAP_BASE_DN`                    | –                  |
| `--ldap-bind-dn string`        | LDAP bind DN                                        | `PF_LDAP_BIND_DN`                    | –                  |
| `--ldap-bind-password string`  | LDAP password                                       | `PF_LDAP_BIND_PASSWORD`              | –                  |
| `--ldap-default-email-domain`  | LDAP default email domain                           | `PF_LDAP_DEFAULT_USER_EMAIL_DOMAIN`  | –                  |
| `--keycloak-realm string`      | Keycloak Realm                                      | `PF_KEYCLOAK_REALM`                  | –                  |
| `--no-color`                   | Disable colored output                              | –                                    | –                  |
| `-v`, `--version`              | Show version                                        | –                                    | –                  |

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
   - `GetGroupMembers(groupID string) ([]models.User, error)`
   - `GetUserInfo(userID string) (models.User, error)`
3. Register it in `internal/sources/sources.go`.


---

## Use CI
Use the Headscale-PF Docker image inside your CI (the Docker image contains the Headscale CLI)  

`PF_TOKEN` - Jumpcloud/etc. API token  
`HEADSCALE_CLI_ADDRESS` - Headscale GRPC Endpoint  
`HEADSCALE_CLI_API_KEY` - Headscale GRPC Token  

```yaml
# .github/workflows/policy.yaml
name: Headscale Policy

on:
  workflow_dispatch:
    inputs:
      apply_policy:
        description: "Apply policy after generation?"
        type: boolean
        required: true
        default: true
      save_artifact:
        description: "Save generated policy.json as artifact?"
        type: boolean
        required: true
        default: true

permissions:
  contents: read

jobs:
  generate-policy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Run policy prepare
        env:
          IMAGE: ghcr.io/yousysadmin/headscale-pf:latest
          PF_TOKEN: ${{ secrets.PF_TOKEN }}
          HEADSCALE_CLI_ADDRESS: ${{ secrets.HEADSCALE_CLI_ADDRESS }}
          HEADSCALE_CLI_API_KEY: ${{ secrets.HEADSCALE_CLI_API_KEY }}
          APPLY_INPUT: ${{ github.event.inputs.apply_policy }}
        run: |
          set -euxo pipefail

          docker pull "$IMAGE"

          APPLY_FLAG=""
          if [ "${APPLY_INPUT}" = "true" ]; then
            APPLY_FLAG="-e APPLY_POLICY=1"
          fi

          # fix: headscale config file should be present anyway
          touch config.yaml

          docker run --rm \
            --user "$(id -u)":"$(id -g)" \
            -e PF_TOKEN="${PF_TOKEN}" \
            ${APPLY_FLAG} \
            -e HEADSCALE_CLI_ADDRESS="${HEADSCALE_CLI_ADDRESS}" \
            -e HEADSCALE_CLI_API_KEY="${HEADSCALE_CLI_API_KEY}" \
            -e INPUT_POLICY="/work/policy.hjson" \
            -e OUTPUT_POLICY="/work/policy.json" \
            -e SOURCE="jc" \
            -e RETRIES="5" \
            -e RETRY_DELAY_SEC="6" \
            -v "$GITHUB_WORKSPACE:/work" \
            "$IMAGE"

      - name: Upload policy.json artifact
        if: ${{ github.event.inputs.save_artifact == 'true' }}
        uses: actions/upload-artifact@v4
        with:
          name: policy.json
          path: ${{ github.workspace }}/policy.json

```
