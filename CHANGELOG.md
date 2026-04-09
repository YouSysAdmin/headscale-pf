# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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

[Unreleased]: https://github.com/YouSysAdmin/jc2aws/compare/v2.3.0...HEAD
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
