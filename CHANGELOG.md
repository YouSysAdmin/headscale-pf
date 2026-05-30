# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [v2.5.0] 2026-05-30
#### Breaking
- **Output policy**: the output is no longer always JSON. Previously the tool always wrote standard JSON; it now defaults to mirroring the input template's format (`--output-format auto`) — HuJSON for an HJSON template (comments and formatting preserved), or JSON for a strict-JSON template. Pass `--output-format json` to restore the always-JSON behavior.
- **Output policy**: the name of the output policy file has changed from `current.json` to `current.hjson`.
  
## [v2.4.0] 2026-04-29
#### Security
- LDAP bind aborts when StartTLS fails. Previously the bind continued over plaintext, leaking the bind password.
- TLS certificate verification is now opt-out via `--insecure-skip-tls-verify` (env `PF_INSECURE_SKIP_TLS_VERIFY`) instead of being silently disabled for Authentik (HTTPS) and LDAP (LDAPS / StartTLS).
- Environment variable values for `--token`, `--ldap-bind-password`, and other secret-bearing flags no longer appear in `headscale-pf --help` output.

#### Added
- `--output-format` flag selecting the output serialization: `auto` (default — detect from the input template: strict JSON in → JSON out, otherwise HuJSON), `hjson` (byte-for-byte HuJSON passthrough), or `json` (pretty-printed, 2-space-indented RFC-8259 JSON).
- `--insecure-skip-tls-verify` flag (env `PF_INSECURE_SKIP_TLS_VERIFY`) controlling TLS verification across all adapters.
- Test coverage: end-to-end `preparePolicy` flow with a stub `Source`, real-world `policy.hjson` round-trip, and `httptest`-based mocks for Authentik, Keycloak, and JumpCloud, plus pure-helper coverage for entry/group mapping functions.

#### Fixed
- **Authentik**: nil-pointer crash when a user had no email; endpoint port was silently stripped (e.g. `:9000` rerouted to `:443`); bearer token was skipped on non-HTTPS endpoints; shared-state hack across `GetGroupByName` / `GetGroupMembers` calls removed.
- **LDAP**: `objectClass` matching no longer false-positives on substrings (e.g. `posixGroupExtended` was matching `posixGroup`); empty-result errors no longer render as `<nil>`.
- **JumpCloud**: paginated members are deduplicated across page boundaries.
- **Policy**: `$schema` (editor JSON-schema reference for the HJSON template) is no longer leaked into the JSON output — Headscale doesn't recognize it.
- **Policy**: `WritePolicyToFile` no longer swallows `json.Marshal` errors.
- **CLI**: `--no-color` now actually disables color (it was evaluated before flag parsing).

#### Changed
- **Policy**: the output is now identical to a HuJSON input template one-to-one, the tool only populates a groups and does not format an output in any way.
- **JumpCloud**: per-user lookups during `GetGroupMembers` run through a bounded worker pool (8 workers), materially reducing wall-clock time on large groups.
- **Internal**: `Source` interface simplified — `GetUserInfo` removed (it was unused on three of four adapters). All adapters use pointer receivers; ID parameter naming standardized to `groupID` / `userID`.
- **Dependencies**: dropped a dead `juanfont/headscale` import.
- **Output policy**: the name of the output policy file has changed from `current.json` to `current.hjson`

## [v2.3.0] 2026-04-09
#### Added
- CHANGELOG.md
- Brew release
  ```
  brew install yousysadmin/apps/headscale-pf
  ```
#### Changed
- README.md

## [v2.2.1] 2026-04-09
#### Fixed
- Fix log buffer size and potential nil pointer [#13](https://github.com/YouSysAdmin/headscale-pf/pull/13)
- Fix dependabot security alerts  [#14](https://github.com/YouSysAdmin/headscale-pf/pull/14)
#### Changed
- Update Go and dependencies

## [v2.2.0] 2026-01-09
#### Changed
- Update Go to v1.25.5 and dependencies

## [v2.1.0] 2025-11-22
#### Added
- Add Keycloak source [#8](https://github.com/YouSysAdmin/headscale-pf/pull/8)

## [v2.0.3] 2025-11-12
#### Changed
- Update base image to Headscale v0.27.1

## [v2.0.2] 2025-10-27
#### Changed
- Pin Headscale version to v0.26.1

## [v2.0.1] 2025-10-27
#### Fixed
- Resolve issue where v0.27.0 prevents setting `null` for empty groups

## [v2.0.0] 2025-10-15
#### Removed
- Remove strip email domain

#### BREAKING
- This release should not break your policies for version 0.26.0+ with rare exceptions.
  Headscale uses OIDC Username Claim as the name of a new user who authenticates via OIDC.
  Currently headcale-pf uses OIDC Username as the username in groups, and if it is not an email address (does not contain @),
  appends @ to an username.

## [v1.2.1] 2025-09-25
#### Added
- Add Docker image build capabilities

## [v1.2.0] 2025-09-25
#### Changed
- Update dependency and Go to the v1.25.1

[Unreleased]: https://github.com/YouSysAdmin/headscale-pf/compare/v2.5.0...HEAD
[v2.3.0]: https://github.com/YouSysAdmin/headscale-pf/compare/v2.4.0...v2.5.0
[v2.3.0]: https://github.com/YouSysAdmin/headscale-pf/compare/v2.3.0...v2.4.0
[v2.3.0]: https://github.com/YouSysAdmin/headscale-pf/compare/v2.2.1...v2.3.0
[v2.2.1]: https://github.com/YouSysAdmin/headscale-pf/compare/v2.2.0...v2.2.1
[v2.2.0]: https://github.com/YouSysAdmin/headscale-pf/compare/v2.1.0...v2.2.0
[v2.1.0]: https://github.com/YouSysAdmin/headscale-pf/compare/v2.0.3...v2.1.0
[v2.0.3]: https://github.com/YouSysrad/headscale-pf/compare/v2.0.2...v2.0.3
[v2.0.2]: https://github.com/YouSysAdmin/headscale-pf/compare/v2.0.1...v2.0.2
[v2.0.1]: https://github.com/YouSysAdmin/headscale-pf/compare/v2.0.0...v2.0.1
[v2.0.0]: https://github.com/YouSysAdmin/headscale-pf/compare/v1.3.0-pre...v2.0.0
[v1.2.1]: https://github.com/YouSysAdmin/headscale-pf/compare/v1.2.0...v1.2.1
[v1.2.0]: https://github.com/YouSysAdmin/headscale-pf/compare/v1.1.0...v1.2.0
